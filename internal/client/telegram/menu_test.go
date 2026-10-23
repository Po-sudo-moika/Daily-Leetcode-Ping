package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func newMenu(subs *fakeSubscribers) (*Menu, *fakeAPI) {
	api := &fakeAPI{}
	return NewMenu(api, subs, nil), api
}

func buttons(t *testing.T, markup any) []models.InlineKeyboardButton {
	t.Helper()

	kb, ok := markup.(*models.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("reply markup is %T, want *models.InlineKeyboardMarkup", markup)
	}

	var out []models.InlineKeyboardButton
	for _, row := range kb.InlineKeyboard {
		out = append(out, row...)
	}

	return out
}

// callbacks returns the callback data of the buttons, in order.
func callbacks(t *testing.T, markup any) []string {
	t.Helper()

	var out []string
	for _, b := range buttons(t, markup) {
		if b.CallbackData != "" {
			out = append(out, b.CallbackData)
		}
	}

	return out
}

// onlyEdit checks that a button press edited the menu message 77 of chat 10
// once, answered the press, and sent nothing new. It returns the edit.
func onlyEdit(t *testing.T, api *fakeAPI) *bot.EditMessageTextParams {
	t.Helper()

	if len(api.answered) != 1 || api.answered[0].CallbackQueryID != "cb-1" {
		t.Fatalf("callback not answered once: %v", api.answered)
	}
	if len(api.sent) != 0 {
		t.Fatalf("a button press must not send a new message, sent %+v", api.sent)
	}
	if len(api.edited) != 1 {
		t.Fatalf("edited %d times, want 1", len(api.edited))
	}
	edit := api.edited[0]
	if edit.ChatID != int64(10) || edit.MessageID != 77 {
		t.Fatalf("edited chat %v message %d, want chat 10 message 77", edit.ChatID, edit.MessageID)
	}

	return edit
}

// markedHours returns the hours whose button is marked as the current choice.
func markedHours(t *testing.T, markup any) []string {
	t.Helper()

	var out []string
	for _, b := range buttons(t, markup) {
		if strings.HasPrefix(b.CallbackData, callbackHourPrefix) && strings.HasPrefix(b.Text, "✅") {
			out = append(out, b.CallbackData)
		}
	}

	return out
}

func TestHandleStart_ShowsWelcomeAndMenu(t *testing.T) {
	menu, api := newMenu(subscribed())

	menu.HandleStart(context.Background(), nil, startUpdate(10))

	if len(api.sent) != 1 || len(api.edited) != 0 {
		t.Fatalf("sent %d, edited %d; want one new message", len(api.sent), len(api.edited))
	}
	msg := api.sent[0]
	if msg.ChatID != int64(10) {
		t.Fatalf("sent to %v, want 10", msg.ChatID)
	}
	if msg.Text != welcomeFor(0, false) {
		t.Fatalf("text %q, want welcome text", msg.Text)
	}

	got := callbacks(t, msg.ReplyMarkup)
	want := []string{callbackSubscribe, callbackUnsubscribe, callbackSettings, callbackAbout}
	if !slices.Equal(got, want) {
		t.Fatalf("menu buttons %v, want %v", got, want)
	}
}

func TestHandleStart_IgnoresUpdateWithoutMessage(t *testing.T) {
	menu, api := newMenu(subscribed())

	menu.HandleStart(context.Background(), nil, &models.Update{})
	menu.HandleSettingsCommand(context.Background(), nil, &models.Update{})

	if len(api.sent) != 0 {
		t.Fatalf("sent %d messages, want 0", len(api.sent))
	}
}

func TestHandleSubscribe_EditsMenuInPlace(t *testing.T) {
	subs := subscribed()
	menu, api := newMenu(subs)

	menu.HandleSubscribe(context.Background(), nil, callbackUpdate(10, callbackSubscribe))

	if !slices.Equal(subs.chats(), []int64{10}) {
		t.Fatalf("subscribers %v, want [10]", subs.chats())
	}
	edit := onlyEdit(t, api)
	if edit.Text != subscribedText(subscribers.DefaultTime) || !strings.Contains(edit.Text, "11:00") {
		t.Fatalf("unexpected text %q", edit.Text)
	}
	if got := callbacks(t, edit.ReplyMarkup); !slices.Contains(got, callbackSettings) || len(got) != 4 {
		t.Fatalf("edit must keep the menu buttons: %v", got)
	}
}

