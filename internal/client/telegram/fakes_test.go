package telegram

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type fakeAPI struct {
	mu       sync.Mutex
	sent     []*bot.SendMessageParams
	edited   []*bot.EditMessageTextParams
	answered []*bot.AnswerCallbackQueryParams
	commands []*bot.SetMyCommandsParams
	descs    []*bot.SetMyDescriptionParams
	shorts   []*bot.SetMyShortDescriptionParams
	gotChats []*bot.GetChatParams
	// chatIDs is the id GetChat finds for a chat name; other names are not
	// found.
	chatIDs map[string]int64
	// chatInfos is what GetChat returns for a chat id or name, as printed by
	// fmt.Sprint; other chats are not found.
	chatInfos map[string]models.ChatFullInfo
	// getChatErr is returned by GetChat for a chat id or name, as printed by
	// fmt.Sprint, when set.
	getChatErr map[string]error
	// members is the status GetChatMember returns for a user, whatever the
	// chat; other users are plain members. memberErr is returned instead,
	// when set.
	members   map[int64]models.ChatMemberType
	memberErr error
	asked     []*bot.GetChatMemberParams
	// sendErr returns an error for a given chat id, when set.
	sendErr map[any]error
	// editErr is returned by every EditMessageText, when set.
	editErr error
	// setupErr is returned by the profile calls, when set.
	setupErr error
}

func (f *fakeAPI) SendMessage(_ context.Context, params *bot.SendMessageParams) (*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.sent = append(f.sent, params)
	if err := f.sendErr[params.ChatID]; err != nil {
		return nil, err
	}

	return &models.Message{}, nil
}

func (f *fakeAPI) EditMessageText(_ context.Context, params *bot.EditMessageTextParams) (*models.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.edited = append(f.edited, params)
	if f.editErr != nil {
		return nil, f.editErr
	}

	return &models.Message{}, nil
}

func (f *fakeAPI) AnswerCallbackQuery(_ context.Context, params *bot.AnswerCallbackQueryParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.answered = append(f.answered, params)
	return true, nil
}

func (f *fakeAPI) SetMyCommands(_ context.Context, params *bot.SetMyCommandsParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.commands = append(f.commands, params)
	return f.setupErr == nil, f.setupErr
}

func (f *fakeAPI) SetMyDescription(_ context.Context, params *bot.SetMyDescriptionParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.descs = append(f.descs, params)
	return f.setupErr == nil, f.setupErr
}

func (f *fakeAPI) SetMyShortDescription(_ context.Context, params *bot.SetMyShortDescriptionParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.shorts = append(f.shorts, params)
	return f.setupErr == nil, f.setupErr
}

func (f *fakeAPI) GetChat(_ context.Context, params *bot.GetChatParams) (*models.ChatFullInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.gotChats = append(f.gotChats, params)
	key := fmt.Sprint(params.ChatID)
	if err := f.getChatErr[key]; err != nil {
		return nil, err
	}
	if info, ok := f.chatInfos[key]; ok {
		return &info, nil
	}
	name, _ := params.ChatID.(string)
	if id, ok := f.chatIDs[name]; ok {
		return &models.ChatFullInfo{ID: id}, nil
	}

	return nil, fmt.Errorf("%w, Bad Request: chat not found", bot.ErrorBadRequest)
}

func (f *fakeAPI) GetChatMember(_ context.Context, params *bot.GetChatMemberParams) (*models.ChatMember, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.asked = append(f.asked, params)
	if f.memberErr != nil {
		return nil, f.memberErr
	}
	status, ok := f.members[params.UserID]
	if !ok {
		status = models.ChatMemberTypeMember
	}

	return &models.ChatMember{Type: status}, nil
}

func (f *fakeAPI) recipients() []any {
	var out []any
	for _, p := range f.sent {
		out = append(out, p.ChatID)
	}

	return out
}

// fakeSubscribers keeps the delivery time per chat and the last day each
// chat got the post, like the real store.
type fakeSubscribers struct {
	times     map[int64]int
	optedOut  map[int64]bool
	delivered map[string]string
	languages map[int64]subscribers.Language
	greeted   map[int64]bool
	failWrite error
}

// subscribed returns a fake with these chats at the default time. They have
// had their greeting already, so /start shows only the menu.
func subscribed(chats ...int64) *fakeSubscribers {
	f := &fakeSubscribers{}
	for _, chatID := range chats {
		f.set(chatID, subscribers.DefaultTime)
		_ = f.MarkGreeted(chatID)
	}

	return f
}

// greet marks chats as greeted, as after their first /start. It returns f.
func (f *fakeSubscribers) greet(chats ...int64) *fakeSubscribers {
	for _, chatID := range chats {
		_ = f.MarkGreeted(chatID)
	}

	return f
}

func (f *fakeSubscribers) set(chatID int64, at int) {
	if f.times == nil {
		f.times = map[int64]int{}
	}
	f.times[chatID] = at
}

