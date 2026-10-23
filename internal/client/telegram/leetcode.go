package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/client/leetcode"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/domain"
	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/labstack/gommon/log"
)

const (
	callbackLeetcode = "leetcode"
	callbackUnlink   = "unlink"
)

// The texts of linking a LeetCode username and of /show, in each language of
// a chat.
var (
	privateOnlyText = message{
		ru: "Привязать LeetCode можно только в личном чате с ботом. Напиши мне туда: /leetcode <имя>.",
		en: "Linking LeetCode works only in a private chat with the bot. Write to me there: /leetcode <username>.",
	}

	leetcodeFailedText = message{
		ru: "LeetCode сейчас не отвечает. Попробуй ещё раз чуть позже.",
		en: "LeetCode is not answering now. Please try again a little later.",
	}

	noLoginHintText = message{
		ru: "У тебя в Telegram нет @логина, поэтому /show тебя не найдёт. Задай его в настройках Telegram: бот подхватит его, когда ты напишешь ему в следующий раз.",
		en: "You have no @login in Telegram, so /show can't find you. Set one in the Telegram settings: the bot picks it up the next time you write to it.",
	}

	showUsageText = message{
		ru: "Как пользоваться: /show @логин — покажу серию человека, который привязал LeetCode к боту.",
		en: "How to use it: /show @login — I show the streak of a person who linked LeetCode with the bot.",
	}

	leetcodeButton = message{ru: "🔗 LeetCode", en: "🔗 LeetCode"}
	unlinkButton   = message{ru: "🗑 Убрать привязку", en: "🗑 Remove the link"}

	unlinkedText = message{ru: "🗑 Привязка убрана.", en: "🗑 The link is removed."}

	notPrivateScreenText = message{
		ru: "🔗 LeetCode\n\nПривязать LeetCode можно только в личном чате с ботом.",
		en: "🔗 LeetCode\n\nLinking LeetCode works only in a private chat with the bot.",
	}
)

func linkedText(name string, lang subscribers.Language) string {
	return fmt.Sprintf(message{
		ru: "✅ Готово: твой LeetCode — %s. Теперь любой, кто знает твой @логин в Telegram, может посмотреть твою серию командой /show.",
		en: "✅ Done: your LeetCode is %s. Now anyone who knows your Telegram @login can see your streak with /show.",
	}.in(lang), name)
}

func unknownUserText(name string, lang subscribers.Language) string {
	return fmt.Sprintf(message{
		ru: "LeetCode не знает пользователя «%s». Ничего не сохранено. Проверь имя и попробуй ещё раз.",
		en: "LeetCode does not know a user “%s”. Nothing is saved. Check the name and try again.",
	}.in(lang), name)
}

func notLinkedText(login string, lang subscribers.Language) string {
	return fmt.Sprintf(message{
		ru: "У @%s в боте нет привязанного LeetCode.",
		en: "@%s has not linked LeetCode with the bot.",
	}.in(lang), login)
}

func showText(leetcode string, current, best int, lang subscribers.Language) string {
	return fmt.Sprintf(message{
		ru: "🔥 LeetCode: %s\n\nТекущая серия: %s\nЛучшая серия: %s",
		en: "🔥 LeetCode: %s\n\nCurrent streak: %s\nBest streak: %s",
	}.in(lang), leetcode, dayCount(current, lang), dayCount(best, lang))
}

// dayCount is "1 day", "2 days" and so on, in lang.
func dayCount(n int, lang subscribers.Language) string {
	if lang == subscribers.English {
		if n == 1 {
			return "1 day"
		}

		return fmt.Sprintf("%d days", n)
	}

	word := "дней"
	switch {
	case n%100 >= 11 && n%100 <= 14:
	case n%10 == 1:
		word = "день"
	case n%10 >= 2 && n%10 <= 4:
		word = "дня"
	}

	return fmt.Sprintf("%d %s", n, word)
}

