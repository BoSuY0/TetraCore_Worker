package hub

import (
	"context"
	"time"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
)

// handleHeartbeat обробляє вхідний heartbeat від Hub:
// записує час останнього пінгу та відповідає pong.
func (c *Client) handleHeartbeat(msg *HeartbeatMessage) {
	c.logger.Debug().
		Str("heartbeat_id", msg.MessageID).
		Msg("heartbeat received, sending pong")

	pong := NewPongMsg(c.config.ClientID)
	if err := c.send(pong); err != nil {
		c.logger.Error().Err(err).Msg("failed to send pong")
		return
	}

	c.stats.LastPongAt.Store(time.Now().Unix())
}

// PingLoop запускає горутину, що періодично перевіряє стан з'єднання:
// якщо останній pong старший за PingTimeout — з'єднання вважається мертвим
// і виконується disconnect.
//
// Hub сам надсилає heartbeat, а клієнт відповідає pong.
// Ця горутина перевіряє, чи Hub не перестав пінгувати.
func (c *Client) PingLoop(ctx context.Context) {
	if c.config.PingInterval <= 0 {
		return
	}

	ticker := time.NewTicker(c.config.PingInterval)
	defer ticker.Stop()

	c.logger.Debug().
		Dur("interval", c.config.PingInterval).
		Dur("timeout", c.config.PingTimeout).
		Msg("ping monitor started")

	for {
		select {
		case <-ctx.Done():
			c.logger.Debug().Msg("ping monitor stopped: context cancelled")
			return

		case <-c.Done():
			c.logger.Debug().Msg("ping monitor stopped: connection closed")
			return

		case <-ticker.C:
			if !c.IsConnected() {
				continue
			}

			lastPong := c.stats.LastPongAt.Load()
			if lastPong == 0 {
				// Ще не було жодного pong — не перевіряємо.
				continue
			}

			elapsed := time.Since(time.Unix(lastPong, 0))
			if elapsed > c.config.PingTimeout {
				c.logger.Warn().
					Dur("elapsed", elapsed).
					Dur("timeout", c.config.PingTimeout).
					Msg("hub heartbeat timeout, disconnecting")
				c.Disconnect()
				return
			}
		}
	}
}

// ReportStatus надсилає WorkerStatusMessage до Hub на основі WorkerStats.
// Це обгортка для SendWorkerStatus, але з додатковим логуванням.
func (c *Client) ReportStatus(stats domain.WorkerStats) {
	if !c.IsConnected() {
		c.logger.Debug().Msg("skip status report: not connected")
		return
	}

	if err := c.SendWorkerStatus(stats); err != nil {
		c.logger.Error().Err(err).Msg("failed to report worker status")
		return
	}

	c.logger.Debug().
		Str("state", string(stats.State)).
		Int("active_tasks", stats.ActiveTasks).
		Msg("worker status reported to hub")
}
