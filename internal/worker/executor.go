// Package worker реалізує оркестраційний шар TetraCore Worker —
// конкурентне виконання задач, lifecycle-менеджмент та інтеграцію
// з Hub (через WebSocket), реєстром дій (actions) і метриками.
package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// ---------------------------------------------------------------------------
// Executor — конкурентно-обмежений виконавець задач.
// ---------------------------------------------------------------------------
//
// Використовує буферизований канал як семафор для обмеження кількості
// одночасно виконуваних задач. Збирає атомарні лічильники для моніторингу.

// Executor контролює паралелізм виконання задач.
// Семафор (buffered channel) гарантує, що одночасно виконується
// не більше concurrency задач. Метрики оновлюються атомарно.
type Executor struct {
	semaphore chan struct{} // буферизований канал — семафор доступних слотів

	logger zerolog.Logger

	mu sync.RWMutex // захист для неатомарних полів (наразі не використовується,
	// зарезервовано для майбутніх розширень)

	limit int64 // atomic — поточний ліміт конкурентності

	activeTasks    int64 // atomic — кількість задач, що виконуються зараз
	totalProcessed int64 // atomic — загальна кількість успішно оброблених задач
	totalFailed    int64 // atomic — загальна кількість невдалих задач
}

// NewExecutor створює новий Executor з обмеженням конкурентності.
//
// concurrency — максимальна кількість задач, що виконуються одночасно.
// Якщо concurrency <= 0, використовується значення за замовчуванням (10).
func NewExecutor(concurrency int, logger zerolog.Logger) *Executor {
	return NewExecutorWithMax(concurrency, concurrency, logger)
}

// NewExecutorWithMax створює Executor з початковим та максимальним лімітами.
// maxConcurrency задає ємність семафора для можливості зростання.
func NewExecutorWithMax(concurrency, maxConcurrency int, logger zerolog.Logger) *Executor {
	if concurrency <= 0 {
		concurrency = 10
	}
	if maxConcurrency < concurrency {
		maxConcurrency = concurrency
	}

	sem := make(chan struct{}, maxConcurrency)
	for i := 0; i < concurrency; i++ {
		sem <- struct{}{}
	}

	e := &Executor{
		semaphore: sem,
		logger:    logger.With().Str("component", "executor").Logger(),
		limit:     int64(concurrency),
	}

	return e
}

// Execute виконує функцію fn з обмеженням конкурентності та таймаутом.
//
// Послідовність:
//  1. Чекає вільний слот у семафорі (або скасування ctx).
//  2. Створює дочірній контекст з таймаутом (якщо timeout > 0).
//  3. Виконує fn у поточній горутині (виклик уже має бути в горутині).
//  4. Оновлює атомарні лічильники.
//  5. Звільняє слот семафора (defer).
//
// Повертає помилку, якщо:
//   - ctx скасовано до отримання слота;
//   - timeout вичерпано під час виконання fn;
//   - fn повернула помилку.
func (e *Executor) Execute(ctx context.Context, fn func(ctx context.Context) error, timeout time.Duration) error {
	// --- 1. Захоплення слота семафора ---
	sem := e.semaphore
	select {
	case <-sem:
		// Слот отримано — продовжуємо.
	case <-ctx.Done():
		return fmt.Errorf("не вдалося отримати слот виконання: %w", ctx.Err())
	}
	// Гарантуємо звільнення слота при виході.
	defer func() { sem <- struct{}{} }()

	// --- 2. Інкремент активних задач ---
	active := atomic.AddInt64(&e.activeTasks, 1)
	e.logger.Debug().Int64("active_tasks", active).Msg("задача розпочата")

	defer func() {
		newActive := atomic.AddInt64(&e.activeTasks, -1)
		e.logger.Debug().Int64("active_tasks", newActive).Msg("задача завершена")
	}()

	// --- 3. Таймаут ---
	execCtx := ctx
	var cancelTimeout context.CancelFunc

	if timeout > 0 {
		execCtx, cancelTimeout = context.WithTimeout(ctx, timeout)
		defer cancelTimeout()
	}

	// --- 4. Виконання ---
	start := time.Now()

	errCh := make(chan error, 1)
	go func() {
		errCh <- fn(execCtx)
	}()

	var execErr error
	select {
	case execErr = <-errCh:
		// fn завершилась — обробляємо результат нижче.
	case <-execCtx.Done():
		// Таймаут або скасування — чекаємо завершення fn,
		// щоб уникнути витоку горутини (з коротким таймаутом).
		execErr = execCtx.Err()

		// Даємо горутині шанс завершитись (fn має помітити скасування ctx).
		drainTimeout := 5 * time.Second
		select {
		case <-errCh:
			// Горутина завершилась — витоку немає.
			e.logger.Debug().Msg("task goroutine finished after context cancellation")
		case <-time.After(drainTimeout):
			// Горутина не завершилась — потенційний витік.
			e.logger.Warn().
				Dur("drain_timeout", drainTimeout).
				Msg("task goroutine did not finish after context cancellation — potential goroutine leak")
		}
	}

	elapsed := time.Since(start)

	// --- 5. Оновлення метрик ---
	if execErr != nil {
		atomic.AddInt64(&e.totalFailed, 1)
		e.logger.Warn().
			Err(execErr).
			Dur("elapsed", elapsed).
			Msg("задача завершилася з помилкою")
		return execErr
	}

	atomic.AddInt64(&e.totalProcessed, 1)
	e.logger.Debug().
		Dur("elapsed", elapsed).
		Msg("задача завершилася успішно")

	return nil
}

// ActiveTasks повертає поточну кількість активних задач.
func (e *Executor) ActiveTasks() int {
	return int(atomic.LoadInt64(&e.activeTasks))
}

// Stats повертає поточні метрики виконавця:
//   - active     — кількість задач, що виконуються зараз;
//   - processed  — загальна кількість успішно оброблених задач;
//   - failed     — загальна кількість невдалих задач.
func (e *Executor) Stats() (active int, processed, failed int64) {
	active = int(atomic.LoadInt64(&e.activeTasks))
	processed = atomic.LoadInt64(&e.totalProcessed)
	failed = atomic.LoadInt64(&e.totalFailed)
	return
}

// Concurrency повертає максимальну кількість одночасних задач (розмір семафора).
func (e *Executor) Concurrency() int {
	return int(atomic.LoadInt64(&e.limit))
}

// MaxConcurrency повертає максимальну ємність семафора.
func (e *Executor) MaxConcurrency() int {
	return cap(e.semaphore)
}

// SetConcurrency змінює поточний ліміт конкурентності.
// Якщо потрібно зменшити, метод може блокуватися до звільнення слотів.
func (e *Executor) SetConcurrency(newLimit int) int {
	if newLimit < 1 {
		newLimit = 1
	}

	maxC := cap(e.semaphore)
	if newLimit > maxC {
		newLimit = maxC
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	cur := int(atomic.LoadInt64(&e.limit))
	if newLimit == cur {
		return cur
	}

	if newLimit > cur {
		for i := 0; i < newLimit-cur; i++ {
			e.semaphore <- struct{}{}
		}
	} else {
		for i := 0; i < cur-newLimit; i++ {
			<-e.semaphore
		}
	}

	atomic.StoreInt64(&e.limit, int64(newLimit))
	return newLimit
}
