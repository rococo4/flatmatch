package tgbot

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

const moderationChat = int64(-100500)

type fakeModeration struct {
	handled  []string
	ack      string
	err      error
	queued   int
	queueErr error
}

func (f *fakeModeration) HandleCallback(_ context.Context, data string) (string, error) {
	f.handled = append(f.handled, data)
	return f.ack, f.err
}

func (f *fakeModeration) SendQueue(context.Context) error {
	f.queued++
	return f.queueErr
}

type fakePushes struct {
	searchTaps []buttonTap
	pollTaps   []buttonTap
	pointTaps  []buttonTap
	unblocked  []int64
	ack        string
	err        error
}

type buttonTap struct {
	tgID int64
	data string
}

func (f *fakePushes) HandleSearchCallback(_ context.Context, tgID int64, data string) (string, error) {
	f.searchTaps = append(f.searchTaps, buttonTap{tgID, data})
	return f.ack, f.err
}

func (f *fakePushes) HandlePollCallback(_ context.Context, tgID int64, data string) (string, error) {
	f.pollTaps = append(f.pollTaps, buttonTap{tgID, data})
	return f.ack, f.err
}

func (f *fakePushes) HandlePointCallback(_ context.Context, tgID int64, data string) (string, error) {
	f.pointTaps = append(f.pointTaps, buttonTap{tgID, data})
	return f.ack, f.err
}

func (f *fakePushes) ClearPushBlocked(_ context.Context, tgID int64) error {
	f.unblocked = append(f.unblocked, tgID)
	return nil
}

func newTestBot(api *botAPI, mod Moderation, chatID int64) *Bot {
	return newTestBotWithPushes(api, mod, chatID, &fakePushes{})
}

func newTestBotWithPushes(api *botAPI, mod Moderation, chatID int64, pushes Pushes) *Bot {
	return NewBot(api.client(), "https://app.example", slog.New(slog.DiscardHandler), mod, chatID, pushes)
}

func privateTap(tgID int64, data string) Update {
	return Update{
		UpdateID: 4,
		CallbackQuery: &CallbackQuery{
			ID:      "q-2",
			Data:    data,
			Message: &Message{MessageID: 501, Chat: Chat{ID: tgID, Type: "private"}},
		},
	}
}

func tap(chatID int64, data string) Update {
	return Update{
		UpdateID: 1,
		CallbackQuery: &CallbackQuery{
			ID:      "q-1",
			Data:    data,
			Message: &Message{MessageID: 777, Chat: Chat{ID: chatID, Type: "supergroup"}},
		},
	}
}

func TestVerdictIsAppliedAndTheCardStopsBeingTappable(t *testing.T) {
	for _, ack := range []string{"✓ Одобрено", "уже обработано"} {
		t.Run(ack, func(t *testing.T) {
			api := newBotAPI(t)
			mod := &fakeModeration{ack: ack}

			newTestBot(api, mod, moderationChat).handle(t.Context(), tap(moderationChat, "mod:p:ok:42"))

			if len(mod.handled) != 1 || mod.handled[0] != "mod:p:ok:42" {
				t.Fatalf("moderation got %v, want [mod:p:ok:42]", mod.handled)
			}
			answer := api.got("answerCallbackQuery")
			if answer == nil {
				t.Fatal("the tap was never answered, the button keeps spinning")
			}
			if got := answer.body["text"]; got != ack {
				t.Errorf("ack = %v, want %q", got, ack)
			}
			edit := api.got("editMessageReplyMarkup")
			if edit == nil {
				t.Fatal("the buttons were left under a handled card")
			}
			if got := edit.body["chat_id"]; got != float64(moderationChat) {
				t.Errorf("chat_id = %v, want %d", got, moderationChat)
			}
			if got := edit.body["message_id"]; got != float64(777) {
				t.Errorf("message_id = %v, want 777", got)
			}
		})
	}
}

