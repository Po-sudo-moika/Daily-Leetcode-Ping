package telegram

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot/models"
)

const (
	group   = int64(-1001)
	private = int64(10)
)

func TestMyChatMember_AddedToGroupSubscribesItAtDefaultHourAndItGetsThePost(t *testing.T) {
	for _, chatType := range []models.ChatType{models.ChatTypeGroup, models.ChatTypeSupergroup, models.ChatTypeChannel} {
		for _, role := range []models.ChatMemberType{models.ChatMemberTypeMember, models.ChatMemberTypeAdministrator} {
			t.Run(string(chatType)+"/"+string(role), func(t *testing.T) {
				subs := subscribed()
				b, api := newRegisteredBot(t, subs)

				b.ProcessUpdate(context.Background(), myChatMemberUpdate(group, chatType, models.ChatMemberTypeLeft, role))

				if hour, ok := subs.Hour(group); !ok || hour != subscribers.DefaultHour {
					t.Fatalf("hour=%d subscribed=%v, want subscribed at %d", hour, ok, subscribers.DefaultHour)
				}

				c, _ := New(api, nil, subs)
				if err := c.SendDue(context.Background(), at(11, 11, 0), question); err != nil {
					t.Fatal(err)
				}
				if want := []any{group}; !slices.Equal(api.recipients(), want) {
					t.Fatalf("the next post went to %v, want %v", api.recipients(), want)
				}
			})
		}
	}
}

func TestMyChatMember_RemovedFromGroupUnsubscribesIt(t *testing.T) {
	for _, gone := range []models.ChatMemberType{models.ChatMemberTypeLeft, models.ChatMemberTypeBanned} {
		t.Run(string(gone), func(t *testing.T) {
			subs := subscribed()
			b, _ := newRegisteredBot(t, subs)
			ctx := context.Background()

			b.ProcessUpdate(ctx, myChatMemberUpdate(group, models.ChatTypeSupergroup, models.ChatMemberTypeLeft, models.ChatMemberTypeMember))
			b.ProcessUpdate(ctx, myChatMemberUpdate(group, models.ChatTypeSupergroup, models.ChatMemberTypeMember, gone))
			if len(subs.chats()) != 0 {
				t.Fatalf("subscribers %v, want none after the bot was removed", subs.chats())
			}

			b.ProcessUpdate(ctx, myChatMemberUpdate(group, models.ChatTypeSupergroup, gone, models.ChatMemberTypeMember))
			if !slices.Equal(subs.chats(), []int64{group}) {
				t.Fatalf("subscribers %v, want the group again after the bot was added again", subs.chats())
			}
		})
	}
}

func TestMyChatMember_AddedAgainSubscribesAGroupThatUnsubscribed(t *testing.T) {
	subs := subscribed(group)
	b, _ := newRegisteredBot(t, subs)
	ctx := context.Background()

	b.ProcessUpdate(ctx, callbackUpdate(group, callbackUnsubscribe))
	b.ProcessUpdate(ctx, myChatMemberUpdate(group, models.ChatTypeGroup, models.ChatMemberTypeMember, models.ChatMemberTypeLeft))
	b.ProcessUpdate(ctx, myChatMemberUpdate(group, models.ChatTypeGroup, models.ChatMemberTypeLeft, models.ChatMemberTypeMember))

	if !slices.Equal(subs.chats(), []int64{group}) {
		t.Fatalf("subscribers %v, want the group", subs.chats())
	}
}

// Being made an administrator is not being added: a group that pressed
// Unsubscribe must stay out.
func TestMyChatMember_PromotionDoesNotUndoUnsubscribe(t *testing.T) {
	subs := subscribed(group)
	b, _ := newRegisteredBot(t, subs)
	ctx := context.Background()

	b.ProcessUpdate(ctx, callbackUpdate(group, callbackUnsubscribe))
	b.ProcessUpdate(ctx, myChatMemberUpdate(group, models.ChatTypeGroup, models.ChatMemberTypeMember, models.ChatMemberTypeAdministrator))
	b.ProcessUpdate(ctx, myChatMemberUpdate(group, models.ChatTypeGroup, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember))

	if len(subs.chats()) != 0 {
		t.Fatalf("subscribers %v, want none", subs.chats())
	}
}

func TestMyChatMember_RestrictedBotChangesNothing(t *testing.T) {
	subs := subscribed(group)
	b, _ := newRegisteredBot(t, subs)

	b.ProcessUpdate(context.Background(), myChatMemberUpdate(group, models.ChatTypeGroup, models.ChatMemberTypeMember, models.ChatMemberTypeRestricted))

	if !slices.Equal(subs.chats(), []int64{group}) {
		t.Fatalf("subscribers %v, want the group kept", subs.chats())
	}
}

func TestMyChatMember_UserBlockingTheBotUnsubscribesButUnblockingDoesNotSubscribe(t *testing.T) {
	subs := subscribed(private)
	b, _ := newRegisteredBot(t, subs)
	ctx := context.Background()

	b.ProcessUpdate(ctx, myChatMemberUpdate(private, models.ChatTypePrivate, models.ChatMemberTypeMember, models.ChatMemberTypeBanned))
	if len(subs.chats()) != 0 {
		t.Fatalf("subscribers %v, want none after the block", subs.chats())
	}

	// A user who unblocks the bot and presses Start is met again; the
	// unblock alone changes nothing.
	b.ProcessUpdate(ctx, myChatMemberUpdate(private, models.ChatTypePrivate, models.ChatMemberTypeBanned, models.ChatMemberTypeMember))
	if len(subs.chats()) != 0 {
		t.Fatalf("subscribers %v, want none after the unblock alone", subs.chats())
	}
	b.ProcessUpdate(ctx, commandUpdate(private, "/start"))
	if !slices.Equal(subs.chats(), []int64{private}) {
		t.Fatalf("subscribers %v, want the user after /start", subs.chats())
	}
}