// leetcodeScreenText says which LeetCode username the person linked, or how
// to link one, and who can see the streak.
func leetcodeScreenText(link string, linked bool, lang subscribers.Language) string {
	if linked {
		return fmt.Sprintf(message{
			ru: "🔗 LeetCode\n\nПривязан: %s\n\nЧтобы сменить имя, отправь /leetcode <имя>. " +
				"Любой, кто знает твой @логин в Telegram, видит твою серию командой /show.",
			en: "🔗 LeetCode\n\nLinked: %s\n\nTo change it, send /leetcode <username>. " +
				"Anyone who knows your Telegram @login can see your streak with /show.",
		}.in(lang), link)
	}

	return message{
		ru: "🔗 LeetCode\n\nLeetCode не привязан. Отправь /leetcode <имя> (или ссылку на профиль), чтобы привязать. " +
			"Хранится только имя, а не пароль или токен. После этого любой, кто знает твой @логин в Telegram, " +
			"сможет посмотреть твою серию командой /show.",
		en: "🔗 LeetCode\n\nNo LeetCode is linked. Send /leetcode <username> (or a profile link) to link one. " +
			"Only the username is kept, never a password or token. After that, anyone who knows your Telegram @login " +
			"can see your streak with /show.",
	}.in(lang)
}

// leetcodeUsername reads a LeetCode username from what the person wrote: the
// name itself, or a profile link such as https://leetcode.com/u/name/.
func leetcodeUsername(arg string) (string, bool) {
	name := arg
	if strings.Contains(arg, "/") {
		raw := arg
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Hostname() != "leetcode.com" && u.Hostname() != "www.leetcode.com") {
			return "", false
		}

		parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
		if len(parts) == 2 && parts[0] == "u" {
			parts = parts[1:]
		}
		if len(parts) != 1 {
			return "", false
		}
		name = parts[0]
	}

	return name, leetcode.ValidUsername(name)
}

// WithLeetCode turns on linking a LeetCode username and /show: links keeps the
// usernames and leetcode reads the public profile. It returns the menu.
func (m *Menu) WithLeetCode(links Links, leetcode LeetCode) *Menu {
	m.links, m.leetcode = links, leetcode

	return m
}

// WithBotUsername sets the @username of this bot, without the "@", so a
// command written as /settings@<username> is answered. A command addressed to
// any other bot is ignored. It returns the menu.
func (m *Menu) WithBotUsername(username string) *Menu {
	m.botUsername = username

	return m
}

// isPrivate reports whether the chat is a private chat with a person, which
// has the id of that person; groups and channels have negative ids.
func isPrivate(chatID int64) bool {
	return chatID > 0
}

// HandleLeetcode links the LeetCode username in the message to the sender, or
// shows what is linked when there is none. It works only in a private chat.
func (m *Menu) HandleLeetcode(ctx context.Context, _ *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil || msg.From == nil {
		return
	}

	chatID := msg.Chat.ID
	lang := m.subscribers.Language(chatID)
	if msg.Chat.Type != models.ChatTypePrivate {
		m.sendText(ctx, chatID, privateOnlyText.in(lang))
		return
	}

	_, args := splitCommand(msg)
	fields := strings.Fields(args)
	if len(fields) == 0 {
		m.send(ctx, chatID, m.leetcodeScreen(msg.From.ID, lang, ""))
		return
	}

	name, ok := leetcodeUsername(fields[0])
	if !ok {
		m.sendText(ctx, chatID, unknownUserText(fields[0], lang))
		return
	}

	if !m.allowLookup(ctx, chatID, msg.From.ID, lang) {
		return
	}

	calendar, err := m.leetcode.GetUserCalendar(ctx, name)
	switch {
	case errors.Is(err, domain.ErrUserNotFound):
		m.sendText(ctx, chatID, unknownUserText(name, lang))
		return
	case err != nil:
		log.Errorf("error while checking LeetCode user %q: %v", name, err)
		m.sendText(ctx, chatID, leetcodeFailedText.in(lang))
		return
	}

	// Keep the spelling LeetCode uses.
	if calendar.Username != "" {
		name = calendar.Username
	}
	if err := m.links.Set(msg.From.ID, msg.From.Username, name); err != nil {
		log.Errorf("error while saving the link of user %d: %v", msg.From.ID, err)
		m.sendText(ctx, chatID, saveFailedText.in(lang))
		return
	}

	text := linkedText(name, lang)
	if msg.From.Username == "" {
		text += "\n\n" + noLoginHintText.in(lang)
	}
	m.sendText(ctx, chatID, text)
}

