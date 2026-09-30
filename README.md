# TetraCore Worker

A Go worker for the TetraCore project. It connects to
[TetraCore Hub](https://github.com/BoSuY0/TetraCore_Hub) over WebSocket, receives tasks,
runs the matching action and sends the result back to the hub.

TetraCore is my learning project. I built it with the help of AI tools.

## How it works

1. The worker opens a WebSocket connection to the hub (`/ws`). It signs the handshake
   with its token (HMAC-SHA256) and registers as a `worker` client.
2. The hub sends a `task` message. The worker takes the action name and its parameters
   from the task data (`action` and `params`).
3. The action runs with a timeout. The worker sends a `task_result` message
   with the result or an error.
4. If the connection drops, the worker reconnects with a growing delay.

The number of tasks that run at the same time is set by `worker.concurrency`.
With `worker.adaptive_concurrency` turned on, the worker changes this number
based on CPU load, or on CPU load together with task latency and errors (AIMD).

For example, this task sent to the hub's `POST /api/v1/tasks` runs the test action:

```json
{
  "task_type": "test_simple",
  "executor_type": "worker",
  "data": { "action": "test_simple", "params": { "message": "hello" } },
  "timeout": 10
}
```

## Actions

| Action | What it does | Uses |
| --- | --- | --- |
| `get_chat_settings` | Reads the settings of a Telegram chat. Keeps them in a Redis cache for 5 minutes | MySQL, Redis |
| `create_group_settings` | Creates or updates the settings of a group | MySQL, Redis |
| `set_group_active_status` | Recalculates whether a group is active: it has an owner and the bot is an admin. `set_group_status` is an old name for the same action | MySQL, Redis |
| `remove_inactive_chats` | Deletes chats that have been inactive for more than 30 days | MySQL |
| `check_subscriptions` | Moves users with expired subscriptions back to the free plan | MySQL |
| `cleanup_inactive_modules` | Turns off modules in inactive chats | MySQL |
| `cleanup_cache` | Deletes chat settings keys in Redis that have no expiry time | Redis |
| `test_simple` | Returns the message it gets. Used to test the setup | — |

The MySQL tables (`chats`, `users`, `chat_modules`) belong to the TetraCore bot.
The worker does not create them.

## Run

You need Go 1.24 or newer, a running hub, Redis and MySQL.

```bash
git clone https://github.com/BoSuY0/TetraCore_Worker.git
cd TetraCore_Worker
cp .env.example .env    # then fill in your values
go run ./cmd/worker -config config/worker.yaml
```

The worker reads `config/worker.yaml`. Values like `${HUB_URL:ws://localhost:8000/ws}`
come from environment variables, with a default after the colon.

| Variable | Meaning |
| --- | --- |
| `HUB_URL` | WebSocket address of the hub, for example `ws://localhost:8000/ws` |
| `AUTH_TOKEN` | Token for the hub, for example an access token from the hub's `/auth/login` |
| `WORKER_ID` | Worker name. If empty, the worker makes one: `worker-<uuid>` |
| `REDIS_URL`, `REDIS_PASSWORD` | Redis connection |
| `MYSQL_DSN` | MySQL connection, for example `user:password@tcp(localhost:3306)/tetracore?parseTime=true` |
| `LOG_LEVEL`, `LOG_FORMAT` | Log level and format (`json` or `console`) |
| `METRICS_ENABLED`, `METRICS_PORT` | Prometheus metrics at `/metrics`, off by default |

Other ways to run it:

```bash
make build           # builds bin/worker
make docker-build    # builds the Docker image tetracore-worker
```

## Tests

```bash
go test ./...
```

## Related repositories

- [TetraCore Hub](https://github.com/BoSuY0/TetraCore_Hub): the hub that sends tasks to this worker
- TetraCore Bot: the Telegram bot client (private for now)