func TestHandleSubscribe_TellsTheChosenHour(t *testing.T) {
	subs := subscribed(10)
	subs.set(10, subscribers.At(9, 0))
	menu, api := newMenu(subs)

	menu.HandleSubscribe(context.Background(), nil, callbackUpdate(10, callbackSubscribe))

	if got := onlyEdit(t, api).Text; !strings.Contains(got, "09:00") {
		t.Fatalf("text %q must name the chosen hour", got)
	}
	if hour, _ := subs.Time(10); hour != subscribers.At(9, 0) {
		t.Fatalf("subscribing again must keep the chosen hour, got %d", hour)
	}
}

func TestHandleSubscribe_TwiceKeepsOneSubscription(t *testing.T) {
	subs := subscribed()
	menu, api := newMenu(subs)

	menu.HandleSubscribe(context.Background(), nil, callbackUpdate(10, callbackSubscribe))
	menu.HandleSubscribe(context.Background(), nil, callbackUpdate(10, callbackSubscribe))

	if !slices.Equal(subs.chats(), []int64{10}) {
		t.Fatalf("subscribers %v, want [10]", subs.chats())
	}
	if len(api.edited) != 2 || api.edited[0].Text != api.edited[1].Text {
		t.Fatalf("second press must show the same state: %+v", api.edited)
	}
}

func TestHandleUnsubscribe_EditsMenuInPlace(t *testing.T) {
	subs := subscribed(10, 20)
	menu, api := newMenu(subs)

	menu.HandleUnsubscribe(context.Background(), nil, callbackUpdate(10, callbackUnsubscribe))

	if !slices.Equal(subs.chats(), []int64{20}) {
		t.Fatalf("subscribers %v, want [20]", subs.chats())
	}
	edit := onlyEdit(t, api)
	if edit.Text != unsubscribedText {
		t.Fatalf("unexpected text %q", edit.Text)
	}
	if len(buttons(t, edit.ReplyMarkup)) == 0 {
		t.Fatal("edit must keep the menu")
	}
}

func TestHandleUnsubscribe_WhenNotSubscribed(t *testing.T) {
	menu, api := newMenu(subscribed())

	menu.HandleUnsubscribe(context.Background(), nil, callbackUpdate(10, callbackUnsubscribe))

	if edit := onlyEdit(t, api); edit.Text != unsubscribedText {
		t.Fatalf("unexpected text %q", edit.Text)
	}
}

func TestSubscribeUnsubscribe_ReportStoreFailure(t *testing.T) {
	handlers := map[string]func(*Menu) func(context.Context, *models.Update){
		"subscribe": func(m *Menu) func(context.Context, *models.Update) {
			return func(ctx context.Context, u *models.Update) { m.HandleSubscribe(ctx, nil, u) }
		},
		"unsubscribe": func(m *Menu) func(context.Context, *models.Update) {
			return func(ctx context.Context, u *models.Update) { m.HandleUnsubscribe(ctx, nil, u) }
		},
	}

	for name, handler := range handlers {
		t.Run(name, func(t *testing.T) {
			menu, api := newMenu(&fakeSubscribers{failWrite: errors.New("disk full")})

			handler(menu)(context.Background(), callbackUpdate(10, name))

			if edit := onlyEdit(t, api); edit.Text != saveFailedText {
				t.Fatalf("unexpected text %q", edit.Text)
			}
		})
	}
}

