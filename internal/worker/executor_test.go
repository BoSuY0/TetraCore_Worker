package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// nopLogger повертає тихий логер для тестів.
func nopLogger() zerolog.Logger {
	return zerolog.Nop()
}

// --------------------------------------------------------------------------
// 1. TestExecutor_BasicExecution
// --------------------------------------------------------------------------

func TestExecutor_BasicExecution(t *testing.T) {
	exec := NewExecutor(5, nopLogger())

	err := exec.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	}, 0)

	if err != nil {
		t.Fatalf("очікувалось nil, отримано: %v", err)
	}

	_, processed, failed := exec.Stats()
	if processed != 1 {
		t.Fatalf("очікувалось processed=1, отримано: %d", processed)
	}
	if failed != 0 {
		t.Fatalf("очікувалось failed=0, отримано: %d", failed)
	}
}

// --------------------------------------------------------------------------
// 2. TestExecutor_ConcurrencyLimit
// --------------------------------------------------------------------------

func TestExecutor_ConcurrencyLimit(t *testing.T) {
	const (
		concurrency = 3
		totalTasks  = 20
	)

	exec := NewExecutor(concurrency, nopLogger())

	var (
		maxConcurrent int64
		current       int64
		wg            sync.WaitGroup
	)

	// Канал, який тримає горутину "працюючою" до команди відпустити.
	gate := make(chan struct{})

	wg.Add(totalTasks)
	for i := 0; i < totalTasks; i++ {
		go func() {
			defer wg.Done()
			_ = exec.Execute(context.Background(), func(ctx context.Context) error {
				c := atomic.AddInt64(&current, 1)
				// Оновлюємо максимум, що бачили.
				for {
					old := atomic.LoadInt64(&maxConcurrent)
					if c <= old {
						break
					}
					if atomic.CompareAndSwapInt64(&maxConcurrent, old, c) {
						break
					}
				}
				// Чекаємо дозволу завершитись.
				<-gate
				atomic.AddInt64(&current, -1)
				return nil
			}, 0)
		}()
	}

	// Даємо час горутинам набратись у семафор.
	time.Sleep(200 * time.Millisecond)

	// Перевіряємо, що одночасно працює не більше concurrency.
	mc := atomic.LoadInt64(&maxConcurrent)
	if mc > int64(concurrency) {
		t.Fatalf("максимальна конкурентність %d перевищує ліміт %d", mc, concurrency)
	}
	if mc == 0 {
		t.Fatal("жодна задача не розпочалась")
	}

	// Відпускаємо всі задачі.
	close(gate)
	wg.Wait()

	_, processed, _ := exec.Stats()
	if processed != int64(totalTasks) {
		t.Fatalf("очікувалось processed=%d, отримано: %d", totalTasks, processed)
	}
}

// --------------------------------------------------------------------------
// 3. TestExecutor_Timeout
// --------------------------------------------------------------------------

