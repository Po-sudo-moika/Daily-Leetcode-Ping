# Daily-Leetcode-Ping
Little service for making post in telegram with description about daily task ob Leetcode.com

The bot posts the LeetCode daily problem once at start-up and then every day.
Anyone can open the bot, press Start (or send `/start`) and use the menu. The
menu is one message: every button edits it in place, and About and Settings
open as screens inside it, each with a Back button. At start-up the bot also
sets its description and its command list (`/start`, `/settings`, `/leetcode`,
`/show`).

## Who gets the post

Every chat the bot is in gets the same post, at 11:00 Moscow time unless the
chat picks another time with `/settings`. Nobody has to press Subscribe:

- **A group, supergroup or channel the bot is added to** is subscribed at
  once, and unsubscribed when the bot is removed or leaves. Adding the bot
  again subscribes it again.
- **A private chat** is subscribed when the person presses Start.
- **A chat the bot has not seen before** is subscribed on its first update.
  Telegram cannot list the chats a bot is already in, so a group the bot was
  added to before this behaviour existed is picked up when someone writes a
  command or presses a button there (for example `/start`), or through
  `telegram.telegramChats` in `config.toml`.
- **A chat that pressed Unsubscribe** stays out until it presses Subscribe
  again, or the bot is removed and added again. `/start` or any other
  command does not subscribe it. The choice is kept in the subscribers file,
  so it survives a restart.
- **A group that Telegram turns into a supergroup** gets a new chat id. It
  keeps its choice under the new id: its time, or its Unsubscribe, and its
  language.

### Groups: admins only

In a group or supergroup, only the group's admins (the creator or an
administrator) can change the time or the language, subscribe or unsubscribe,
with the menu buttons or from `/settings`. Anyone else who presses one of those buttons
gets a short note that only admins can change it, and nothing changes. If
Telegram cannot say whether the person is an admin, nothing changes either,
and the note asks them to try again. Everyone can still open the screens
(Settings, About, Back). Private chats work as before. A channel has no
members who press buttons, so it has no such rule.

## The menu

- **Подписаться / Отписаться** — start or stop getting the daily post.
- **Настройки** — pick the time of the daily post, in Moscow time: first the
  hour, then the minutes in steps of 5 (`:00`, `:05`, … `:55`), for example
  09:30. New subscribers get 11:00. The times offered are 04:00–23:55:
  LeetCode changes the problem at 00:00 UTC (03:00 in Moscow), so a post never
  carries the previous day's problem. The Back button on the minutes goes back
  to the hours.
- **О проекте** — short description and a link to this repository.
- **🇬🇧 English / 🇷🇺 Русский** — switch the chat to the other language. The
  button is labelled in the language it switches to, and the menu is redrawn in
  place.

## LeetCode streak: `/leetcode` and `/show`

A person can link their LeetCode username to the bot, and then anyone can ask
for their streak.

- **Link** — in a **private chat** with the bot, send `/leetcode <username>`.
  A profile link such as `https://leetcode.com/u/<username>/` works too. The
  bot first asks LeetCode whether it knows the name; if not, it says so and
  saves nothing. `/leetcode` with no name shows the linked username, or how to
  link. The private menu has a **LeetCode** button with the same screen and a
  button that removes the link. In a group, `/leetcode` only says
  that linking works in a private chat.
- **Show** — in **any chat**, `/show @login` answers with the person's LeetCode
  username, their current streak and their best streak. The login is their
  Telegram @login and its case does not matter. If that person has not linked,
  the bot says so; `/show` alone says how to use it; if LeetCode does not
  answer, the bot asks to try again later.
- **How the streak is counted** — from the public submission calendar of the
  LeetCode profile, no login needed. The current streak is the number of UTC
  days in a row, up to today, with at least one submission; a streak that ended
  yesterday still counts until today ends. The best streak is the one LeetCode
  reports for the calendar's period.
- **What is stored** — for each person who linked: their Telegram user id, their
  Telegram @login as last seen and their LeetCode username, in
  `storage.linksPath` (default `data/leetcode-links.json`), saved the same safe
  way as the subscribers file. The @login is refreshed whenever the person
  writes to the bot. The bot never asks for or stores a LeetCode password,
  token or cookie.
- **Privacy** — once a person links, anyone who knows their Telegram @login can
  see their LeetCode username and streak with `/show`. Remove the link with the
  button on the LeetCode screen.

## Commands

`/start`, `/settings`, `/chats`, `/leetcode` and `/show` also work when
written as `/command@<bot username>`, in groups and private chats. A command
addressed to another bot (`/settings@OtherBot`) is ignored. The bot asks
Telegram for its own username at start-up.