func TestHandleAbout_OpensScreenWithBackButton(t *testing.T) {
	subs := subscribed()
	menu, api := newMenu(subs)

	menu.HandleAbout(context.Background(), nil, callbackUpdate(10, callbackAbout))

	edit := onlyEdit(t, api)
	if edit.Text != aboutText || !strings.Contains(aboutText, "LeetCode") {
		t.Fatalf("unexpected about text: %q", edit.Text)
	}

	var urls []string
	for _, b := range buttons(t, edit.ReplyMarkup) {
		if b.URL != "" {
			urls = append(urls, b.URL)
		}
	}
	want := "https://github.com/Po-sudo-moika/Daily-Leetcode-Ping"
	if !slices.Equal(urls, []string{want}) {
		t.Fatalf("link buttons %v, want [%s]", urls, want)
	}
	if got := callbacks(t, edit.ReplyMarkup); !slices.Equal(got, []string{callbackMenu}) {
		t.Fatalf("about must have a Back button, got %v", got)
	}
	if len(subs.chats()) != 0 {
		t.Fatalf("about must not change subscriptions: %v", subs.chats())
	}
}

func TestHandleBack_ReturnsToMenu(t *testing.T) {
	menu, api := newMenu(subscribed())

	menu.HandleBack(context.Background(), nil, callbackUpdate(10, callbackMenu))

	edit := onlyEdit(t, api)
	if edit.Text != welcomeFor(0, false) {
		t.Fatalf("unexpected text %q", edit.Text)
	}
	if got := callbacks(t, edit.ReplyMarkup); !slices.Contains(got, callbackSubscribe) {
		t.Fatalf("Back must show the menu: %v", got)
	}
}

func TestHandleSettings_ShowsHoursWithCurrentChoiceMarked(t *testing.T) {
	subs := subscribed(10)
	menu, api := newMenu(subs)

	menu.HandleSettings(context.Background(), nil, callbackUpdate(10, callbackSettings))

	edit := onlyEdit(t, api)
	if !strings.Contains(edit.Text, "МСК") || !strings.Contains(edit.Text, "11:00") {
		t.Fatalf("settings text must name Moscow time and the current hour: %q", edit.Text)
	}
	if !strings.Contains(edit.Text, "03:00") {
		t.Fatalf("settings text must explain the range: %q", edit.Text)
	}

	var hours []string
	for h := subscribers.MinHour; h <= subscribers.MaxHour; h++ {
		hours = append(hours, fmt.Sprintf("%s%d", callbackHourPrefix, h))
	}
	var got []string
	for _, c := range callbacks(t, edit.ReplyMarkup) {
		if strings.HasPrefix(c, callbackHourPrefix) {
			got = append(got, c)
		}
	}
	if !slices.Equal(got, hours) {
		t.Fatalf("hour buttons %v, want %v", got, hours)
	}
	if marked := markedHours(t, edit.ReplyMarkup); !slices.Equal(marked, []string{callbackHourPrefix + "11"}) {
		t.Fatalf("marked %v, want only 11", marked)
	}
	if !slices.Contains(callbacks(t, edit.ReplyMarkup), callbackMenu) {
		t.Fatal("settings must have a Back button")
	}

	for _, b := range buttons(t, edit.ReplyMarkup) {
		if len(b.CallbackData) > 64 {
			t.Errorf("callback data %q is longer than Telegram allows", b.CallbackData)
		}
	}
}

func TestHandleSettings_MarkedHourShowsTheSavedMinutes(t *testing.T) {
	subs := subscribed()
	subs.set(10, subscribers.At(9, 30))
	menu, api := newMenu(subs)

	menu.HandleSettings(context.Background(), nil, callbackUpdate(10, callbackSettings))

	edit := onlyEdit(t, api)
	var marked []string
	for _, b := range buttons(t, edit.ReplyMarkup) {
		if strings.HasPrefix(b.CallbackData, callbackHourPrefix) && strings.HasPrefix(b.Text, "✅") {
			marked = append(marked, b.CallbackData+" "+b.Text)
		}
	}
	if want := []string{callbackHourPrefix + "9 ✅ 09:30"}; !slices.Equal(marked, want) {
		t.Fatalf("marked hour buttons %v, want %v", marked, want)
	}
}

