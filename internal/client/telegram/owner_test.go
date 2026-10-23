package telegram

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Po-sudo-moika/Daily-Leetcode-Ping/internal/subscribers"
	"github.com/go-telegram/bot/models"
)

const ownerUser int64 = 9001

// chatsUpdate is the /chats command typed by user in the chat.
func chatsUpdate(chatID, user int64, chatType models.ChatType) *models.Update {
	update := commandUpdate(chatID, "/chats")
	update.Message.From = &models.User{ID: user}
	update.Message.Chat.Type = chatType

	return update
}

// ownerFixture: a private chat (pinned to 09:30, got the post on 2026-10-10),
// a group on the default time, a supergroup that pressed Unsubscribe, a fixed
// chat given by id that is not subscribed, a fixed chat that is also a
// subscriber, and a fixed chat Telegram cannot find.
func ownerFixture() (*Menu, *fakeAPI, *fakeSubscribers) {
	subs := subscribed(group)
	subs.set(private, subscribers.At(9, 30))
	subs.set(-1005, subscribers.At(18, 5))
	subs.optedOut = map[int64]bool{-1002: true}
	subs.delivered = map[string]string{"10": "2026-10-10", "-1001": "2026-10-11"}

	api := &fakeAPI{chatInfos: map[string]models.ChatFullInfo{
		"10":    {ID: private, Type: models.ChatTypePrivate, FirstName: "Ann", LastName: "Lee", Username: "annlee"},
		"-1001": {ID: group, Type: models.ChatTypeGroup, Title: "Go Club"},
		"-1002": {ID: -1002, Type: models.ChatTypeSupergroup, Title: "Old Friends"},
		"-1003": {ID: -1003, Type: models.ChatTypeChannel, Title: "News"},
		"-1005": {ID: -1005, Type: models.ChatTypeSupergroup, Title: "Fixed And Subscribed"},
	}}
	menu := NewMenu(api, subs, []string{"-1003", "-1005", "@gone"}).WithOwner(ownerUser)

	return menu, api, subs
}

func sentToOwner(t *testing.T, api *fakeAPI) string {
	t.Helper()

	var texts []string
	for _, msg := range api.sent {
		if msg.ChatID != ownerUser {
			t.Fatalf("the list went to %v, want only the owner", msg.ChatID)
		}
		texts = append(texts, msg.Text)
	}

	return strings.Join(texts, "\n")
}

func TestChats_OwnerGetsEveryKnownChatWithItsState(t *testing.T) {
	menu, api, _ := ownerFixture()

	menu.HandleChats(context.Background(), nil, chatsUpdate(int64(ownerUser), ownerUser, models.ChatTypePrivate))

	got := sentToOwner(t, api)
	for _, want := range []string{
		"Чаты бота: 6",
		// a private chat: name, @username, type, time, subscribed, last post day
		"Ann Lee @annlee — личный чат (10)\n   время 09:30 МСК · подписан · последний пост: 2026-10-10",
		// a group on the default time
		"Go Club — группа (-1001)\n   время 11:00 МСК · подписан · последний пост: 2026-10-11",
		// a supergroup that pressed Unsubscribe
		"Old Friends — супергруппа (-1002)\n   время — · отписан · последний пост: —",
		// fixed chats: one without a subscription, one that is also a subscriber
		"News — канал (-1003)\n   время 11:00 МСК · подписки нет · чат из config.toml · последний пост: —",
		"Fixed And Subscribed — супергруппа (-1005)\n   время 18:05 МСК · подписан · чат из config.toml · последний пост: —",
		// a fixed chat Telegram cannot find
		"⚠️ Telegram не нашёл этот чат (@gone)\n   время 11:00 МСК · подписки нет · чат из config.toml · последний пост: —",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the list lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "Fixed And Subscribed") != 1 {
		t.Errorf("a fixed chat that is also a subscriber must be listed once:\n%s", got)
	}
}

func TestChats_OtherUsersGetNoAnswer(t *testing.T) {
	for name, update := range map[string]*models.Update{
		"another user in a private chat": chatsUpdate(10, 10, models.ChatTypePrivate),
		"a member of a group":            chatsUpdate(group, memberUser, models.ChatTypeGroup),
		"a message without a sender":     {Message: &models.Message{Chat: models.Chat{ID: group}, Text: "/chats"}},
		"a button press":                 callbackUpdate(10, "chats"),
	} {
		t.Run(name, func(t *testing.T) {
			menu, api, _ := ownerFixture()

			menu.HandleChats(context.Background(), nil, update)

			if len(api.sent) != 0 || len(api.edited) != 0 || len(api.answered) != 0 || len(api.gotChats) != 0 {
				t.Fatalf("answered a user who is not the owner: sent %v, edited %v, asked Telegram %d times", api.sent, api.edited, len(api.gotChats))
			}
		})
	}
}

func TestChats_NoOwnerSetAnswersNobody(t *testing.T) {
	menu, api, _ := ownerFixture()
	menu.WithOwner(0)

	menu.HandleChats(context.Background(), nil, chatsUpdate(10, 0, models.ChatTypePrivate))

	if len(api.sent) != 0 {
		t.Fatalf("sent %v, want nothing", api.sent)
	}
}