// chats returns the subscribed chat ids in ascending order.
func (f *fakeSubscribers) chats() []int64 {
	var out []int64
	for chatID := range f.times {
		out = append(out, chatID)
	}
	slices.Sort(out)

	return out
}

func (f *fakeSubscribers) Add(chatID int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	if _, ok := f.times[chatID]; ok {
		return false, nil
	}
	f.set(chatID, subscribers.DefaultTime)
	delete(f.optedOut, chatID)

	return true, nil
}

func (f *fakeSubscribers) Meet(chatID int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	if _, ok := f.times[chatID]; ok || f.optedOut[chatID] {
		return false, nil
	}
	f.set(chatID, subscribers.DefaultTime)

	return true, nil
}

func (f *fakeSubscribers) Drop(chatID int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	if _, ok := f.times[chatID]; !ok {
		return false, nil
	}
	delete(f.times, chatID)

	return true, nil
}

func (f *fakeSubscribers) Migrate(from, to int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	at, subscribed := f.times[from]
	if !subscribed && !f.optedOut[from] {
		return false, nil
	}
	delete(f.times, from)
	delete(f.optedOut, from)
	delete(f.times, to)
	delete(f.optedOut, to)
	if subscribed {
		f.set(to, at)
	} else {
		f.optedOut[to] = true
	}

	return true, nil
}

func (f *fakeSubscribers) Remove(chatID int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	if f.optedOut == nil {
		f.optedOut = map[int64]bool{}
	}
	f.optedOut[chatID] = true
	if _, ok := f.times[chatID]; !ok {
		return false, nil
	}
	delete(f.times, chatID)

	return true, nil
}

func (f *fakeSubscribers) Time(chatID int64) (int, bool) {
	at, ok := f.times[chatID]

	return at, ok
}

func (f *fakeSubscribers) SetTime(chatID int64, at int) error {
	if at < subscribers.MinTime || at > subscribers.MaxTime || at%subscribers.MinuteStep != 0 {
		return subscribers.ErrInvalidTime
	}
	if _, ok := f.times[chatID]; !ok {
		return subscribers.ErrNotSubscribed
	}
	if f.failWrite != nil {
		return f.failWrite
	}
	f.times[chatID] = at

	return nil
}

func (f *fakeSubscribers) Due(now int, day string) []int64 {
	var out []int64
	for chatID, at := range f.times {
		if at <= now && !f.Delivered(strconv.FormatInt(chatID, 10), day) {
			out = append(out, chatID)
		}
	}
	slices.Sort(out)

	return out
}

func (f *fakeSubscribers) Known() []int64 {
	out := f.chats()
	for chatID := range f.optedOut {
		out = append(out, chatID)
	}
	slices.Sort(out)

	return out
}

func (f *fakeSubscribers) Language(chatID int64) subscribers.Language {
	if lang, ok := f.languages[chatID]; ok {
		return lang
	}

	return subscribers.Russian
}

func (f *fakeSubscribers) SetLanguage(chatID int64, lang subscribers.Language) error {
	if f.failWrite != nil {
		return f.failWrite
	}
	if f.languages == nil {
		f.languages = map[int64]subscribers.Language{}
	}
	f.languages[chatID] = lang

	return nil
}

func (f *fakeSubscribers) Greeted(chatID int64) bool {
	return f.greeted[chatID]
}

func (f *fakeSubscribers) MarkGreeted(chatID int64) error {
	if f.greeted == nil {
		f.greeted = map[int64]bool{}
	}
	f.greeted[chatID] = true

	return nil
}

func (f *fakeSubscribers) LastDelivered(chat string) string {
	return f.delivered[chat]
}

func (f *fakeSubscribers) Delivered(chat, day string) bool {
	return f.delivered[chat] == day
}

func (f *fakeSubscribers) MarkDelivered(chat, day string) error {
	if f.delivered == nil {
		f.delivered = map[string]string{}
	}
	f.delivered[chat] = day

	return f.failWrite
}

// callbackUpdate is a button press on menu message 77 of the chat.
func callbackUpdate(chatID int64, data string) *models.Update {
	return &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "cb-1",
		Data: data,
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{ID: 77, Chat: models.Chat{ID: chatID}},
		},
	}}
}

func startUpdate(chatID int64) *models.Update {
	return &models.Update{Message: &models.Message{Chat: models.Chat{ID: chatID}, Text: "/start"}}
}

var errBlocked = fmt.Errorf("%w, Forbidden: bot was blocked by the user", bot.ErrorForbidden)

// myChatMemberUpdate reports the bot's own membership of a chat changing.
func myChatMemberUpdate(chatID int64, chatType models.ChatType, from, to models.ChatMemberType) *models.Update {
	return &models.Update{MyChatMember: &models.ChatMemberUpdated{
		Chat:          models.Chat{ID: chatID, Type: chatType},
		OldChatMember: models.ChatMember{Type: from},
		NewChatMember: models.ChatMember{Type: to},
	}}
}
