# Daily-Leetcode-Ping

A small Go service. It posts the LeetCode daily problem to Telegram chats. It posts once at start-up, then every day: each subscriber at the hour they chose (Europe/Moscow, default 11:00), the fixed chats at 11:00 (hourly cron).

## Build, test, run

- Go 1.25 (see `go.mod`).
- Build: `go build ./...`
- Test: `go test ./...`
- Run: `go run ./cmd/` from the repo root. The service reads `config.toml` from the current directory.
- Docker: the `Dockerfile` builds `./cmd/` into `./service` and runs it.

## Code layout

- `cmd/main.go`: entry point, cron schedule, sends the post.
- `internal/config`: config structs.
- `internal/client/leetcode`: LeetCode API client.
- `internal/client/telegram`: Telegram client and message text.
- `internal/domain`: task types.

## Configuration

`config.toml` holds the service name, version, env, the chat ids (`telegramChats`) and the subscribers file path. The bot token is not in `config.toml`: it comes from the `TELEGRAM_BOT_TOKEN` environment variable or the gitignored `.env` file.

Never commit the real bot token or chat ids. This repo is public. The committed `config.toml` has placeholders only. Keep real values in an uncommitted local copy.

## Agent skills

### Issue tracker

Issues live in GitHub Issues for this repo. See `docs/agents/issue-tracker.md`.

### Triage labels

Default labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.
