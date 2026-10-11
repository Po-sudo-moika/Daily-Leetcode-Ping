package telegram

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type fakeAPI struct {
	mu       sync.Mutex
	sent     []*bot.SendMessageParams
	answered []*bot.AnswerCallbackQueryParams
	// sendErr returns an error for a given chat id, when set.
	sendErr map[any]error
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

func (f *fakeAPI) AnswerCallbackQuery(_ context.Context, params *bot.AnswerCallbackQueryParams) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.answered = append(f.answered, params)
	return true, nil
}

func (f *fakeAPI) recipients() []any {
	var out []any
	for _, p := range f.sent {
		out = append(out, p.ChatID)
	}

	return out
}

type fakeSubscribers struct {
	chats     []int64
	failWrite error
}

func (f *fakeSubscribers) Add(chatID int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	if slices.Contains(f.chats, chatID) {
		return false, nil
	}
	f.chats = append(f.chats, chatID)

	return true, nil
}

func (f *fakeSubscribers) Remove(chatID int64) (bool, error) {
	if f.failWrite != nil {
		return false, f.failWrite
	}
	i := slices.Index(f.chats, chatID)
	if i < 0 {
		return false, nil
	}
	f.chats = slices.Delete(f.chats, i, i+1)

	return true, nil
}

func (f *fakeSubscribers) List() []int64 {
	return slices.Clone(f.chats)
}

func callbackUpdate(chatID int64, data string) *models.Update {
	return &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "cb-1",
		Data: data,
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{Chat: models.Chat{ID: chatID}},
		},
	}}
}

func startUpdate(chatID int64) *models.Update {
	return &models.Update{Message: &models.Message{Chat: models.Chat{ID: chatID}, Text: "/start"}}
}

var errBlocked = fmt.Errorf("%w, Forbidden: bot was blocked by the user", bot.ErrorForbidden)
