package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/actions"
	"github.com/BoSuY0/tetracore-worker/internal/domain"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/config"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/observability"
	"github.com/BoSuY0/tetracore-worker/internal/infrastructure/redis"
	"github.com/BoSuY0/tetracore-worker/internal/transport/hub"
	"github.com/BoSuY0/tetracore-worker/internal/worker/adaptive"
)

// ---------------------------------------------------------------------------
// Deps — зовнішні залежності Worker (альтернативний конструктор).
// ---------------------------------------------------------------------------

// Deps містить усі зовнішні залежності, необхідні для створення Worker.
// Поля, позначені як optional, можуть бути nil — Worker працюватиме без них.
type Deps struct {
	HubClient   *hub.Client            // з'єднання з Hub (WebSocket); optional — якщо nil, Worker створить сам
	Registry    *actions.Registry      // реєстр дій (actions)
	RedisClient *redis.Client          // Redis-клієнт для дій
	Logger      zerolog.Logger         // структурований логер
	Metrics     *observability.Metrics // Prometheus-метрики (optional, може бути nil)
}

// ---------------------------------------------------------------------------
// Worker — серце оркестраційного шару TetraCore.
// ---------------------------------------------------------------------------

// Worker — головна структура оркестраційного шару.
// Зв'язує Hub-транспорт, реєстр дій, виконавець задач та метрики.
//
// Основний потік:
//  1. main.go створює Worker через New().
//  2. Викликає Run(ctx) — блокуючий метод, що працює до скасування ctx.
//  3. Run() створює Hub client, підключається, реєструє callback-и, запускає фонові горутини.
//  4. При отриманні задачі від Hub — виконує через Executor і Registry.
//  5. При скасуванні ctx — виконує graceful shutdown.
type Worker struct {
	config       config.Config                 // повна конфігурація (Hub, Worker, Redis тощо)
	hubClient    *hub.Client                   // WebSocket-клієнт до Hub
	registry     *actions.Registry             // реєстр дій
	redisClient  *redis.Client                 // Redis-клієнт
	executor     *Executor                     // конкурентно-обмежений виконавець
	logger       zerolog.Logger                // логер з контекстом worker_id
	metrics      *observability.Metrics        // Prometheus-метрики (може бути nil)
	adaptiveEWMA *adaptive.EWMAMetrics         // EWMA-метрики для adaptive concurrency
	taskMetrics  adaptive.TaskMetricsCollector // додатковий колектор метрик задач (optional)

	startTime   time.Time          // час запуску (для uptime)
	state       domain.WorkerState // поточний стан воркера
	mu          sync.RWMutex       // захист state, cancelFunc
	cancelFunc  context.CancelFunc // скасування основного контексту
	wg          sync.WaitGroup     // очікування завершення фонових горутин
	runCtx      context.Context    // контекст життєвого циклу для виконання задач
	taskCancels sync.Map           // map[string]context.CancelFunc для скасування задач
}

// ---------------------------------------------------------------------------
// Конструктори.
// ---------------------------------------------------------------------------

// New створює новий Worker з наданою конфігурацією та залежностями.
//
// Сигнатура узгоджена з main.go: New(cfg, registry, redisClient, logger).
// Hub client створюється у Run() при першому підключенні.
func New(cfg *config.Config, registry *actions.Registry, redisClient *redis.Client, logger zerolog.Logger, metrics *observability.Metrics) (*Worker, error) {
	if cfg == nil {
		return nil, errors.New("конфігурація не може бути nil")
	}
	if registry == nil {
		return nil, errors.New("реєстр дій не може бути nil")
	}

	workerLogger := logger.With().
		Str("component", "worker").
		Str("worker_id", cfg.Worker.ID).
		Logger()

	concurrency := cfg.Worker.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	maxConcurrency := concurrency
	if cfg.Worker.AdaptiveConcurrency && cfg.Worker.AdaptiveMaxConcurrency > maxConcurrency {
		maxConcurrency = cfg.Worker.AdaptiveMaxConcurrency
	}

	executor := NewExecutorWithMax(concurrency, maxConcurrency, workerLogger)

	w := &Worker{
		config:      *cfg,
		registry:    registry,
		redisClient: redisClient,
		executor:    executor,
		logger:      workerLogger,
		metrics:     metrics,
		state:       domain.WorkerStateIdle,
	}

	return w, nil
}