// HandleShow answers with the LeetCode username, the current streak and the
// best streak of the person who linked under the @login. It works in any chat.
func (m *Menu) HandleShow(ctx context.Context, _ *bot.Bot, update *models.Update) {
	msg := update.Message
	if msg == nil {
		return
	}

	chatID := msg.Chat.ID
	lang := m.subscribers.Language(chatID)

	_, args := splitCommand(msg)
	fields := strings.Fields(args)
	login := ""
	if len(fields) > 0 {
		login = strings.TrimPrefix(fields[0], "@")
	}
	if login == "" {
		m.sendText(ctx, chatID, showUsageText.in(lang))
		return
	}

	link, ok := m.links.ByLogin(login)
	if !ok {
		m.sendText(ctx, chatID, notLinkedText(login, lang))
		return
	}

	asker := chatID
	if msg.From != nil {
		asker = msg.From.ID
	}
	if !m.allowLookup(ctx, chatID, asker, lang) {
		return
	}

	calendar, err := m.leetcode.GetUserCalendar(ctx, link.LeetCode)
	if err != nil {
		log.Errorf("error while reading the LeetCode profile %q: %v", link.LeetCode, err)
		m.sendText(ctx, chatID, leetcodeFailedText.in(lang))
		return
	}

	m.sendText(ctx, chatID, showText(link.LeetCode, calendar.CurrentStreak(m.now()), calendar.BestStreak, lang))
}

// allowLookup reports whether the user may ask LeetCode now, and tells them in
// chatID when not. The limit counts a lookup even when the answer is cached.
func (m *Menu) allowLookup(ctx context.Context, chatID, userID int64, lang subscribers.Language) bool {
	if m.limits.allowLookup(userID) {
		return true
	}

	m.sendText(ctx, chatID, tooManyLookupsText.in(lang))

	return false
}

// HandleLeetcodeScreen opens the LeetCode screen of the private menu.
func (m *Menu) HandleLeetcodeScreen(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, anyone)
	if !ok {
		return
	}

	m.show(ctx, t, m.leetcodeScreen(t.chatID, m.subscribers.Language(t.chatID), ""))
}

// HandleUnlink removes the link of the person who pressed the button.
func (m *Menu) HandleUnlink(ctx context.Context, _ *bot.Bot, update *models.Update) {
	t, ok := m.ackCallback(ctx, update, anyone)
	if !ok {
		return
	}

	lang := m.subscribers.Language(t.chatID)
	if !isPrivate(t.chatID) {
		// A stale or made-up button: links are kept for private chats only.
		m.show(ctx, t, m.leetcodeScreen(t.chatID, lang, ""))
		return
	}

	if _, err := m.links.Remove(t.chatID); err != nil {
		log.Errorf("error while removing the link of user %d: %v", t.chatID, err)
		m.show(ctx, t, m.leetcodeScreen(t.chatID, lang, saveFailedText.in(lang)))
		return
	}

	m.show(ctx, t, m.leetcodeScreen(t.chatID, lang, unlinkedText.in(lang)))
}

// leetcodeScreen shows what the person with this id linked. The id of a private
// chat is the id of the person.
func (m *Menu) leetcodeScreen(userID int64, lang subscribers.Language, note string) screen {
	if !isPrivate(userID) {
		return screen{
			text:   notPrivateScreenText.in(lang),
			markup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{backButton(lang)}}},
		}
	}

	link, linked := m.links.Get(userID)
	rows := [][]models.InlineKeyboardButton{{backButton(lang)}}
	if linked {
		rows = [][]models.InlineKeyboardButton{
			{{Text: unlinkButton.in(lang), CallbackData: callbackUnlink}},
			{backButton(lang)},
		}
	}

	return screen{
		text:   withNote(note, leetcodeScreenText(link.LeetCode, linked, lang)),
		markup: &models.InlineKeyboardMarkup{InlineKeyboard: rows},
	}
}

// withLeetcodeButton adds the button of the LeetCode screen to the main menu
// of a private chat, above the language button, once linking is turned on.
func (m *Menu) withLeetcodeButton(chatID int64, s screen, lang subscribers.Language) screen {
	if m.links == nil || !isPrivate(chatID) {
		return s
	}

	kb := s.markup.InlineKeyboard
	last := len(kb) - 1
	row := []models.InlineKeyboardButton{{Text: leetcodeButton.in(lang), CallbackData: callbackLeetcode}}
	s.markup = &models.InlineKeyboardMarkup{InlineKeyboard: append(append(append([][]models.InlineKeyboardButton{}, kb[:last]...), row), kb[last:]...)}

	return s
}

// mainMenu is mainScreen for a chat: in a private chat it also has the
// LeetCode button.
func (m *Menu) mainMenu(chatID int64, text string, lang subscribers.Language) screen {
	return m.withLeetcodeButton(chatID, mainScreen(text, lang), lang)
}

func (m *Menu) sendText(ctx context.Context, chatID int64, text string) {
	if _, err := m.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text}); err != nil {
		log.Errorf("error while sending message to %d: %v", chatID, err)
	}
}
