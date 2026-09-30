package worker

// Цей файл містить утилітні функції для керування життєвим циклом Worker-а:
// SetupSignalHandler, SetupSignalHandlerWithLogger та GracefulShutdown.
//
// Ці функції НЕ використовуються у стандартній точці входу (main.go), яка має
// власну реалізацію обробки сигналів. Вони надаються як готові утиліти для
// альтернативних шаблонів інтеграції (наприклад, вбудовування Worker у інший
// сервіс, тестові сценарії або спеціалізовані оркестратори).

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
)

// ---------------------------------------------------------------------------
// SetupSignalHandler — створення контексту, що скасовується при OS-сигналах.
// ---------------------------------------------------------------------------

// SetupSignalHandler створює context.Context, який автоматично скасовується
// при отриманні SIGINT або SIGTERM.
//
// Повертає ctx і cancel-функцію (для ручного скасування, якщо потрібно).
//
// Використання:
//
//	ctx, cancel := worker.SetupSignalHandler()
//	defer cancel()
//	if err := w.Run(ctx); err != nil { ... }
//
// При отриманні другого сигналу — негайний os.Exit(1) (force shutdown).
func SetupSignalHandler() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 2) // буфер 2 — для першого і другого сигналу
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// Перший сигнал — graceful shutdown.
		sig := <-sigCh
		// Логуємо через stderr, бо у цей момент логер може бути недоступний.
		_, _ = os.Stderr.WriteString("отримано сигнал " + sig.String() + " — починаємо graceful shutdown\n")
		cancel()

		// Другий сигнал — force exit.
		sig = <-sigCh
		_, _ = os.Stderr.WriteString("отримано повторний сигнал " + sig.String() + " — примусовий вихід\n")
		os.Exit(1)
	}()

	return ctx, cancel
}

// SetupSignalHandlerWithLogger аналогічний SetupSignalHandler, але використовує
// наданий zerolog.Logger для структурованого логування замість stderr.
func SetupSignalHandlerWithLogger(logger zerolog.Logger) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())

	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		// Перший сигнал — graceful shutdown.
		sig := <-sigCh
		logger.Info().
			Str("signal", sig.String()).
			Msg("отримано сигнал — починаємо graceful shutdown")
		cancel()

		// Другий сигнал — force exit.
		sig = <-sigCh
		logger.Fatal().
			Str("signal", sig.String()).
			Msg("отримано повторний сигнал — примусовий вихід")
	}()

	return ctx, cancel
}

// ---------------------------------------------------------------------------
// GracefulShutdown — координація зупинки Worker з таймаутом.
// ---------------------------------------------------------------------------

// GracefulShutdown виконує graceful shutdown Worker з обмеженням часу.
//
// Послідовність:
//  1. Викликає w.Stop() з причиною "graceful shutdown".
//  2. Чекає завершення або вичерпання timeout.
//  3. Якщо timeout вичерпано — логує попередження та повертає помилку.
//
// Параметри:
//   - w: Worker, який потрібно зупинити.
//   - timeout: максимальний час на graceful shutdown. Якщо <= 0, використовується 30с.
//   - logger: логер для структурованого логування процесу.
//
// Повертає nil при успішному завершенні або помилку при timeout.
func GracefulShutdown(w *Worker, timeout time.Duration, logger zerolog.Logger) error {
	if w == nil {
		logger.Warn().Msg("GracefulShutdown: worker is nil — пропускаємо")
		return nil
	}

	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	logger.Info().
		Dur("timeout", timeout).
		Msg("починаємо graceful shutdown")

	// Запускаємо Stop у окремій горутині, щоб контролювати таймаут.
	done := make(chan struct{})
	go func() {
		w.Stop("graceful shutdown")
		close(done)
	}()

	select {
	case <-done:
		logger.Info().Msg("graceful shutdown завершено успішно")
		return nil

	case <-time.After(timeout):
		logger.Error().
			Dur("timeout", timeout).
			Msg("graceful shutdown перевищив таймаут — примусова зупинка")
		return &ShutdownTimeoutError{Timeout: timeout}
	}
}

// ---------------------------------------------------------------------------
// ShutdownTimeoutError — помилка таймауту graceful shutdown.
// ---------------------------------------------------------------------------

// ShutdownTimeoutError повертається, коли graceful shutdown не завершився
// протягом заданого часу.
type ShutdownTimeoutError struct {
	Timeout time.Duration
}

// Error реалізує інтерфейс error.
func (e *ShutdownTimeoutError) Error() string {
	return "graceful shutdown перевищив таймаут: " + e.Timeout.String()
}