// NewWithDeps створює Worker з явно переданими залежностями (Deps).
// Альтернативний конструктор для випадків, коли Hub client вже створений зовні.
func NewWithDeps(cfg *config.Config, deps Deps) (*Worker, error) {
	if cfg == nil {
		return nil, errors.New("конфігурація не може бути nil")
	}
	if deps.Registry == nil {
		return nil, errors.New("реєстр дій не може бути nil")
	}

	workerLogger := deps.Logger.With().
		Str("component", "worker").
		Str("worker_id", cfg.Worker.ID).
		Logger()

	concurrency := cfg.Worker.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	maxConcurrency := concurrency
	if cfg.Worker.AdaptiveConcurrency && cfg.Worker.AdaptiveMaxConcurrency > maxConcurrency {
		maxConcurrency = cfg.Worker.AdaptiveMaxConcurrency
	}

	executor := NewExecutorWithMax(concurrency, maxConcurrency, workerLogger)

	w := &Worker{
		config:      *cfg,
		hubClient:   deps.HubClient,
		registry:    deps.Registry,
		redisClient: deps.RedisClient,
		executor:    executor,
		logger:      workerLogger,
		metrics:     deps.Metrics,
		state:       domain.WorkerStateIdle,
	}

	return w, nil
}

// ---------------------------------------------------------------------------
// Run — основний блокуючий метод.
// ---------------------------------------------------------------------------

// Run запускає Worker і блокує до скасування ctx або фатальної помилки.
//
// Послідовність:
//  1. Встановлює стан Idle, фіксує час запуску.
//  2. Створює Hub client (якщо не було передано через Deps) з callback-ами.
//  3. Підключається до Hub.
//  4. Запускає фонові горутини: heartbeat, reconnect manager.
//  5. Блокується до скасування ctx.
//  6. Виконує graceful shutdown.
func (w *Worker) Run(ctx context.Context) error {
	w.startTime = time.Now()
	w.setState(domain.WorkerStateIdle)

	// Створюємо дочірній контекст для можливості зупинки зсередини.
	runCtx, cancel := context.WithCancel(ctx)
	w.mu.Lock()
	w.cancelFunc = cancel
	w.runCtx = runCtx
	w.mu.Unlock()
	defer cancel()

	if w.config.Worker.AdaptiveConcurrency {
		w.initAdaptiveMetrics()
	}

	w.logger.Info().
		Int("concurrency", w.executor.Concurrency()).
		Str("hub_url", w.config.Hub.URL).
		Msg("worker запускається")

	// --- Створення Hub client (якщо не був переданий) ---
	if w.hubClient == nil {
		w.hubClient = w.createHubClient()
	}

	// --- Підключення до Hub ---
	connectCtx, connectCancel := context.WithTimeout(runCtx, time.Duration(w.config.Hub.ConnectTimeout)*time.Second)
	err := w.hubClient.Connect(connectCtx)
	connectCancel()

	if err != nil {
		w.setState(domain.WorkerStateError)
		w.logger.Error().Err(err).Msg("не вдалося підключитися до Hub")
		return fmt.Errorf("підключення до Hub: %w", err)
	}

	w.logger.Info().Msg("підключено до Hub")
	w.updateHubMetric(true)

	// --- Фонові горутини ---
	w.wg.Add(1)
	go w.backgroundHeartbeat(runCtx)

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		rcfg := hub.ReconnectConfigFromHubConfig(w.hubClient.Config(), w.config.Hub.MaxReconnectFailures)
		if err := w.hubClient.BackgroundReconnectManager(runCtx, rcfg); err != nil {
			w.logger.Error().Err(err).Msg("reconnect manager завершився з помилкою")
			// Якщо досягнуто ліміт — зупиняємо worker.
			cancel()
		}
	}()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.hubClient.PingLoop(runCtx)
	}()

	if w.config.Worker.AdaptiveConcurrency {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			w.runAdaptiveConcurrency(runCtx)
		}()
	}

	w.logger.Info().Msg("worker запущено — очікування задач")

	// --- Блокування до скасування ---
	<-runCtx.Done()

	w.logger.Info().Msg("отримано сигнал зупинки — починаємо graceful shutdown")

	// --- Graceful shutdown ---
	w.performShutdown()

	return nil
}

