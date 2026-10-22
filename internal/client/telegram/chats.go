package telegram

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/labstack/gommon/log"
)

// AllowedUpdates is what the bot asks Telegram to send. my_chat_member says
// when the bot is added to or removed from a chat; Telegram's default list
// includes it, but asking for it by name keeps the bot from depending on a
// default, or on an older setting Telegram still remembers.
var AllowedUpdates = bot.AllowedUpdates{
	models.AllowedUpdateMessage,
	models.AllowedUpdateCallbackQuery,
	models.AllowedUpdateMyChatMember,
}

// BotOptions are the options that make the bot follow the chats it is in:
// the updates it asks for, and the middleware that subscribes every chat it
// meets for the first time. fixedChats are the chats from config.toml.
func BotOptions(subscribers Subscribers, fixedChats []string) []bot.Option {
	return []bot.Option{
		bot.WithAllowedUpdates(AllowedUpdates),
		bot.WithMiddlewares(meetChats(subscribers, fixedChats)),
	}
}

// meetChats subscribes the chat of every update at the default hour when the
// bot has not seen that chat before: a private chat that pressed Start, or a
// group the bot was in before it learned to follow its chats. A chat that
// pressed Unsubscribe stays out, and so does a fixed chat listed by its
// @name, which already gets the post as a fixed chat. A failed save is logged
// and the update goes on, so the person still gets an answer.
func meetChats(subscribers Subscribers, fixedChats []string) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if chat, ok := updateChat(update, b.ID()); ok && !isFixedByName(fixedChats, chat) {
				met, err := subscribers.Meet(chat.ID)
				switch {
				case err != nil:
					log.Errorf("error while subscribing new chat %d: %v", chat.ID, err)
				case met:
					log.Infof("new chat %d subscribed on first contact", chat.ID)
				}
			}

			next(ctx, b, update)
		}
	}
}

// updateChat returns the chat a message or a button press came from. A
// message that says the bot botID left the chat, or that the group moved to a
// new id, has no chat: the bot is not in that chat any more.
func updateChat(update *models.Update, botID int64) (models.Chat, bool) {
	switch {
	case update.Message != nil:
		msg := update.Message
		if (msg.LeftChatMember != nil && msg.LeftChatMember.ID == botID) || msg.MigrateToChatID != 0 {
			return models.Chat{}, false
		}
		return msg.Chat, true
	case update.CallbackQuery != nil:
		query := update.CallbackQuery
		switch {
		case query.Message.Message != nil:
			return query.Message.Message.Chat, true
		case query.Message.InaccessibleMessage != nil:
			return query.Message.InaccessibleMessage.Chat, true
		}
	}

	return models.Chat{}, false
}

// isFixedByName reports whether chat is listed in fixedChats by its @name.
// Telegram does not care about the case of a name.
func isFixedByName(fixedChats []string, chat models.Chat) bool {
	for _, fixed := range fixedChats {
		if strings.EqualFold(fixed, "@"+chat.Username) {
			return true
		}
	}

	return false
}

// HandleMyChatMember follows the bot's own membership of a chat. When the bot
// becomes a member or an administrator of a group, supergroup or channel, the
// chat is subscribed at the default hour, also if it unsubscribed before:
// adding the bot again is a new decision. A fixed chat listed by its @name is
// left out, as it already gets the post. When the bot is removed, kicked, or
// a user blocks it in a private chat, the chat is unsubscribed.
//
// Changes that keep the bot in the chat (made an administrator, restricted)
// change nothing, so they never undo an Unsubscribe.
func (m *Menu) HandleMyChatMember(_ context.Context, _ *bot.Bot, update *models.Update) {
	change := update.MyChatMember
	if change == nil {
		return
	}

	chatID := change.Chat.ID
	was, now := isMember(change.OldChatMember), isMember(change.NewChatMember)

	switch {
	case now && !was && change.Chat.Type != models.ChatTypePrivate && !isFixedByName(m.fixedChats, change.Chat):
		if _, err := m.subscribers.Add(chatID); err != nil {
			log.Errorf("error while subscribing chat %d that added the bot: %v", chatID, err)
			return
		}
		log.Infof("bot added to chat %d, subscribed", chatID)
	case isGone(change.NewChatMember):
		if dropped, err := m.subscribers.Drop(chatID); err != nil {
			log.Errorf("error while unsubscribing chat %d that removed the bot: %v", chatID, err)
		} else if dropped {
			log.Infof("bot removed from chat %d, unsubscribed", chatID)
		}
	}
}

// isMember reports whether the bot is in the chat. A restricted bot may be in
// it or not; Telegram says which.
func isMember(m models.ChatMember) bool {
	switch m.Type {
	case models.ChatMemberTypeMember, models.ChatMemberTypeAdministrator:
		return true
	case models.ChatMemberTypeRestricted:
		return m.Restricted != nil && m.Restricted.IsMember
	}

	return false
}

func isGone(m models.ChatMember) bool {
	switch m.Type {
	case models.ChatMemberTypeLeft, models.ChatMemberTypeBanned:
		return true
	case models.ChatMemberTypeRestricted:
		return m.Restricted != nil && !m.Restricted.IsMember
	}

	return false
}
