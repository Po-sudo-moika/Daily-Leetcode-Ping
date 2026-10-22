package telegram

import (
	"context"

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
// meets for the first time.
func BotOptions(subscribers Subscribers) []bot.Option {
	return []bot.Option{
		bot.WithAllowedUpdates(AllowedUpdates),
		bot.WithMiddlewares(meetChats(subscribers)),
	}
}

// meetChats subscribes the chat of every update at the default hour when the
// bot has not seen that chat before: a private chat that pressed Start, or a
// group the bot was in before it learned to follow its chats. A chat that
// pressed Unsubscribe stays out. A failed save is logged and the update goes
// on, so the person still gets an answer.
func meetChats(subscribers Subscribers) bot.Middleware {
	return func(next bot.HandlerFunc) bot.HandlerFunc {
		return func(ctx context.Context, b *bot.Bot, update *models.Update) {
			if chatID, ok := updateChat(update); ok {
				met, err := subscribers.Meet(chatID)
				switch {
				case err != nil:
					log.Errorf("error while subscribing new chat %d: %v", chatID, err)
				case met:
					log.Infof("new chat %d subscribed on first contact", chatID)
				}
			}

			next(ctx, b, update)
		}
	}
}

// updateChat returns the chat a message or a button press came from.
func updateChat(update *models.Update) (int64, bool) {
	switch {
	case update.Message != nil:
		return update.Message.Chat.ID, true
	case update.CallbackQuery != nil:
		query := update.CallbackQuery
		switch {
		case query.Message.Message != nil:
			return query.Message.Message.Chat.ID, true
		case query.Message.InaccessibleMessage != nil:
			return query.Message.InaccessibleMessage.Chat.ID, true
		}
	}

	return 0, false
}

// HandleMyChatMember follows the bot's own membership of a chat. When the bot
// becomes a member or an administrator of a group, supergroup or channel, the
// chat is subscribed at the default hour, also if it unsubscribed before:
// adding the bot again is a new decision. When the bot is removed, kicked, or
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
	was, now := isMember(change.OldChatMember.Type), isMember(change.NewChatMember.Type)

	switch {
	case now && !was && change.Chat.Type != models.ChatTypePrivate:
		if _, err := m.subscribers.Add(chatID); err != nil {
			log.Errorf("error while subscribing chat %d that added the bot: %v", chatID, err)
			return
		}
		log.Infof("bot added to chat %d, subscribed", chatID)
	case isGone(change.NewChatMember.Type):
		if dropped, err := m.subscribers.Drop(chatID); err != nil {
			log.Errorf("error while unsubscribing chat %d that removed the bot: %v", chatID, err)
		} else if dropped {
			log.Infof("bot removed from chat %d, unsubscribed", chatID)
		}
	}
}

func isMember(t models.ChatMemberType) bool {
	return t == models.ChatMemberTypeMember || t == models.ChatMemberTypeAdministrator
}

func isGone(t models.ChatMemberType) bool {
	return t == models.ChatMemberTypeLeft || t == models.ChatMemberTypeBanned
}