// ---------------------------------------------------------------------------
// handleTask — обробка вхідного завдання від Hub.
// ---------------------------------------------------------------------------

// handleTask обробляє задачу, отриману від Hub через callback OnTaskAssigned.
// Виконується у окремій горутині (запускається з callback-а).
//
// Послідовність:
//  1. Перевіряє, чи worker не зупиняється.
//  2. Витягує ActionParams з повідомлення (через domain.TaskMessage.ExtractActionParams).
//  3. Перевіряє наявність дії у реєстрі.
//  4. Виконує через Executor з таймаутом.
//  5. Формує TaskResult (успіх/помилка).
//  6. Надсилає результат на Hub через hubClient.SendTaskResult().
//  7. Оновлює метрики та стан.
func (w *Worker) handleTask(msg *hub.TaskAssignmentMessage) {
	if msg == nil {
		w.logger.Error().Msg("отримано nil TaskAssignmentMessage")
		return
	}

	taskLogger := w.logger.With().
		Str("task_id", msg.TaskID).
		Str("task_type", msg.TaskType).
		Logger()

	taskLogger.Info().Msg("отримано завдання від Hub")

	// --- Перевірка стану ---
	if w.getState() == domain.WorkerStateShutdown {
		taskLogger.Warn().Msg("worker зупиняється — відхиляємо завдання")
		w.sendTaskError(msg.TaskID, domain.TaskStatusFailed, "worker_shutting_down", domain.ErrWorkerShuttingDown.Error())
		return
	}

	// Оновлюємо стан.
	w.updateWorkerState()

	// --- 1. Витягуємо ActionParams ---
	taskMsg := w.toTaskMessage(msg)
	actionParams := taskMsg.ExtractActionParams()

	if actionParams.Action == "" {
		taskLogger.Error().Msg("завдання не містить дії (action)")
		w.sendTaskError(msg.TaskID, domain.TaskStatusFailed, "missing_action", "завдання не містить поля action")
		return
	}

	taskLogger = taskLogger.With().Str("action", actionParams.Action).Logger()

	// --- 2. Перевіряємо наявність дії у реєстрі ---
	if !w.registry.Has(actionParams.Action) {
		taskLogger.Error().Msg("невідома дія")
		w.sendTaskError(msg.TaskID, domain.TaskStatusFailed, "unknown_action",
			fmt.Sprintf("дія %q не зареєстрована", actionParams.Action))
		return
	}

	// --- 3. Виконуємо через Executor ---
	timeout := msg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second // таймаут за замовчуванням
	}

	start := time.Now()

	var actionResult domain.ActionResult
	var execErr error

	taskCtx, taskCancel := context.WithCancel(w.runCtx)
	defer taskCancel()

	// Зберігаємо cancel-функцію для можливості скасування через handleTaskCancel.
	w.taskCancels.Store(msg.TaskID, taskCancel)
	defer w.taskCancels.Delete(msg.TaskID)

	execErr = w.executor.Execute(taskCtx, func(ctx context.Context) error {
		var err error
		actionResult, err = w.registry.Execute(ctx, actionParams.Action, actionParams)
		return err
	}, timeout)

	elapsed := time.Since(start)

	// --- 4. Формуємо TaskResult ---
	taskResult := domain.TaskResult{
		TaskID:        msg.TaskID,
		ExecutionTime: elapsed,
	}

	if execErr != nil {
		// Системна помилка (таймаут, скасування, або помилка дії).
		taskResult.Status = domain.TaskStatusFailed

		errType := "execution_error"
		if errors.Is(execErr, context.DeadlineExceeded) {
			taskResult.Status = domain.TaskStatusTimeout
			errType = "timeout"
		} else if errors.Is(execErr, context.Canceled) {
			taskResult.Status = domain.TaskStatusCancelled
			errType = "cancelled"
		}

		taskResult.Error = &domain.TaskError{
			Type:    errType,
			Message: execErr.Error(),
		}

		taskLogger.Error().
			Err(execErr).
			Dur("elapsed", elapsed).
			Str("error_type", errType).
			Msg("задача завершилася з помилкою")

		w.recordTaskMetrics(actionParams.Action, false, errType, elapsed)
		w.recordAdaptiveMetrics(elapsed, false)

	} else if !actionResult.Success {
		// Дія виконалась без системної помилки, але повернула бізнес-помилку.
		taskResult.Status = domain.TaskStatusFailed
		taskResult.Result = actionResult.Data

		errCode := "action_error"
		errMsg := "дія повернула помилку"
		if actionResult.Error != nil {
			errCode = actionResult.Error.Code
			errMsg = actionResult.Error.Message
		}

		taskResult.Error = &domain.TaskError{
			Type:    errCode,
			Message: errMsg,
		}

		taskLogger.Warn().
			Str("error_code", errCode).
			Dur("elapsed", elapsed).
			Msg("дія повернула бізнес-помилку")

		w.recordTaskMetrics(actionParams.Action, false, errCode, elapsed)
		w.recordAdaptiveMetrics(elapsed, false)

	} else {
		// Успішне виконання.
		taskResult.Status = domain.TaskStatusCompleted
		taskResult.Result = actionResult.Data

		taskLogger.Info().
			Dur("elapsed", elapsed).
			Msg("задача виконана успішно")

		w.recordTaskMetrics(actionParams.Action, true, "", elapsed)
		w.recordAdaptiveMetrics(elapsed, true)
	}

	// --- 5. Надсилаємо результат на Hub ---
	w.sendTaskResult(taskResult)

	// --- 6. Оновлюємо стан ---
	w.updateWorkerState()
}