func TestExecutor_Timeout(t *testing.T) {
	exec := NewExecutor(5, nopLogger())

	err := exec.Execute(context.Background(), func(ctx context.Context) error {
		// Блокуємо до скасування контексту (таймаут спрацює першим).
		<-ctx.Done()
		return ctx.Err()
	}, 100*time.Millisecond)

	if err == nil {
		t.Fatal("очікувалась помилка, отримано nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("очікувалось context.DeadlineExceeded, отримано: %v", err)
	}

	_, _, failed := exec.Stats()
	if failed != 1 {
		t.Fatalf("очікувалось failed=1, отримано: %d", failed)
	}
}

// --------------------------------------------------------------------------
// 4. TestExecutor_ContextCancellation
// --------------------------------------------------------------------------

func TestExecutor_ContextCancellation(t *testing.T) {
	exec := NewExecutor(5, nopLogger())

	ctx, cancel := context.WithCancel(context.Background())

	// Сигналізуємо, що fn розпочалась.
	started := make(chan struct{})

	go func() {
		<-started
		// Скасовуємо після того, як fn точно працює.
		cancel()
	}()

	err := exec.Execute(ctx, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}, 0)

	if err == nil {
		t.Fatal("очікувалась помилка, отримано nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("очікувалось context.Canceled, отримано: %v", err)
	}
}

// --------------------------------------------------------------------------
// 5. TestExecutor_GoroutineDrain
// --------------------------------------------------------------------------

func TestExecutor_GoroutineDrain(t *testing.T) {
	exec := NewExecutor(5, nopLogger())

	// Підтверджуємо, що горутина fn завершилася після drain.
	drained := make(chan struct{})

	err := exec.Execute(context.Background(), func(ctx context.Context) error {
		<-ctx.Done()
		// Імітуємо cleanup ~100ms.
		time.Sleep(100 * time.Millisecond)
		close(drained)
		return ctx.Err()
	}, 50*time.Millisecond)

	if err == nil {
		t.Fatal("очікувалась помилка таймауту")
	}

	// Drain period — 5 секунд; горутина має завершитися задовго до цього.
	select {
	case <-drained:
		// Горутина завершилась — OK.
	case <-time.After(3 * time.Second):
		t.Fatal("горутина не завершилась протягом drain-періоду")
	}
}

// --------------------------------------------------------------------------
// 6. TestExecutor_FunctionError
// --------------------------------------------------------------------------

func TestExecutor_FunctionError(t *testing.T) {
	exec := NewExecutor(5, nopLogger())

	knownErr := errors.New("тестова помилка")

	err := exec.Execute(context.Background(), func(ctx context.Context) error {
		return knownErr
	}, 0)

	if !errors.Is(err, knownErr) {
		t.Fatalf("очікувалась %v, отримано: %v", knownErr, err)
	}

	_, processed, failed := exec.Stats()
	if failed != 1 {
		t.Fatalf("очікувалось failed=1, отримано: %d", failed)
	}
	if processed != 0 {
		t.Fatalf("очікувалось processed=0, отримано: %d", processed)
	}
}

// --------------------------------------------------------------------------
// 7. TestExecutor_Stats
// --------------------------------------------------------------------------

func TestExecutor_Stats(t *testing.T) {
	exec := NewExecutor(10, nopLogger())

	knownErr := errors.New("fail")

	// 5 успішних.
	for i := 0; i < 5; i++ {
		_ = exec.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		}, 0)
	}
	// 3 невдалих.
	for i := 0; i < 3; i++ {
		_ = exec.Execute(context.Background(), func(ctx context.Context) error {
			return knownErr
		}, 0)
	}

	active, processed, failed := exec.Stats()
	if active != 0 {
		t.Fatalf("очікувалось active=0, отримано: %d", active)
	}
	if processed != 5 {
		t.Fatalf("очікувалось processed=5, отримано: %d", processed)
	}
	if failed != 3 {
		t.Fatalf("очікувалось failed=3, отримано: %d", failed)
	}
}

// --------------------------------------------------------------------------
// 8. TestExecutor_Concurrency
// --------------------------------------------------------------------------

func TestExecutor_Concurrency(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{5, 5},
		{1, 1},
		{100, 100},
		{0, 10},  // за замовчуванням
		{-1, 10}, // за замовчуванням
	}

	for _, tc := range tests {
		exec := NewExecutor(tc.input, nopLogger())
		got := exec.Concurrency()
		if got != tc.expected {
			t.Errorf("NewExecutor(%d).Concurrency() = %d, очікувалось %d",
				tc.input, got, tc.expected)
		}
	}
}

// --------------------------------------------------------------------------
// 9. TestExecutor_ContextCancelledBeforeSemaphore
// --------------------------------------------------------------------------

func TestExecutor_ContextCancelledBeforeSemaphore(t *testing.T) {
	// Створюємо Executor з конкурентністю 1.
	exec := NewExecutor(1, nopLogger())

	// Зайнятий слот: тримаємо семафор.
	blocker := make(chan struct{})
	blockerStarted := make(chan struct{})

	go func() {
		_ = exec.Execute(context.Background(), func(ctx context.Context) error {
			close(blockerStarted)
			<-blocker
			return nil
		}, 0)
	}()

	// Чекаємо, поки blocker зайняв єдиний слот.
	<-blockerStarted

	// Скасований контекст.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Скасовуємо одразу.

	err := exec.Execute(ctx, func(ctx context.Context) error {
		t.Fatal("ця функція не повинна бути виконана")
		return nil
	}, 0)

	if err == nil {
		t.Fatal("очікувалась помилка, отримано nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("очікувалось context.Canceled, отримано: %v", err)
	}

	// Звільняємо blocker.
	close(blocker)
}