func TestHandleSettings_NotSubscribed(t *testing.T) {
	menu, api := newMenu(subscribed())

	menu.HandleSettings(context.Background(), nil, callbackUpdate(10, callbackSettings))

	edit := onlyEdit(t, api)
	if edit.Text != settingsNotSubscribedText {
		t.Fatalf("unexpected text %q", edit.Text)
	}
	if got := callbacks(t, edit.ReplyMarkup); !slices.Equal(got, []string{callbackSubscribe, callbackMenu}) {
		t.Fatalf("buttons %v, want subscribe and back", got)
	}
}

func TestHandleSettingsCommand_SendsSettingsScreen(t *testing.T) {
	subs := subscribed(10)
	subs.set(10, subscribers.At(7, 0))
	menu, api := newMenu(subs)

	update := &models.Update{Message: &models.Message{Chat: models.Chat{ID: 10}, Text: "/settings"}}
	menu.HandleSettingsCommand(context.Background(), nil, update)

	if len(api.sent) != 1 || api.sent[0].ChatID != int64(10) {
		t.Fatalf("unexpected messages: %+v", api.sent)
	}
	if marked := markedHours(t, api.sent[0].ReplyMarkup); !slices.Equal(marked, []string{callbackHourPrefix + "7"}) {
		t.Fatalf("marked %v, want only 7", marked)
	}
}

// markedTimes returns the minute buttons whose button is marked as the current
// choice.
func markedTimes(t *testing.T, markup any) []string {
	t.Helper()

	var out []string
	for _, b := range buttons(t, markup) {
		if strings.HasPrefix(b.CallbackData, callbackTimePrefix) && strings.HasPrefix(b.Text, "✅") {
			out = append(out, b.CallbackData)
		}
	}

	return out
}

func timeData(hour, minute int) string {
	return callbackTimePrefix + strconv.Itoa(subscribers.At(hour, minute))
}

func TestHandleHour_OffersTheMinutesAndSavesNothing(t *testing.T) {
	subs := subscribed(10, 20)
	menu, api := newMenu(subs)

	menu.HandleHour(context.Background(), nil, callbackUpdate(10, callbackHourPrefix+"9"))

	if at, _ := subs.Time(10); at != subscribers.DefaultTime {
		t.Fatalf("choosing an hour saved %s, want nothing saved", subscribers.Format(at))
	}
	edit := onlyEdit(t, api)
	if !strings.Contains(edit.Text, "09:__") {
		t.Fatalf("text must name the hour: %q", edit.Text)
	}

	var got, want []string
	for _, b := range buttons(t, edit.ReplyMarkup) {
		if strings.HasPrefix(b.CallbackData, callbackTimePrefix) {
			got = append(got, b.Text+"="+b.CallbackData)
		}
	}
	for minute := 0; minute < 60; minute += 5 {
		want = append(want, fmt.Sprintf("09:%02d=%s", minute, timeData(9, minute)))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("minute buttons %v, want %v", got, want)
	}
	backs := 0
	for _, data := range callbacks(t, edit.ReplyMarkup) {
		if data == callbackSettings {
			backs++
		}
	}
	if backs != 1 {
		t.Fatalf("want one Back button to the hours, got %d", backs)
	}
}

func TestHandleHour_MarksTheCurrentTimeInItsOwnHour(t *testing.T) {
	subs := subscribed()
	subs.set(10, subscribers.At(9, 30))
	menu, api := newMenu(subs)

	menu.HandleHour(context.Background(), nil, callbackUpdate(10, callbackHourPrefix+"9"))
	menu.HandleHour(context.Background(), nil, callbackUpdate(10, callbackHourPrefix+"10"))

	if len(api.edited) != 2 {
		t.Fatalf("edited %d times, want 2", len(api.edited))
	}
	if marked := markedTimes(t, api.edited[0].ReplyMarkup); !slices.Equal(marked, []string{timeData(9, 30)}) {
		t.Fatalf("hour 9: marked %v, want only 09:30", marked)
	}
	if marked := markedTimes(t, api.edited[1].ReplyMarkup); len(marked) != 0 {
		t.Fatalf("hour 10: marked %v, want nothing", marked)
	}
}