// handleTaskCancel обробляє скасування задачі від Hub.
// Наразі логує подію. При масштабуванні — можна додати відстеження
// активних задач з їхніми cancel-функціями для точкового скасування.
func (w *Worker) handleTaskCancel(msg *hub.TaskCancelMessage) {
	if msg == nil {
		return
	}

	w.logger.Info().
		Str("task_id", msg.TaskID).
		Str("reason", msg.Reason).
		Msg("отримано скасування задачі від Hub")

	// Скасовуємо контекст задачі, якщо вона ще виконується.
	if cancel, ok := w.taskCancels.LoadAndDelete(msg.TaskID); ok {
		cancel.(context.CancelFunc)()
		w.logger.Info().
			Str("task_id", msg.TaskID).
			Msg("задачу скасовано")
	} else {
		w.logger.Debug().
			Str("task_id", msg.TaskID).
			Msg("задача для скасування не знайдена (можливо, вже завершена)")
	}
}

// handleHubError обробляє повідомлення про помилку від Hub.
func (w *Worker) handleHubError(msg *hub.ErrorMessage) {
	if msg == nil {
		return
	}
	w.logger.Error().
		Str("code", msg.Code).
		Str("message", msg.Message).
		Interface("details", msg.Details).
		Msg("отримано помилку від Hub")
}

