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

func (f *fakeAPI) recipients() []any {
	var out []any
	for _, p := range f.sent {
		out = append(out, p.ChatID)
	}

	return out
}

// fakeSubscribers keeps the delivery hour per chat and the last day each
// chat got the post, like the real store.
type fakeSubscribers struct {
	hours     map[int64]int
	delivered map[string]string
	failWrite error
}

// subscribed returns a fake with these chats at the default hour.
func subscribed(chats ...int64) *fakeSubscribers {
	f := &fakeSubscribers{}
	for _, chatID := range chats {
		f.set(chatID, subscribers.DefaultHour)
	}

	return f
}

func (f *fakeSubscribers) set(chatID int64, hour int) {
	if f.hours == nil {
		f.hours = map[int64]int{}
	}
	f.hours[chatID] = hour
}

// chats returns the subscribed chat ids in ascending order.
func (f *fakeSubscribers) chats() []int64 {
	var out []int64
	for chatID := range f.hours {
		out = append(out, chatID)
	}
	slices.Sort(out)

	return out
}

func (f *fakeSubscribers) Add(chatID int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	if _, ok := f.hours[chatID]; ok {
		return false, nil
	}
	f.set(chatID, subscribers.DefaultHour)

	return true, nil
}

func (f *fakeSubscribers) Remove(chatID int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	if _, ok := f.hours[chatID]; !ok {
		return false, nil
	}
	delete(f.hours, chatID)

	return true, nil
}

func (f *fakeSubscribers) Hour(chatID int64) (int, bool) {
	hour, ok := f.hours[chatID]

	return hour, ok
}

func (f *fakeSubscribers) SetHour(chatID int64, hour int) error {
	if hour < subscribers.MinHour || hour > subscribers.MaxHour {
		return subscribers.ErrInvalidHour
	}
	if _, ok := f.hours[chatID]; !ok {
		return subscribers.ErrNotSubscribed
	}
	if f.failWrite != nil {
		return f.failWrite
	}
	f.hours[chatID] = hour

	return nil
}

func (f *fakeSubscribers) Due(hour int, day string) []int64 {
	var out []int64
	for chatID, h := range f.hours {
		if h <= hour && !f.Delivered(strconv.FormatInt(chatID, 10), day) {
			out = append(out, chatID)
		}
	}
	slices.Sort(out)

	return out
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
