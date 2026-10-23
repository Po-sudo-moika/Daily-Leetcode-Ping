package telegram

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	adminUser  int64 = 501
	memberUser int64 = 502
)

// pressUpdate is a button press by user on the menu message 77 of a chat of
// the given type.
func pressUpdate(chatID int64, chatType models.ChatType, user int64, data string) *models.Update {
	update := callbackUpdate(chatID, data)
	update.CallbackQuery.From = models.User{ID: user}
	update.CallbackQuery.Message.Message.Chat.Type = chatType

	return update
}

// newGroupMenu is a menu where adminUser is an admin and the group is
// subscribed at 09:30.
func newGroupMenu() (*Menu, *fakeAPI, *fakeSubscribers) {
	subs := subscribed()
	subs.set(group, subscribers.At(9, 30))
	menu, api := newMenu(subs)
	api.members = map[int64]models.ChatMemberType{adminUser: models.ChatMemberTypeAdministrator}

	return menu, api, subs
}

type pressed struct {
	name    string
	data    string
	handler func(*Menu) func(context.Context, *bot.Bot, *models.Update)
}

var changingButtons = []pressed{
	{"Unsubscribe", callbackUnsubscribe, func(m *Menu) func(context.Context, *bot.Bot, *models.Update) { return m.HandleUnsubscribe }},
	{"Subscribe", callbackSubscribe, func(m *Menu) func(context.Context, *bot.Bot, *models.Update) { return m.HandleSubscribe }},
	{"hour", callbackHourPrefix + "15", func(m *Menu) func(context.Context, *bot.Bot, *models.Update) { return m.HandleHour }},
	{"time", timeData(15, 45), func(m *Menu) func(context.Context, *bot.Bot, *models.Update) { return m.HandleTime }},
}

func TestAdminsOnly_MemberOfAGroupChangesNothingAndGetsTheNote(t *testing.T) {
	for _, chatType := range []models.ChatType{models.ChatTypeGroup, models.ChatTypeSupergroup} {
		for _, button := range changingButtons {
			t.Run(string(chatType)+"/"+button.name, func(t *testing.T) {
				menu, api, subs := newGroupMenu()

				button.handler(menu)(context.Background(), nil, pressUpdate(group, chatType, memberUser, button.data))

				if at, ok := subs.Time(group); !ok || at != subscribers.At(9, 30) {
					t.Fatalf("subscription changed: time=%s subscribed=%v, want 09:30", subscribers.Format(at), ok)
				}
				if len(api.edited) != 0 || len(api.sent) != 0 {
					t.Fatalf("the menu changed: edited %d, sent %d", len(api.edited), len(api.sent))
				}
				if len(api.answered) != 1 || api.answered[0].CallbackQueryID != "cb-1" || api.answered[0].Text != adminsOnlyText {
					t.Fatalf("answers %+v, want one with the admins-only note", api.answered)
				}
				if len(api.asked) != 1 || api.asked[0].ChatID != group || api.asked[0].UserID != memberUser {
					t.Fatalf("asked Telegram %+v, want the member of the group", api.asked)
				}
			})
		}
	}
}

func TestAdminsOnly_AdminsOfAGroupCanChangeIt(t *testing.T) {
	for _, status := range []models.ChatMemberType{models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator} {
		t.Run(string(status), func(t *testing.T) {
			menu, api, subs := newGroupMenu()
			api.members = map[int64]models.ChatMemberType{adminUser: status}
			ctx := context.Background()

			menu.HandleHour(ctx, nil, pressUpdate(group, models.ChatTypeSupergroup, adminUser, callbackHourPrefix+"15"))
			menu.HandleTime(ctx, nil, pressUpdate(group, models.ChatTypeSupergroup, adminUser, timeData(15, 45)))
			if at, _ := subs.Time(group); at != subscribers.At(15, 45) {
				t.Fatalf("an admin's time press: got %s, want 15:45", subscribers.Format(at))
			}

			menu.HandleUnsubscribe(ctx, nil, pressUpdate(group, models.ChatTypeSupergroup, adminUser, callbackUnsubscribe))
			if _, ok := subs.Time(group); ok {
				t.Fatal("an admin's Unsubscribe must work")
			}

			menu.HandleSubscribe(ctx, nil, pressUpdate(group, models.ChatTypeSupergroup, adminUser, callbackSubscribe))
			if _, ok := subs.Time(group); !ok {
				t.Fatal("an admin's Subscribe must work")
			}

			for _, answer := range api.answered {
				if answer.Text != "" {
					t.Fatalf("an admin got a note: %q", answer.Text)
				}
			}
			if len(api.edited) != 4 {
				t.Fatalf("edited %d times, want 4", len(api.edited))
			}
		})
	}
}