func TestMyChatMember_StoreFailureIsSurvived(t *testing.T) {
	subs := subscribed()
	subs.failWrite = errBlocked
	b, _ := newRegisteredBot(t, subs)

	b.ProcessUpdate(context.Background(), myChatMemberUpdate(group, models.ChatTypeGroup, models.ChatMemberTypeLeft, models.ChatMemberTypeMember))

	if len(subs.chats()) != 0 {
		t.Fatalf("subscribers %v, want none", subs.chats())
	}
}

func TestMyChatMember_IgnoresUpdateWithoutMembership(t *testing.T) {
	menu, _ := newMenu(subscribed())

	menu.HandleMyChatMember(context.Background(), nil, &models.Update{})
}

func TestFirstContact_NewPrivateChatIsSubscribedByStart(t *testing.T) {
	subs := subscribed()
	b, api := newRegisteredBot(t, subs)

	b.ProcessUpdate(context.Background(), commandUpdate(private, "/start"))

	if hour, ok := subs.Hour(private); !ok || hour != subscribers.DefaultHour {
		t.Fatalf("hour=%d subscribed=%v, want subscribed at %d with no Subscribe press", hour, ok, subscribers.DefaultHour)
	}
	if len(api.sent) != 1 || api.sent[0].Text != welcomeFor(subscribers.DefaultHour, true) {
		t.Fatalf("the menu must show the subscribed state: %+v", api.sent)
	}
}

func TestFirstContact_AnyUpdateFromAnUnseenChatSubscribesIt(t *testing.T) {
	updates := map[string]*models.Update{
		"command in a group":      commandUpdate(group, "/settings"),
		"plain message":           {Message: &models.Message{Chat: models.Chat{ID: group}, Text: "hello"}},
		"button on an old menu":   callbackUpdate(group, callbackAbout),
		"button, message too old": oldMenuUpdate(t, group, callbackAbout),
	}

	for name, update := range updates {
		t.Run(name, func(t *testing.T) {
			subs := subscribed()
			b, _ := newRegisteredBot(t, subs)

			b.ProcessUpdate(context.Background(), update)

			if hour, ok := subs.Hour(group); !ok || hour != subscribers.DefaultHour {
				t.Fatalf("hour=%d subscribed=%v, want subscribed at %d", hour, ok, subscribers.DefaultHour)
			}
		})
	}
}

func TestFirstContact_KeepsTheHourOfASubscriber(t *testing.T) {
	subs := subscribed()
	subs.set(private, 15)
	b, _ := newRegisteredBot(t, subs)

	b.ProcessUpdate(context.Background(), commandUpdate(private, "/start"))

	if hour, _ := subs.Hour(private); hour != 15 {
		t.Fatalf("hour %d, want 15", hour)
	}
}

func TestFirstContact_UnsubscribedChatStaysOutAlsoAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subscribers.json")
	store := openStore(t, path, nil)
	b, api := newRegisteredBot(t, store)
	ctx := context.Background()

	b.ProcessUpdate(ctx, commandUpdate(private, "/start"))
	b.ProcessUpdate(ctx, callbackUpdate(private, callbackUnsubscribe))
	b.ProcessUpdate(ctx, commandUpdate(private, "/start"))
	b.ProcessUpdate(ctx, commandUpdate(private, "/settings"))
	if _, ok := store.Hour(private); ok {
		t.Fatal("/start must not subscribe a chat that pressed Unsubscribe")
	}

	restarted := openStore(t, path, nil)
	b, api = newRegisteredBot(t, restarted)
	b.ProcessUpdate(ctx, commandUpdate(private, "/start"))
	if _, ok := restarted.Hour(private); ok {
		t.Fatal("after a restart /start must not subscribe a chat that pressed Unsubscribe")
	}
	if len(api.sent) != 1 || api.sent[0].Text != welcomeFor(0, false) {
		t.Fatalf("the menu must show the unsubscribed state: %+v", api.sent)
	}

	// Pressing Subscribe is the way back.
	b.ProcessUpdate(ctx, callbackUpdate(private, callbackSubscribe))
	if _, ok := restarted.Hour(private); !ok {
		t.Fatal("Subscribe must subscribe the chat again")
	}
}

func TestFirstContact_StoreFailureStillAnswers(t *testing.T) {
	subs := subscribed()
	subs.failWrite = errBlocked
	b, api := newRegisteredBot(t, subs)

	b.ProcessUpdate(context.Background(), commandUpdate(private, "/start"))

	if len(api.sent) != 1 {
		t.Fatalf("sent %d messages, want the menu", len(api.sent))
	}
}

func TestFirstContact_UpdateWithoutAChatIsPassedOn(t *testing.T) {
	subs := subscribed()
	b, _ := newRegisteredBot(t, subs)

	b.ProcessUpdate(context.Background(), &models.Update{CallbackQuery: &models.CallbackQuery{ID: "cb-1", Data: callbackAbout}})

	if len(subs.chats()) != 0 {
		t.Fatalf("subscribers %v, want none", subs.chats())
	}
}

func TestAllowedUpdates_IncludeWhatTheBotNeeds(t *testing.T) {
	for _, kind := range []string{models.AllowedUpdateMessage, models.AllowedUpdateCallbackQuery, models.AllowedUpdateMyChatMember} {
		if !slices.Contains(AllowedUpdates, kind) {
			t.Errorf("allowed updates %v miss %q", AllowedUpdates, kind)
		}
	}
}