func TestFailedVerdictKeepsTheButtons(t *testing.T) {
	api := newBotAPI(t)
	mod := &fakeModeration{err: errors.New("connection refused")}

	newTestBot(api, mod, moderationChat).handle(t.Context(), tap(moderationChat, "mod:u:ok:42"))

	answer := api.got("answerCallbackQuery")
	if answer == nil {
		t.Fatal("the tap was never answered")
	}
	if got := answer.body["text"]; got != ackFailed {
		t.Errorf("ack = %v, want %q", got, ackFailed)
	}
	if api.got("editMessageReplyMarkup") != nil {
		t.Error("the buttons were cleared although the verdict did not apply")
	}
}

func TestTapsFromOtherChatsAreIgnored(t *testing.T) {
	api := newBotAPI(t)
	mod := &fakeModeration{ack: "✓ Одобрено"}

	newTestBot(api, mod, moderationChat).handle(t.Context(), tap(777001, "mod:u:ok:42"))

	if len(mod.handled) != 0 {
		t.Errorf("moderation was called with %v", mod.handled)
	}
	if api.count() != 0 {
		t.Errorf("%d calls made, want silence", api.count())
	}
}

func TestTapsAreIgnoredWithoutAModerationChat(t *testing.T) {
	api := newBotAPI(t)
	mod := &fakeModeration{ack: "✓ Одобрено"}

	newTestBot(api, mod, 0).handle(t.Context(), tap(moderationChat, "mod:u:ok:42"))

	if len(mod.handled) != 0 || api.count() != 0 {
		t.Errorf("moderation got %v and %d calls were made, want nothing", mod.handled, api.count())
	}
}

func TestQueueCommandResendsTheQueue(t *testing.T) {
	api := newBotAPI(t)
	mod := &fakeModeration{}

	newTestBot(api, mod, moderationChat).handle(t.Context(), Update{
		UpdateID: 2,
		Message:  &Message{Chat: Chat{ID: moderationChat, Type: "supergroup"}, Text: "/queue"},
	})

	if mod.queued != 1 {
		t.Errorf("SendQueue called %d times, want 1", mod.queued)
	}

	if api.got("sendMessage") != nil {
		t.Error("the moderation chat was answered with the start message")
	}
}