// When Telegram cannot say who the user is, nothing changes.
func TestAdminsOnly_FailedCheckChangesNothing(t *testing.T) {
	menu, api, subs := newGroupMenu()
	api.memberErr = errors.New("telegram is down")

	menu.HandleTime(context.Background(), nil, pressUpdate(group, models.ChatTypeGroup, adminUser, timeData(15, 45)))

	if at, _ := subs.Time(group); at != subscribers.At(9, 30) {
		t.Fatalf("time changed to %s", subscribers.Format(at))
	}
	if len(api.edited) != 0 || len(api.answered) != 1 || api.answered[0].Text != adminCheckFailedText {
		t.Fatalf("edited %d, answers %+v, want one note that the check failed", len(api.edited), api.answered)
	}
}

// Private chats are unchanged, a channel has no menu users to check, and
// looking at the screens is open to everyone in a group.
func TestAdminsOnly_OnlyChangesInGroupsAreChecked(t *testing.T) {
	t.Run("private chat", func(t *testing.T) {
		menu, api, subs := newGroupMenu()
		subs.set(private, subscribers.At(9, 30))

		menu.HandleTime(context.Background(), nil, pressUpdate(private, models.ChatTypePrivate, memberUser, timeData(15, 45)))

		if at, _ := subs.Time(private); at != subscribers.At(15, 45) {
			t.Fatalf("time %s, want 15:45", subscribers.Format(at))
		}
		if len(api.asked) != 0 {
			t.Fatalf("a private chat asked Telegram about admins: %+v", api.asked)
		}
	})

	t.Run("channel", func(t *testing.T) {
		menu, api, subs := newGroupMenu()
		subs.set(-1002, subscribers.At(9, 30))

		menu.HandleTime(context.Background(), nil, pressUpdate(-1002, models.ChatTypeChannel, memberUser, timeData(15, 45)))

		if at, _ := subs.Time(-1002); at != subscribers.At(15, 45) {
			t.Fatalf("time %s, want 15:45", subscribers.Format(at))
		}
		if len(api.asked) != 0 {
			t.Fatalf("a channel asked Telegram about admins: %+v", api.asked)
		}
	})

	t.Run("looking at the screens", func(t *testing.T) {
		menu, api, subs := newGroupMenu()
		ctx := context.Background()

		menu.HandleSettings(ctx, nil, pressUpdate(group, models.ChatTypeGroup, memberUser, callbackSettings))
		menu.HandleAbout(ctx, nil, pressUpdate(group, models.ChatTypeGroup, memberUser, callbackAbout))
		menu.HandleBack(ctx, nil, pressUpdate(group, models.ChatTypeGroup, memberUser, callbackMenu))

		if len(api.edited) != 3 || len(api.asked) != 0 {
			t.Fatalf("edited %d, asked %d; want 3 screens and no admin check", len(api.edited), len(api.asked))
		}
		if at, _ := subs.Time(group); at != subscribers.At(9, 30) {
			t.Fatalf("time changed to %s", subscribers.Format(at))
		}
	})
}

// The buttons reach the check through the bot, as Telegram sends them.
func TestAdminsOnly_ThroughTheBot(t *testing.T) {
	subs := subscribed()
	subs.set(group, subscribers.At(9, 30))
	b, api := newRegisteredBot(t, subs)
	api.members = map[int64]models.ChatMemberType{adminUser: models.ChatMemberTypeAdministrator}
	ctx := context.Background()

	b.ProcessUpdate(ctx, pressUpdate(group, models.ChatTypeSupergroup, memberUser, callbackUnsubscribe))
	if !slices.Equal(subs.chats(), []int64{group}) {
		t.Fatalf("a member's Unsubscribe changed the subscribers: %v", subs.chats())
	}

	b.ProcessUpdate(ctx, pressUpdate(group, models.ChatTypeSupergroup, adminUser, callbackUnsubscribe))
	if len(subs.chats()) != 0 {
		t.Fatalf("an admin's Unsubscribe left subscribers: %v", subs.chats())
	}
}