func TestHandleTime_SavesAndMovesTheMark(t *testing.T) {
	subs := subscribed(10, 20)
	menu, api := newMenu(subs)

	menu.HandleTime(context.Background(), nil, callbackUpdate(10, timeData(9, 30)))

	if at, _ := subs.Time(10); at != subscribers.At(9, 30) {
		t.Fatalf("saved time %s, want 09:30", subscribers.Format(at))
	}
	if at, _ := subs.Time(20); at != subscribers.DefaultTime {
		t.Fatalf("another subscriber's time changed to %s", subscribers.Format(at))
	}
	edit := onlyEdit(t, api)
	if !strings.Contains(edit.Text, "09:30") || !strings.Contains(edit.Text, "МСК") {
		t.Fatalf("text must confirm the new time in Moscow time: %q", edit.Text)
	}
	if marked := markedHours(t, edit.ReplyMarkup); !slices.Equal(marked, []string{callbackHourPrefix + "9"}) {
		t.Fatalf("marked %v, want only the hour 9", marked)
	}
}

func TestHandleTime_SameTimeAgainIsNotAnError(t *testing.T) {
	subs := subscribed(10)
	menu, api := newMenu(subs)
	// Telegram refuses an edit that changes nothing.
	api.editErr = fmt.Errorf("%w, Bad Request: message is not modified", bot.ErrorBadRequest)

	menu.HandleTime(context.Background(), nil, callbackUpdate(10, timeData(11, 0)))

	if len(api.sent) != 0 {
		t.Fatalf("an unchanged menu must not be re-sent: %+v", api.sent)
	}
}

func TestHandleTime_RejectsBadButtons(t *testing.T) {
	for name, tc := range map[string]struct {
		data string
		subs *fakeSubscribers
		want []int64
	}{
		"before the allowed range": {timeData(3, 55), subscribed(10), []int64{10}},
		"after the allowed range":  {timeData(24, 0), subscribed(10), []int64{10}},
		"off the five minute step": {timeData(9, 7), subscribed(10), []int64{10}},
		"not a number":             {callbackTimePrefix + "noon", subscribed(10), []int64{10}},
		"not subscribed":           {timeData(9, 30), subscribed(), nil},
	} {
		t.Run(name, func(t *testing.T) {
			menu, api := newMenu(tc.subs)

			menu.HandleTime(context.Background(), nil, callbackUpdate(10, tc.data))

			onlyEdit(t, api)
			if got := tc.subs.chats(); !slices.Equal(got, tc.want) {
				t.Fatalf("a bad button changed the subscribers: %v, want %v", got, tc.want)
			}
			if at, found := tc.subs.Time(10); found && at != subscribers.DefaultTime {
				t.Fatalf("time changed to %s", subscribers.Format(at))
			}
		})
	}
}

func TestHandleHour_RejectsBadButtons(t *testing.T) {
	for name, data := range map[string]string{
		"before the allowed range": callbackHourPrefix + "3",
		"after the allowed range":  callbackHourPrefix + "24",
		"not a number":             callbackHourPrefix + "noon",
	} {
		t.Run(name, func(t *testing.T) {
			menu, api := newMenu(subscribed(10))

			menu.HandleHour(context.Background(), nil, callbackUpdate(10, data))

			edit := onlyEdit(t, api)
			if strings.Contains(edit.Text, "__") {
				t.Fatalf("a bad hour must show the settings, not minutes: %q", edit.Text)
			}
		})
	}
}

func TestHandleHour_NotSubscribedShowsTheSettingsScreen(t *testing.T) {
	menu, api := newMenu(subscribed())

	menu.HandleHour(context.Background(), nil, callbackUpdate(10, callbackHourPrefix+"9"))

	if got := onlyEdit(t, api).Text; got != settingsNotSubscribedText {
		t.Fatalf("text %q, want %q", got, settingsNotSubscribedText)
	}
}

func TestHandleTime_ReportsStoreFailure(t *testing.T) {
	subs := subscribed(10)
	subs.failWrite = errors.New("disk full")
	menu, api := newMenu(subs)

	menu.HandleTime(context.Background(), nil, callbackUpdate(10, timeData(9, 30)))

	edit := onlyEdit(t, api)
	if !strings.Contains(edit.Text, saveFailedText) {
		t.Fatalf("text must report the failure: %q", edit.Text)
	}
	if marked := markedHours(t, edit.ReplyMarkup); !slices.Equal(marked, []string{callbackHourPrefix + "11"}) {
		t.Fatalf("the mark must stay on the saved hour, got %v", marked)
	}
}