// ---------------------------------------------------------------------------
// Stop — явна зупинка Worker.
// ---------------------------------------------------------------------------

// Stop ініціює graceful shutdown Worker з зазначенням причини.
//
// Послідовність:
//  1. Встановлює стан Shutdown.
//  2. Скасовує контекст (що розблоковує Run).
//  3. performShutdown буде викликано з Run після розблокування.
func (w *Worker) Stop(reason string) {
	w.logger.Info().
		Str("reason", reason).
		Msg("ініціюється зупинка worker")

	w.setState(domain.WorkerStateShutdown)

	// Скасовуємо контекст — Run() розблокується і викличе performShutdown.
	w.mu.RLock()
	cancel := w.cancelFunc
	w.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
}

// performShutdown виконує фактичну процедуру зупинки:
// чекає фонові горутини, дренує активні задачі, відключається від Hub.
func (w *Worker) performShutdown() {
	w.setState(domain.WorkerStateShutdown)

	const drainTimeout = 30 * time.Second

	// --- 1. Чекаємо завершення фонових горутин ---
	bgDone := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(bgDone)
	}()

	select {
	case <-bgDone:
		w.logger.Info().Msg("усі фонові горутини завершились")
	case <-time.After(drainTimeout):
		w.logger.Warn().
			Dur("timeout", drainTimeout).
			Msg("таймаут очікування завершення фонових горутин")
	}

	// --- 2. Чекаємо завершення активних задач ---
	if active := w.executor.ActiveTasks(); active > 0 {
		w.logger.Info().
			Int("active_tasks", active).
			Msg("очікуємо завершення активних задач")

		deadline := time.After(drainTimeout)
		for w.executor.ActiveTasks() > 0 {
			select {
			case <-deadline:
				w.logger.Warn().
					Int("remaining", w.executor.ActiveTasks()).
					Msg("таймаут: деякі задачі не завершились вчасно")
				goto afterDrain
			case <-time.After(500 * time.Millisecond):
				// Перевіряємо знову.
			}
		}
		w.logger.Info().Msg("усі активні задачі завершились")
	}

afterDrain:
	// --- 3. Відключаємося від Hub ---
	if w.hubClient != nil {
		w.hubClient.Disconnect() // void — не повертає помилку
		w.logger.Info().Msg("відключено від Hub")
		w.updateHubMetric(false)
	}

	// --- 4. Фінальна статистика ---
	w.logFinalStats()
}

// ---------------------------------------------------------------------------
// GetStats — поточна статистика Worker.
// ---------------------------------------------------------------------------

// GetStats повертає поточну статистику Worker у форматі domain.WorkerStats.
func (w *Worker) GetStats() domain.WorkerStats {
	active, processed, failed := w.executor.Stats()

	hubConnected := false
	if w.hubClient != nil {
		hubConnected = w.hubClient.IsConnected()
	}

	return domain.WorkerStats{
		WorkerID:       w.config.Worker.ID,
		State:          w.getState(),
		ActiveTasks:    active,
		TotalProcessed: processed,
		TotalFailed:    failed,
		UptimeSeconds:  int64(time.Since(w.startTime).Seconds()),
		HubConnected:   hubConnected,
		StartedAt:      w.startTime,
	}
}

// ---------------------------------------------------------------------------
// Фонові горутини.
// ---------------------------------------------------------------------------