## Language and greeting

Each chat has a language, Russian or English, and every chat starts in
Russian. The button at the bottom of the menu switches the chat; the choice is
kept in the subscribers file, so it survives a restart. Everything the bot
sends to that chat follows it: the menu and its screens, the notes (such as
"admins only"), the greeting and the daily post. The `/chats` list for the
owner follows the language of the owner's private chat. A subscribers file from
an earlier version loads with every chat in Russian.

The first time a chat sends `/start` (or presses Start), the bot first sends a
short hello in the chat's language: who it is and what it does. Then it shows
the menu, as before. Each chat gets the hello once, and that is kept in the
subscribers file, so a later `/start` shows only the menu. Chats that used the
bot before the greeting existed get it once on their next `/start`.

The bot's profile (the description shown before Start, and the command list)
is the same for everyone and stays in Russian.

## Delivery

Every minute the service posts to everyone whose time has come and who has
not got today's post yet: subscribers from the time they chose, and the chats
listed in `telegram.telegramChats` in `config.toml` from 11:00. So a post is
never more than a minute late. The subscribers file records the Moscow date
of each chat's last post, so every chat gets the post once a day. The first
run after the host wakes up, or after a restart, catches up on the times it
missed. A send that fails is tried again in the next clock hour, not every
minute. After a change of time, a chat that already got today's post waits for
tomorrow; one that has not gets it at the first minute at or after its new
time. It asks LeetCode for the problem only when someone is due; a failed ask
is tried again the next minute. A fixed chat that is also a
subscriber gets the post once a day, at its chosen time.
The start-up post goes to the fixed chats only, so a restart does not send
subscribers a second copy. Once a fixed chat's delivery time has come (11:00,
or the time it chose if it is also a subscriber), the start-up post counts as
its post for the day, so the next round does not repeat it.
If Telegram says a user blocked the bot, the bot was kicked from a group or
channel, or the chat no longer exists, that chat is unsubscribed. It is
subscribed again when the bot is added again, or on a new `/start` unless it
had pressed Unsubscribe.

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
  At start-up the bot looks up the id of each `@name` once (`getChat`). From
  then on that chat is treated like a fixed chat given by id: if it is also a
  subscriber, it gets one post a day, at the time it chose. A name that
  cannot be looked up is logged and still gets the post by name at 11:00.
  The menu of a fixed chat without a subscription of its own says it gets
  the post every day at 11:00.
- `storage.subscribersPath` — where subscriptions are saved
  (default `data/subscribers.json`). The `data/` folder is ignored by git.
  The file lists the subscribed chats, their chosen hours and minutes, the
  chats that pressed Unsubscribe (`optedOut`), the last post day of each
  chat (kept for 90 days), the chats switched to English (`languages`) and
  the chats that got the hello (`greeted`). A file from an earlier version loads as is: every
  chat keeps its hour, at `:00`.
- `storage.linksPath` — where the linked LeetCode usernames are saved
  (default `data/leetcode-links.json`, in the same ignored `data/` folder).

### Owner and `/chats`

`/chats` lists every chat the bot knows. It answers only the bot's owner: a
Telegram user id you set in local configuration. Anyone else gets no answer,
and with no owner set the command is off. To find your id, ask a bot such as
`@userinfobot`. Then set it in the `TELEGRAM_OWNER_ID` environment variable,
or as `TELEGRAM_OWNER_ID=...` in the `.env` file, next to the bot token (the
Docker `--env-file .env` passes it into the container). The variable wins over
the file. The id is not part of `config.toml`.

Never commit your id: this repository is public.

The list goes to the owner's private chat with the bot (press Start there
first), also when the command is typed in a group. For each chat it shows the
name or title (asked from Telegram when you ask), the type (private chat,
group, supergroup, channel), the delivery time, whether the chat is subscribed,
and the last day it got the post. Listed are the subscribed chats, the chats
that pressed Unsubscribe and the chats from `telegramChats`. A chat Telegram
cannot find, such as a dead fixed chat, is listed with a warning. A subscribed
chat the bot was removed from is forgotten and is not listed, unless it is in
`telegramChats`. A chat that pressed Unsubscribe stays listed, as
unsubscribed, even after the bot is removed, because the bot keeps that
choice. A long list is split across messages.

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
- `-v leetcode-ping-data:/app/data` keeps `data/subscribers.json` and `data/leetcode-links.json` in a Docker
  volume, so subscriptions and links survive when the container is replaced. If you change
  `storage.subscribersPath`, mount the folder of the new path instead.