func TestUsernameIsEmptyUntilTelegramAnswers(t *testing.T) {
	api := newBotAPI(t)
	api.replies["getMe"] = `{"ok":true,"result":{"username":"flatmatch_finder_bot"}}`
	api.replies["getUpdates"] = `{"ok":true,"result":[]}`
	bot := newTestBot(api, &fakeModeration{}, moderationChat)

	if got := bot.Username(); got != "" {
		t.Fatalf("Username() = %q before the poll, want empty", got)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- bot.Run(ctx) }()

	deadline := time.After(5 * time.Second)
	for bot.Username() == "" {
		select {
		case err := <-done:
			t.Fatalf("Run() stopped before the name arrived: %v", err)
		case <-deadline:
			t.Fatal("the username never arrived")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if got := bot.Username(); got != "flatmatch_finder_bot" {
		t.Errorf("Username() = %q, want flatmatch_finder_bot", got)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("Run() failed: %v", err)
	}
}

func TestQueueCommandElsewhereIsAnOrdinaryMessage(t *testing.T) {
	api := newBotAPI(t)
	mod := &fakeModeration{}

	newTestBot(api, mod, moderationChat).handle(t.Context(), Update{
		UpdateID: 3,
		Message:  &Message{Chat: Chat{ID: 42, Type: "private"}, Text: "/queue"},
	})

	if mod.queued != 0 {
		t.Error("a stranger re-sent the moderation queue")
	}
	if api.got("sendMessage") == nil {
		t.Error("the message was left unanswered")
	}
}

func TestPushAnswersAreRoutedByPrefix(t *testing.T) {
	cases := []struct {
		name string
		data string
		ack  string
	}{
		{"search", "srch:yes:1a1c1de1-0000-4000-8000-000000000001", "✓ Ищу дальше"},
		{"poll", "poll:1a1c1de1-0000-4000-8000-000000000002:moving", "✓ Съезжаемся"},
		{"point", "pnt:alive:1a1c1de1-0000-4000-8000-000000000003", "✓ Отмечено: актуальна"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newBotAPI(t)
			pushes := &fakePushes{ack: tc.ack}

			newTestBotWithPushes(api, &fakeModeration{}, moderationChat, pushes).
				handle(t.Context(), privateTap(188993596, tc.data))

			routed := pushes.searchTaps
			switch tc.name {
			case "poll":
				routed = pushes.pollTaps
			case "point":
				routed = pushes.pointTaps
			}
			if len(routed) != 1 || routed[0].data != tc.data || routed[0].tgID != 188993596 {
				t.Fatalf("routed %v, want one tap of %q from 188993596", routed, tc.data)
			}
			if len(pushes.searchTaps)+len(pushes.pollTaps)+len(pushes.pointTaps) != 1 {
				t.Errorf("the tap reached more than one handler: %v / %v / %v",
					pushes.searchTaps, pushes.pollTaps, pushes.pointTaps)
			}

			answer := api.got("answerCallbackQuery")
			if answer == nil {
				t.Fatal("the tap was never answered, the button keeps spinning")
			}
			if got := answer.body["text"]; got != tc.ack {
				t.Errorf("ack = %v, want %q", got, tc.ack)
			}

			if api.got("editMessageReplyMarkup") == nil {
				t.Error("the buttons were left under an answered push")
			}
		})
	}
}

func TestTapOnAnAnsweredPushSaysSo(t *testing.T) {
	api := newBotAPI(t)
	pushes := &fakePushes{}

	newTestBotWithPushes(api, &fakeModeration{}, moderationChat, pushes).
		handle(t.Context(), privateTap(188993596, pushSeen))

	if len(pushes.searchTaps)+len(pushes.pollTaps) != 0 {
		t.Error("the mark was routed to a handler")
	}
	answer := api.got("answerCallbackQuery")
	if answer == nil || answer.body["text"] != ackSeen {
		t.Errorf("ack = %v, want %q", answer, ackSeen)
	}
}

func TestFailedPushAnswerKeepsTheButtons(t *testing.T) {
	api := newBotAPI(t)
	pushes := &fakePushes{err: errors.New("connection refused")}

	newTestBotWithPushes(api, &fakeModeration{}, moderationChat, pushes).
		handle(t.Context(), privateTap(188993596, "srch:found:1a1c1de1-0000-4000-8000-000000000001"))

	if got := api.got("answerCallbackQuery").body["text"]; got != ackFailed {
		t.Errorf("ack = %v, want %q", got, ackFailed)
	}
	if api.got("editMessageReplyMarkup") != nil {
		t.Error("the buttons were cleared although nothing was applied")
	}
}

func TestPushAnswersOutsidePrivateChatsAreIgnored(t *testing.T) {
	api := newBotAPI(t)
	pushes := &fakePushes{ack: "✓ Ищу дальше"}

	newTestBotWithPushes(api, &fakeModeration{}, moderationChat, pushes).
		handle(t.Context(), tap(moderationChat, "srch:yes:1a1c1de1-0000-4000-8000-000000000001"))

	if len(pushes.searchTaps) != 0 {
		t.Errorf("a group tap was routed: %v", pushes.searchTaps)
	}
	if api.count() != 0 {
		t.Errorf("%d calls made, want silence", api.count())
	}
}

func TestVerdictsFromPrivateChatsAreIgnored(t *testing.T) {
	api := newBotAPI(t)
	mod := &fakeModeration{ack: "✓ Одобрено"}

	newTestBotWithPushes(api, mod, moderationChat, &fakePushes{}).
		handle(t.Context(), privateTap(188993596, "mod:u:ok:42"))

	if len(mod.handled) != 0 {
		t.Errorf("moderation got %v from a private chat", mod.handled)
	}
}

func TestMessageToTheBotClearsThePushBlock(t *testing.T) {
	api := newBotAPI(t)
	pushes := &fakePushes{}

	newTestBotWithPushes(api, &fakeModeration{}, moderationChat, pushes).handle(t.Context(), Update{
		UpdateID: 5,
		Message:  &Message{Chat: Chat{ID: 188993596, Type: "private"}, Text: "/start"},
	})

	if len(pushes.unblocked) != 1 || pushes.unblocked[0] != 188993596 {
		t.Errorf("unblocked %v, want [188993596]", pushes.unblocked)
	}
	if api.got("sendMessage") == nil {
		t.Error("/start was left unanswered")
	}
}