// backgroundHeartbeat періодично логує статистику та надсилає статус на Hub.
//
// Інтервал визначається конфігурацією Worker.HeartbeatInterval (секунди).
// Кожні reportCycles ітерацій (300 при 1с інтервалі = ~5 хв) виводить
// детальний звіт у лог.
func (w *Worker) backgroundHeartbeat(ctx context.Context) {
	defer w.wg.Done()

	interval := time.Duration(w.config.Worker.HeartbeatInterval) * time.Second
	if interval <= 0 {
		interval = 1 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	const reportCycles = 300
	cycle := 0

	for {
		select {
		case <-ctx.Done():
			w.logger.Debug().Msg("heartbeat горутина зупинена")
			return

		case <-ticker.C:
			cycle++

			// Оновлюємо Prometheus gauges.
			if w.metrics != nil {
				w.metrics.WorkerUptimeSeconds.Set(time.Since(w.startTime).Seconds())
				w.metrics.ActiveTasks.Set(float64(w.executor.ActiveTasks()))
			}

			// Надсилаємо статус на Hub (якщо підключено).
			if w.hubClient != nil && w.hubClient.IsConnected() {
				stats := w.GetStats()
				if err := w.hubClient.SendWorkerStatus(stats); err != nil {
					w.logger.Warn().Err(err).Msg("не вдалося надіслати статус на Hub")
				}
			}

			// Періодичний детальний звіт.
			if cycle%reportCycles == 0 {
				stats := w.GetStats()
				w.logger.Info().
					Str("state", string(stats.State)).
					Int("active_tasks", stats.ActiveTasks).
					Int64("total_processed", stats.TotalProcessed).
					Int64("total_failed", stats.TotalFailed).
					Int64("uptime_sec", stats.UptimeSeconds).
					Bool("hub_connected", stats.HubConnected).
					Msg("worker heartbeat report")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Допоміжні методи — створення Hub client.
// ---------------------------------------------------------------------------

// createHubClient створює hub.Client з callback-ами для обробки
// вхідних повідомлень від Hub.
func (w *Worker) createHubClient() *hub.Client {
	hubCfg := hub.HubConfig{
		URL:                w.config.Hub.URL,
		AuthToken:          w.config.Hub.AuthToken,
		ClientID:           w.config.Worker.ID,
		ClientName:         "tetracore-worker-" + w.config.Worker.ID,
		PingInterval:       time.Duration(w.config.Hub.PingInterval) * time.Second,
		PingTimeout:        time.Duration(w.config.Hub.PingTimeout) * time.Second,
		ReconnectDelay:     time.Duration(w.config.Hub.ReconnectDelay) * time.Second,
		MaxReconnectDelay:  time.Duration(w.config.Hub.MaxReconnectDelay) * time.Second,
		ConnectTimeout:     time.Duration(w.config.Hub.ConnectTimeout) * time.Second,
		Concurrency:        w.config.Worker.Concurrency,
		SupportedTaskTypes: w.registry.ListActions(),
	}

	callbacks := hub.Callbacks{
		OnTaskAssigned: func(msg *hub.TaskAssignmentMessage) {
			// Запускаємо обробку задачі в окремій горутині,
			// щоб не блокувати readLoop Hub client.
			// Відстежуємо в WaitGroup для коректного graceful shutdown.
			w.wg.Add(1)
			go func() {
				defer w.wg.Done()
				w.handleTask(msg)
			}()
		},
		OnTaskCancelled: func(msg *hub.TaskCancelMessage) {
			go w.handleTaskCancel(msg)
		},
		OnError: func(msg *hub.ErrorMessage) {
			w.handleHubError(msg)
		},
	}

	client := hub.NewClient(hubCfg, callbacks, w.logger)

	w.logger.Info().
		Str("hub_url", hubCfg.URL).
		Str("client_id", hubCfg.ClientID).
		Msg("Hub client створено")

	return client
}

// ---------------------------------------------------------------------------
// Допоміжні методи — відправка результатів.
// ---------------------------------------------------------------------------

// sendTaskResult надсилає результат задачі на Hub.
func (w *Worker) sendTaskResult(result domain.TaskResult) {
	if w.hubClient == nil {
		w.logger.Debug().
			Str("task_id", result.TaskID).
			Str("status", string(result.Status)).
			Msg("Hub client не налаштований — результат задачі не надіслано")
		return
	}

	if err := w.hubClient.SendTaskResult(result); err != nil {
		w.logger.Error().
			Err(err).
			Str("task_id", result.TaskID).
			Str("status", string(result.Status)).
			Msg("не вдалося надіслати результат задачі на Hub")
	}
}

// sendTaskError формує та надсилає помилковий результат задачі на Hub.
func (w *Worker) sendTaskError(taskID string, status domain.TaskStatus, errType, errMsg string) {
	result := domain.TaskResult{
		TaskID: taskID,
		Status: status,
		Error: &domain.TaskError{
			Type:    errType,
			Message: errMsg,
		},
	}
	w.sendTaskResult(result)
}

// ---------------------------------------------------------------------------
// Допоміжні методи — конвертація повідомлень.
// ---------------------------------------------------------------------------

// toTaskMessage перетворює hub.TaskAssignmentMessage на domain.TaskMessage
// для використання ExtractActionParams().
func (w *Worker) toTaskMessage(msg *hub.TaskAssignmentMessage) domain.TaskMessage {
	return domain.TaskMessage{
		TaskID:       msg.TaskID,
		TaskType:     msg.TaskType,
		ExecutorType: msg.ExecutorType,
		Priority:     domain.TaskPriority(msg.Priority),
		Payload:      msg.Payload,
		Timeout:      msg.Timeout,
		RetryCount:   msg.RetryCount,
	}
}

// ---------------------------------------------------------------------------
// Допоміжні методи — стан Worker.
// ---------------------------------------------------------------------------

// setState безпечно змінює стан Worker.
func (w *Worker) setState(state domain.WorkerState) {
	w.mu.Lock()
	w.state = state
	w.mu.Unlock()
}

// getState безпечно повертає поточний стан Worker.
func (w *Worker) getState() domain.WorkerState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.state
}

// updateWorkerState обчислює та встановлює стан Worker на основі
// кількості активних задач відносно concurrency.
//
// Стани: Idle (0 задач), Busy (є задачі, є вільні слоти), Overloaded (усі слоти зайняті).
// Стан Shutdown не змінюється.
func (w *Worker) updateWorkerState() {
	if w.getState() == domain.WorkerStateShutdown {
		return
	}

	active := w.executor.ActiveTasks()
	concurrency := w.executor.Concurrency()

	switch {
	case active == 0:
		w.setState(domain.WorkerStateIdle)
	case active >= concurrency:
		w.setState(domain.WorkerStateOverloaded)
	default:
		w.setState(domain.WorkerStateBusy)
	}
}

// ---------------------------------------------------------------------------
// Допоміжні методи — метрики.
// ---------------------------------------------------------------------------

// updateHubMetric оновлює Prometheus gauge стану з'єднання з Hub.
func (w *Worker) updateHubMetric(connected bool) {
	if w.metrics == nil {
		return
	}
	if connected {
		w.metrics.HubConnected.Set(1)
	} else {
		w.metrics.HubConnected.Set(0)
	}
}

// recordTaskMetrics оновлює Prometheus-метрики після виконання задачі.
func (w *Worker) recordTaskMetrics(action string, success bool, errCode string, duration time.Duration) {
	if w.metrics == nil {
		return
	}

	w.metrics.TaskDurationSeconds.WithLabelValues(action).Observe(duration.Seconds())

	if success {
		w.metrics.TasksProcessedTotal.WithLabelValues(action).Inc()
	} else {
		if errCode == "" {
			errCode = "unknown"
		}
		w.metrics.TasksFailedTotal.WithLabelValues(action, errCode).Inc()
	}
}

// logFinalStats виводить фінальну статистику при зупинці Worker.
func (w *Worker) logFinalStats() {
	stats := w.GetStats()
	w.logger.Info().
		Int64("total_processed", stats.TotalProcessed).
		Int64("total_failed", stats.TotalFailed).
		Int64("uptime_sec", stats.UptimeSeconds).
		Int("remaining_active", stats.ActiveTasks).
		Msg("worker зупинено — фінальна статистика")
}
