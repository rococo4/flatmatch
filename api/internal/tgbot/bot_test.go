package tgbot

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

const moderationChat = int64(-100500)

// fakeModeration records the verdicts routed to it.
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

func newTestBot(api *botAPI, mod Moderation, chatID int64) *Bot {
	return NewBot(api.client(), "https://app.example", slog.New(slog.DiscardHandler), mod, chatID)
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
	// Both acks mean the card is done with: the second tap on an old button is
	// ordinary in a chat where the queue is scrolled top to bottom (§3 дизайна).
	for _, ack := range []string{"✓", "уже обработано"} {
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
	// Nothing was applied, so the tap has to stay repeatable.
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
	// A verdict from anywhere else would be a stranger moderating the product.
	api := newBotAPI(t)
	mod := &fakeModeration{ack: "✓"}

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
	mod := &fakeModeration{ack: "✓"}

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
	// The moderation chat gets the queue, not the Mini App button.
	if api.got("sendMessage") != nil {
		t.Error("the moderation chat was answered with the start message")
	}
}

func TestUsernameIsEmptyUntilTelegramAnswers(t *testing.T) {
	// The Mini App asks /me for this name to build share links, and /me may be
	// answered before the first poll ever happens.
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