// A whole visit goes through one message: every press edits message 77.
func TestWholeVisit_NeverSendsANewMessage(t *testing.T) {
	menu, api := newMenu(subscribed())
	ctx := context.Background()

	menu.HandleStart(ctx, nil, startUpdate(10))
	for _, step := range []struct {
		data    string
		handler func(context.Context, *bot.Bot, *models.Update)
	}{
		{callbackSubscribe, menu.HandleSubscribe},
		{callbackSettings, menu.HandleSettings},
		{callbackHourPrefix + "8", menu.HandleHour},
		{timeData(8, 30), menu.HandleTime},
		{callbackMenu, menu.HandleBack},
		{callbackAbout, menu.HandleAbout},
		{callbackMenu, menu.HandleBack},
		{callbackUnsubscribe, menu.HandleUnsubscribe},
	} {
		step.handler(ctx, nil, callbackUpdate(10, step.data))
	}

	if len(api.sent) != 1 {
		t.Fatalf("sent %d messages, want only the one from /start", len(api.sent))
	}
	if len(api.edited) != 8 || len(api.answered) != 8 {
		t.Fatalf("edited %d, answered %d; want 8 each", len(api.edited), len(api.answered))
	}
	for _, e := range api.edited {
		if e.ChatID != int64(10) || e.MessageID != 77 {
			t.Fatalf("edit went to chat %v message %d", e.ChatID, e.MessageID)
		}
	}
}

func TestEditFallback_SendsOneNewMenuWhenMessageCannotBeEdited(t *testing.T) {
	for name, editErr := range map[string]error{
		"deleted":  fmt.Errorf("%w, Bad Request: message to edit not found", bot.ErrorBadRequest),
		"too old":  fmt.Errorf("%w, Bad Request: message can't be edited", bot.ErrorBadRequest),
		"no right": fmt.Errorf("%w, Bad Request: MESSAGE_ID_INVALID", bot.ErrorBadRequest),
	} {
		t.Run(name, func(t *testing.T) {
			subs := subscribed()
			menu, api := newMenu(subs)
			api.editErr = editErr

			menu.HandleSubscribe(context.Background(), nil, callbackUpdate(10, callbackSubscribe))

			if !slices.Equal(subs.chats(), []int64{10}) {
				t.Fatalf("the change must still be saved: %v", subs.chats())
			}
			if len(api.sent) != 1 {
				t.Fatalf("sent %d messages, want one new menu", len(api.sent))
			}
			msg := api.sent[0]
			if msg.ChatID != int64(10) || msg.Text != subscribedText(subscribers.DefaultTime) {
				t.Fatalf("unexpected new menu: %+v", msg)
			}
			if got := callbacks(t, msg.ReplyMarkup); !slices.Contains(got, callbackSettings) {
				t.Fatalf("the new message must carry the menu buttons: %v", got)
			}
		})
	}
}

func TestEditFallback_TimeSettingShowsSettingsInTheNewMessage(t *testing.T) {
	subs := subscribed(10)
	menu, api := newMenu(subs)
	api.editErr = fmt.Errorf("%w, Bad Request: message to edit not found", bot.ErrorBadRequest)

	menu.HandleTime(context.Background(), nil, callbackUpdate(10, timeData(6, 0)))

	if hour, _ := subs.Time(10); hour != subscribers.At(6, 0) {
		t.Fatalf("hour %d, want 6", hour)
	}
	if len(api.sent) != 1 {
		t.Fatalf("sent %d messages, want 1", len(api.sent))
	}
	if marked := markedHours(t, api.sent[0].ReplyMarkup); !slices.Equal(marked, []string{callbackHourPrefix + "6"}) {
		t.Fatalf("marked %v, want only 6", marked)
	}
}