// The owner may type the command in a group; the list still goes to their
// private chat, so the group never sees it.
func TestChats_FromTheOwnerInAGroupGoesToTheOwnersPrivateChat(t *testing.T) {
	menu, api, _ := ownerFixture()

	menu.HandleChats(context.Background(), nil, chatsUpdate(group, ownerUser, models.ChatTypeGroup))

	if len(api.sent) == 0 {
		t.Fatal("want the list")
	}
	sentToOwner(t, api)
}

func TestChats_ThroughTheBotOnlyForTheOwner(t *testing.T) {
	subs := subscribed(group)
	b, api := newRegisteredBotWithOwner(t, subs, ownerUser)
	ctx := context.Background()

	b.ProcessUpdate(ctx, chatsUpdate(10, 10, models.ChatTypePrivate))
	for _, msg := range api.sent {
		if strings.Contains(msg.Text, "Чаты бота") {
			t.Fatalf("a stranger got the list: %q", msg.Text)
		}
	}

	b.ProcessUpdate(ctx, chatsUpdate(ownerUser, ownerUser, models.ChatTypePrivate))
	var lists int
	for _, msg := range api.sent {
		if strings.HasPrefix(msg.Text, "Чаты бота") && msg.ChatID == ownerUser {
			lists++
		}
	}
	if lists != 1 {
		t.Fatalf("the owner got %d lists, want 1: %+v", lists, api.recipients())
	}
}

func TestChats_AChatTelegramFailsToAnswerForIsNotCalledMissing(t *testing.T) {
	menu, api, _ := ownerFixture()
	api.getChatErr = map[string]error{"-1001": fmt.Errorf("timeout")}

	menu.HandleChats(context.Background(), nil, chatsUpdate(int64(ownerUser), ownerUser, models.ChatTypePrivate))

	got := sentToOwner(t, api)
	if !strings.Contains(got, "❓ Не удалось спросить Telegram про чат (-1001)") || strings.Contains(got, "не нашёл этот чат (-1001)") {
		t.Fatalf("a failed lookup is not a missing chat:\n%s", got)
	}
}

func TestChats_NoChatsAtAll(t *testing.T) {
	menu := NewMenu(&fakeAPI{}, subscribed(), nil).WithOwner(ownerUser)
	api := menu.api.(*fakeAPI)

	menu.HandleChats(context.Background(), nil, chatsUpdate(int64(ownerUser), ownerUser, models.ChatTypePrivate))

	if got := sentToOwner(t, api); got != "Чатов пока нет." {
		t.Fatalf("got %q", got)
	}
}

// A long list is split across messages, none over Telegram's limit, and no
// chat is cut in two or lost.
func TestChats_LongListIsSplitWithinTheMessageLimit(t *testing.T) {
	const total = 150
	subs := subscribed()
	infos := map[string]models.ChatFullInfo{}
	for i := 1; i <= total; i++ {
		id := int64(-1000 - i)
		subs.set(id, subscribers.DefaultTime)
		infos[strconv.FormatInt(id, 10)] = models.ChatFullInfo{ID: id, Type: models.ChatTypeGroup, Title: strings.Repeat("я", 40) + strconv.Itoa(i)}
	}
	api := &fakeAPI{chatInfos: infos}
	menu := NewMenu(api, subs, nil).WithOwner(ownerUser)

	menu.HandleChats(context.Background(), nil, chatsUpdate(int64(ownerUser), ownerUser, models.ChatTypePrivate))

	if len(api.sent) < 2 {
		t.Fatalf("sent %d messages, want the list split", len(api.sent))
	}
	var all []string
	for _, msg := range api.sent {
		if n := utf8.RuneCountInString(msg.Text); n > 4096 {
			t.Fatalf("a message of %d characters is over Telegram's limit", n)
		}
		all = append(all, msg.Text)
	}
	joined := strings.Join(all, "\n")
	for i := 1; i <= total; i++ {
		if !strings.Contains(joined, "я"+strconv.Itoa(i)+" — группа (") {
			t.Fatalf("chat %d is missing from the list", i)
		}
	}
	if !strings.HasPrefix(api.sent[0].Text, fmt.Sprintf("Чаты бота: %d", total)) {
		t.Fatalf("first message starts %q", api.sent[0].Text[:30])
	}
}

func TestChats_FixedChatsComeFirstInConfigOrder(t *testing.T) {
	menu, api, _ := ownerFixture()

	menu.HandleChats(context.Background(), nil, chatsUpdate(int64(ownerUser), ownerUser, models.ChatTypePrivate))

	got := sentToOwner(t, api)
	var order []int
	for _, name := range []string{"News", "Fixed And Subscribed", "@gone", "Old Friends", "Go Club", "Ann Lee"} {
		order = append(order, strings.Index(got, name))
	}
	if !slices.IsSorted(order) || slices.Contains(order, -1) {
		t.Fatalf("order of chats %v in:\n%s", order, got)
	}
}
