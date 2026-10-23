# Security: abuse from Telegram users

The bot uses long polling, so it opens no port. A user can reach it only through
Telegram updates: commands, button presses, and adding the bot to chats. This
page lists what such a user can do to hurt the bot, and the defence for each.
The numbers are constants in `internal/client/telegram/limits.go`,
`internal/client/leetcode/guard.go` and the two stores. They need no config.

Server-level items (SSH, the firewall, OS updates) are outside the bot and are
not covered here.

## Attacks and defences

| # | Attack | Harm | Defence |
|---|--------|------|---------|
| 1 | **Flood of commands or buttons from one user** | Every answer is a Telegram call and a file write. A flood fills memory and slows the daily post. | Token bucket per Telegram user id: a burst of 10, then 20 actions a minute. An extra action is dropped before any work. A dropped button press is still answered, so there is no spinner. The user gets "too many requests, try again in a minute" at most once a minute, in their language. For a button, the notice is the text of the answer, so it costs no extra message. |
| 2 | **Flood from many users or one group** | Many users at once, or many members of one group, send the bot over Telegram's limit for a group (20 messages a minute). | Token bucket per group: a burst of 10, then 15 actions a minute, counted for the whole group, on top of the user limit. Commands for other bots (`/foo@OtherBot`) are not counted. Same drop and same notice. Many accounts can still add up, but each one is limited, and the bot handles only 8 updates at once (see 8). |
| 3 | **Lookup amplification: many `/show` or `/leetcode`** | Each lookup is a call to LeetCode. Too many calls get the server's IP limited or banned, and the lookups for the linking stop working. | A stricter limit per user for the commands that ask LeetCode: a burst of 5, then 5 a minute. Answers are cached per username for 5 minutes, both "user exists" and the calendar, so repeated `/show` for one person makes one call. A global guard lets at most 3 calls run at once and starts at most about 1 a second. A request that would wait more than 3 seconds gets "LeetCode is not answering now, try again a little later". At most about 3 requests wait, so they never pile up. |
| 4 | **Telegram rate limits hurt the daily post** | The bot as a whole may send about 30 messages a second, and 1 a second to one chat. A flood of replies can delay the daily post for everyone. | Defences 1, 2, 3 and 8 cap the replies. The notice is sent at most once a minute to a user. The daily post itself is sent by the cron round, one chat at a time, and a failed chat is tried again in the next hour. There is no way to give the post priority over replies in the Telegram API, so the cap on replies is the defence. |
| 5 | **Oversized or malformed input: long commands** | A very long command wastes memory and parsing time. | A command (a message that starts with `/`) longer than 256 characters is ignored before anything else runs. Telegram itself limits a message to 4096 characters. |
| 6 | **Made-up usernames** | A random text is sent to LeetCode, or used to waste calls. | Before any call, the name is checked: 1 to 40 characters from letters, digits, `_`, `.`, `-`. A bad name gets the normal "unknown user" answer with no call. This check is also in the LeetCode guard, so a name read from the links file is checked too. |
| 7 | **Forged button data** | A button with a wrong hour, minute or language. Telegram clients can press only the buttons the bot sent, so this is mostly an old or stale button. | The handlers parse the data and check its range (hour 4 to 23, a time on the 5-minute step, language `ru` or `en`). A bad value is answered and changes no state: the menu is shown as it really is. Admin-only buttons in a group still check that the user is an admin. |
| 8 | **Unbounded memory or goroutine growth** | The Telegram library starts a new goroutine for every update by default. A flood grows the memory of a 1 GB server. | The bot runs its handlers in 8 worker goroutines, not one goroutine per update. Updates that wait are held in a bounded queue, and then by Telegram, which keeps them. The rate limiters forget users who were idle for 2 minutes. The LeetCode cache keeps at most 1000 names. |
| 9 | **Growth of the stored files** | A flood of new chats or users makes `subscribers.json` and `leetcode-links.json` grow without end, and every save rewrites the whole file. | One link per Telegram user (a new `/leetcode` replaces the old link). A high safety cap of 50,000 known chats, 50,000 languages, 50,000 greetings and 50,000 links. Past it, a new entry is refused and a warning is logged; people already stored are not affected. |
| 10 | **Adding the bot to many chats** | Each new chat is subscribed and gets a post every day, which costs Telegram calls. Many chats can use up the Telegram limit for the daily post. | Only a person can add a bot to a chat, and Telegram limits how fast. The storage cap in 9 stops the growth. A chat that blocks or removes the bot is dropped at the next post. The owner sees all chats with `/chats`. Telegram's own anti-spam is the main defence. The updates that say the bot was added or removed (`my_chat_member`) are not rate limited on purpose: dropping a "removed" update would keep a dead chat on the list. |
| 11 | **Privacy: `/show` shows the streak of anyone who linked** | Anyone who knows the Telegram @login of a person who linked sees their LeetCode username and streak. | This is by design, and the README says it. The bot tells the person so when they link. They can remove the link at any time with the button on the LeetCode screen. Only the public LeetCode username is kept, never a password or token. |
| 12 | **Owner commands from other users (`/chats`)** | A user tries to read the chat list. | `/chats` answers only the owner id. For anyone else the bot says nothing. |

## Known limits

- The cache means `/show` can show a streak that is up to 5 minutes old.
- A flood of lookups from many different accounts can still make others wait up
  to 3 seconds for their turn, then get "try again later". This is by design: it
  protects the server's IP at LeetCode.
- The bot cannot block a user for good. A user over the limit gets their
  actions dropped until the allowance comes back (about 3 seconds per action).
