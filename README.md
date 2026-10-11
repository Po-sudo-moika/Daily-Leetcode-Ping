# Daily-Leetcode-Ping
Little service for making post in telegram with description about daily task ob Leetcode.com

The bot posts the LeetCode daily problem once at start-up and then every day at
11:00 (Europe/Moscow). Anyone can open the bot, send `/start` and use the menu:

- **Подписаться / Отписаться** — start or stop getting the daily post.
- **О проекте** — short description and a link to this repository.

The daily post goes to every subscriber and to the chats listed in
`telegram.telegramChats` in `config.toml`. The start-up post goes to those fixed
chats only, so a restart does not send subscribers a second copy.
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