func TestEditFailure_DoesNotSendDuplicates(t *testing.T) {
	for name, editErr := range map[string]error{
		"not modified":  fmt.Errorf("%w, Bad Request: message is not modified: specified new message content and reply markup are exactly the same", bot.ErrorBadRequest),
		"network error": context.DeadlineExceeded,
		"blocked":       errBlocked,
	} {
		t.Run(name, func(t *testing.T) {
			menu, api := newMenu(subscribed())
			api.editErr = editErr

			menu.HandleAbout(context.Background(), nil, callbackUpdate(10, callbackAbout))

			if len(api.sent) != 0 {
				t.Fatalf("sent %+v, want nothing", api.sent)
			}
		})
	}
}

func TestCallbackHandlers_IgnoreUpdateWithoutMessage(t *testing.T) {
	subs := subscribed()
	menu, api := newMenu(subs)
	update := &models.Update{CallbackQuery: &models.CallbackQuery{ID: "cb-1", Data: callbackSubscribe}}

	menu.HandleSubscribe(context.Background(), nil, update)

	if len(subs.chats()) != 0 || len(api.sent) != 0 || len(api.edited) != 0 {
		t.Fatalf("nothing should happen: subs=%v sent=%v edited=%v", subs.chats(), api.sent, api.edited)
	}
	if len(api.answered) != 1 {
		t.Fatal("the button spinner must still be stopped")
	}
}

// Telegram sends an inaccessible message when the menu message is too old.
// The bot library decodes it from JSON when "date" is 0, so build the update
// the same way.
func oldMenuUpdate(t *testing.T, chatID int64, data string) *models.Update {
	t.Helper()

	raw := fmt.Sprintf(`{"update_id":1,"callback_query":{"id":"cb-1","from":{"id":%d},"data":%q,`+
		`"message":{"message_id":7,"date":0,"chat":{"id":%d,"type":"private"}}}}`, chatID, data, chatID)

	var update models.Update
	if err := json.Unmarshal([]byte(raw), &update); err != nil {
		t.Fatal(err)
	}
	if update.CallbackQuery.Message.Type != models.MaybeInaccessibleMessageTypeInaccessibleMessage {
		t.Fatalf("test setup: message type %v, want inaccessible", update.CallbackQuery.Message.Type)
	}

	return &update
}

func TestOldMenuMessage_StillTriesToEditIt(t *testing.T) {
	subs := subscribed(10, 20)
	menu, api := newMenu(subs)

	menu.HandleUnsubscribe(context.Background(), nil, oldMenuUpdate(t, 10, callbackUnsubscribe))

	if !slices.Equal(subs.chats(), []int64{20}) {
		t.Fatalf("subscribers %v, want [20]", subs.chats())
	}
	if len(api.edited) != 1 || api.edited[0].MessageID != 7 || api.edited[0].ChatID != int64(10) {
		t.Fatalf("unexpected edits: %+v", api.edited)
	}
	if len(api.sent) != 0 {
		t.Fatalf("an edit that worked must not send a new menu: %+v", api.sent)
	}
}

func TestOldMenuMessage_FallsBackToANewMenu(t *testing.T) {
	subs := subscribed()
	menu, api := newMenu(subs)
	api.editErr = fmt.Errorf("%w, Bad Request: message to edit not found", bot.ErrorBadRequest)

	menu.HandleSubscribe(context.Background(), nil, oldMenuUpdate(t, 10, callbackSubscribe))
	menu.HandleAbout(context.Background(), nil, oldMenuUpdate(t, 10, callbackAbout))

	if !slices.Equal(subs.chats(), []int64{10}) {
		t.Fatalf("subscribers %v, want [10]", subs.chats())
	}
	if len(api.sent) != 2 || api.sent[0].Text != subscribedText(subscribers.DefaultTime) || api.sent[1].Text != aboutText {
		t.Fatalf("unexpected replies: %+v", api.sent)
	}
}

