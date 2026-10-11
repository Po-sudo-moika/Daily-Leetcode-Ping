# Daily-Leetcode-Ping
Little service for making post in telegram with description about daily task ob Leetcode.com

The bot posts the LeetCode daily problem once at start-up and then every day.
Anyone can open the bot, press Start (or send `/start`) and use the menu. The
menu is one message: every button edits it in place, and About and Settings
open as screens inside it, each with a Back button. At start-up the bot also
sets its description and its command list (`/start`, `/settings`).

- **Подписаться / Отписаться** — start or stop getting the daily post.
- **Настройки** — pick the hour of the daily post, in Moscow time. New
  subscribers get 11:00. The hours offered are 04:00–23:00: LeetCode changes
  the problem at 00:00 UTC (03:00 in Moscow), so a post never carries the
  previous day's problem.
- **О проекте** — short description and a link to this repository.

Every full hour the service posts to the subscribers who chose that hour, and
at 11:00 also to the chats listed in `telegram.telegramChats` in `config.toml`.
It asks LeetCode for the problem at most once an hour, and only when someone is
due. If the host sleeps through some hours, the first run after it wakes up
serves every hour of that day it missed, each once. A fixed
chat that is also a subscriber gets the post once a day, at its chosen hour.
The start-up post goes to the fixed chats only, so a restart does not send
subscribers a second copy.
If Telegram says a user blocked the bot, that subscriber is dropped.

## Configuration

### Bot token

The token is read from the `TELEGRAM_BOT_TOKEN` environment variable. If the
variable is not set, the service reads a `.env` file in the working directory:

```
TELEGRAM_BOT_TOKEN=123456:your-token
```

The variable wins over the file. The service stops at start-up with a clear
message when there is no token. `.env` is in `.gitignore` and `.dockerignore`;
never commit it. The token is not part of `config.toml`.

### config.toml

- `telegram.telegramChats` — fixed recipients (chat ids or `@channel` names).
- `storage.subscribersPath` — where subscriptions are saved
  (default `data/subscribers.json`). The `data/` folder is ignored by git.

## Run

```
go run ./cmd
go test ./...
```

## Docker

```
docker build -t leetcode-ping .
docker run --env-file .env -v leetcode-ping-data:/app/data leetcode-ping
```

- `--env-file .env` passes the token into the container (the file itself is not
  copied into the image).
- `-v leetcode-ping-data:/app/data` keeps `data/subscribers.json` in a Docker
  volume, so subscriptions survive when the container is replaced. If you change
  `storage.subscribersPath`, mount the folder of the new path instead.
