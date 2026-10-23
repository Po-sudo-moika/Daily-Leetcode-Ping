package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/labstack/gommon/log"
)

// maxMessageRunes keeps a message under Telegram's limit of 4096 characters,
// with room to spare.
const maxMessageRunes = 4000

// HandleChats lists every chat the bot knows to its owner, and answers nobody
// else. The list goes to the owner's private chat, whichever chat the command
// came from, so a group never sees it. It takes the subscribed chats, the ones
// that pressed Unsubscribe and the fixed chats from config.toml. A subscribed
// chat the bot was removed from is forgotten and is not listed, unless it is a
// fixed chat. A chat that pressed Unsubscribe stays listed, as unsubscribed,
// even after the bot is removed, because the store keeps that choice.
func (m *Menu) HandleChats(ctx context.Context, _ *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || m.ownerID == 0 || msg.From == nil || msg.From.ID != m.ownerID {
		return
	}

	entries := m.knownChats()
	for _, text := range splitMessages(chatsHeader(len(entries)), m.describeChats(ctx, entries)) {
		// The private chat of a user has the id of the user.
		if _, err := m.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: m.ownerID, Text: text}); err != nil {
			log.Errorf("error while sending the chat list to the owner: %v", err)
			return
		}
	}
}

// knownChat is a chat the bot knows. ref is what Telegram is asked about: the
// id, or the @name of a fixed chat that could not be resolved. key is how the
// store records its deliveries.
type knownChat struct {
	ref   any
	key   string
	id    int64
	named bool
	fixed bool
}

// knownChats returns the fixed chats in the order of config.toml, then the
// other chats of the store in ascending order. A fixed chat that is also in the
// store is listed once.
func (m *Menu) knownChats() []knownChat {
	var chats []knownChat
	seen := map[string]bool{}

	for _, chat := range m.fixedChats {
		if seen[chat] {
			continue
		}
		seen[chat] = true

		if id, err := strconv.ParseInt(chat, 10, 64); err == nil {
			chats = append(chats, knownChat{ref: id, key: chat, id: id, fixed: true})
		} else {
			chats = append(chats, knownChat{ref: chat, key: chat, named: true, fixed: true})
		}
	}

	for _, id := range m.subscribers.Known() {
		key := strconv.FormatInt(id, 10)
		if !seen[key] {
			seen[key] = true
			chats = append(chats, knownChat{ref: id, key: key, id: id})
		}
	}

	return chats
}

func chatsHeader(n int) string {
	if n == 0 {
		return "Чатов пока нет."
	}

	return fmt.Sprintf("Чаты бота: %d", n)
}

// describeChats asks Telegram about each chat and describes it in a few lines.
func (m *Menu) describeChats(ctx context.Context, chats []knownChat) []string {
	blocks := make([]string, 0, len(chats))
	for i, chat := range chats {
		blocks = append(blocks, fmt.Sprintf("%d. %s\n   %s", i+1, m.chatTitle(ctx, chat), m.chatState(chat)))
	}

	return blocks
}

// chatTitle is the name and type of the chat as Telegram tells them, or a
// note when Telegram cannot find it, with the id or name to tell the chat by.
func (m *Menu) chatTitle(ctx context.Context, chat knownChat) string {
	info, err := m.api.GetChat(ctx, &bot.GetChatParams{ChatID: chat.ref})
	switch {
	case err == nil:
		return fmt.Sprintf("%s — %s (%s)", chatName(info), chatTypeName(info.Type), chat.key)
	case chatUnreachable(err):
		return fmt.Sprintf("⚠️ Telegram не нашёл этот чат (%s)", chat.key)
	default:
		log.Errorf("error while asking Telegram about a chat: %v", err)
		return fmt.Sprintf("❓ Не удалось спросить Telegram про чат (%s)", chat.key)
	}
}

// chatState is the delivery time, the subscription and the last post day.
func (m *Menu) chatState(chat knownChat) string {
	var parts []string

	at, subscribed := 0, false
	if !chat.named {
		at, subscribed = m.subscribers.Time(chat.id)
	}

	switch {
	case subscribed:
		parts = append(parts, "время "+subscribers.Format(at)+" МСК", "подписан")
	case chat.fixed:
		parts = append(parts, "время "+subscribers.Format(fixedChatsTime)+" МСК", "подписки нет")
	default:
		parts = append(parts, "время —", "отписан")
	}

	if chat.fixed {
		parts = append(parts, "чат из config.toml")
	}

	last := m.subscribers.LastDelivered(chat.key)
	if last == "" {
		last = "—"
	}
	parts = append(parts, "последний пост: "+last)

	return strings.Join(parts, " · ")
}

func chatName(info *models.ChatFullInfo) string {
	var name string
	switch {
	case info.Title != "":
		name = info.Title
	default:
		name = strings.TrimSpace(info.FirstName + " " + info.LastName)
	}

	if info.Username != "" {
		if name == "" {
			return "@" + info.Username
		}
		return name + " @" + info.Username
	}
	if name == "" {
		return "без названия"
	}

	return name
}

func chatTypeName(t models.ChatType) string {
	switch t {
	case models.ChatTypePrivate:
		return "личный чат"
	case models.ChatTypeGroup:
		return "группа"
	case models.ChatTypeSupergroup:
		return "супергруппа"
	case models.ChatTypeChannel:
		return "канал"
	}

	return string(t)
}

// splitMessages joins the header and the blocks into messages of at most
// maxMessageRunes characters, never cutting a block in two.
func splitMessages(header string, blocks []string) []string {
	var messages []string
	current := header
	for _, block := range blocks {
		if utf8.RuneCountInString(current)+2+utf8.RuneCountInString(block) > maxMessageRunes {
			messages = append(messages, current)
			current = block
			continue
		}
		current += "\n\n" + block
	}

	return append(messages, current)
}