func TestSetup_PublishesCommandsAndDescriptions(t *testing.T) {
	menu, api := newMenu(subscribed())

	if err := menu.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(api.commands) != 1 {
		t.Fatalf("setMyCommands called %d times, want 1", len(api.commands))
	}
	var names []string
	for _, c := range api.commands[0].Commands {
		names = append(names, c.Command)
		if c.Description == "" || utf8.RuneCountInString(c.Description) > 256 {
			t.Errorf("command %q has a bad description %q", c.Command, c.Description)
		}
		if c.Command != strings.ToLower(c.Command) {
			t.Errorf("Telegram wants lower-case commands, got %q", c.Command)
		}
	}
	if !slices.Contains(names, "start") || !slices.Contains(names, "settings") {
		t.Fatalf("commands %v must include start and settings", names)
	}

	if len(api.descs) != 1 || api.descs[0].Description != botDescription {
		t.Fatalf("description not set: %+v", api.descs)
	}
	if len(api.shorts) != 1 || api.shorts[0].ShortDescription != botShortDescription {
		t.Fatalf("short description not set: %+v", api.shorts)
	}
}

func TestBotProfileTextFitsTelegramLimits(t *testing.T) {
	if n := utf8.RuneCountInString(botDescription); n == 0 || n > 512 {
		t.Errorf("description has %d characters, Telegram allows 1..512", n)
	}
	if n := utf8.RuneCountInString(botShortDescription); n == 0 || n > 120 {
		t.Errorf("short description has %d characters, Telegram allows 1..120", n)
	}
}

func TestSetup_ReportsErrorsButTriesEverything(t *testing.T) {
	menu, api := newMenu(subscribed())
	api.setupErr = errors.New("telegram is down")

	err := menu.Setup(context.Background())

	if err == nil {
		t.Fatal("want an error")
	}
	if len(api.commands) != 1 || len(api.descs) != 1 || len(api.shorts) != 1 {
		t.Fatalf("one failure must not skip the other calls: %d %d %d", len(api.commands), len(api.descs), len(api.shorts))
	}
}

// A fixed chat gets the post even without a subscription of its own, so the
// menu must not tell it the post is off.
func TestWelcome_FixedChatWithoutSubscriptionIsToldItGetsThePost(t *testing.T) {
	api := &fakeAPI{}
	menu := NewMenu(api, subscribed(group), []string{"-1001", "@gone"})
	ctx := context.Background()

	menu.HandleUnsubscribe(ctx, nil, callbackUpdate(group, callbackUnsubscribe))
	if len(api.edited) != 1 || api.edited[0].Text != fixedStatus() || !strings.Contains(fixedStatus(), "11:00") {
		t.Fatalf("after Unsubscribe the fixed chat must be told it gets the post at 11:00: %+v", api.edited)
	}

	menu.HandleStart(ctx, nil, startUpdate(group))
	menu.HandleStart(ctx, nil, startUpdate(private))
	if len(api.sent) != 2 || api.sent[0].Text != welcomeWith(fixedStatus()) {
		t.Fatalf("the fixed chat must be told it gets the post at 11:00: %+v", api.sent)
	}
	if api.sent[1].Text != welcomeFor(0, false) {
		t.Fatalf("another chat sees %q, want the unsubscribed state", api.sent[1].Text)
	}
}

func TestWelcome_ShowsTheSubscriptionState(t *testing.T) {
	subs := subscribed()
	subs.set(10, subscribers.At(15, 0))
	menu, api := newMenu(subs)

	menu.HandleStart(context.Background(), nil, startUpdate(10))
	menu.HandleStart(context.Background(), nil, startUpdate(20))

	if len(api.sent) != 2 {
		t.Fatalf("sent %d messages, want 2", len(api.sent))
	}
	if want := welcomeFor(subscribers.At(15, 0), true); api.sent[0].Text != want || !strings.Contains(want, "15:00") {
		t.Fatalf("subscriber sees %q, want the hour 15:00 in %q", api.sent[0].Text, want)
	}
	if want := welcomeFor(0, false); api.sent[1].Text != want || strings.Contains(want, "00:00") {
		t.Fatalf("non-subscriber sees %q, want %q", api.sent[1].Text, want)
	}
	if welcomeFor(subscribers.At(15, 0), true) == welcomeFor(0, false) {
		t.Fatal("the two states must read differently")
	}
}
