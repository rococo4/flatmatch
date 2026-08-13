package tgbot

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	// pollTimeout is how long Telegram holds a getUpdates call open with no
	// events. Long enough that idle polling is nearly free, short enough that
	// shutdown is not noticeably delayed.
	pollTimeout = 30 * time.Second
	// retryDelay is the pause after a failed poll, so a Telegram outage does
	// not turn into a request flood.
	retryDelay = 5 * time.Second
)

// Copy shown on /start. It is the product's positioning from §1, verbatim:
// FlatMatch is not a flat aggregator, it finds the person you rent with.
const (
	startMessage = "FlatMatch — поиск со-съёмщика в Москве.\n\n" +
		"Квартиру найдёшь где угодно — мы найдём того, с кем её снимать."
	openButtonText = "Открыть FlatMatch"
)

// queueCommand re-sends everything still waiting for a verdict — the way back
// after a restart, when the messages with buttons were never delivered.
const queueCommand = "/queue"

// ackFailed answers a tap whose verdict did not apply. The buttons stay under
// the message, so the moderator can simply tap again.
const verdictSeen = "mod:seen"

const ackFailed = "не вышло — попробуй ещё раз"

// Moderation is the moderation chat's brain: the bot only routes taps to it and
// reports back what it says.
type Moderation interface {
	HandleCallback(ctx context.Context, data string) (string, error)
	SendQueue(ctx context.Context) error
}

// Bot answers messages sent to the bot and serves the moderation chat (§5).
type Bot struct {
	client *Client
	appURL string
	log    *slog.Logger

	moderation Moderation
	// moderationChatID is the only chat whose button taps mean anything: a
	// verdict from anywhere else would be a stranger moderating the product.
	moderationChatID int64

	// username is what getMe answered, and Telegram is the only one who knows
	// it. Run fills it in while requests are already being served, so it is
	// read through an atomic — and it is empty until the answer arrives.
	username atomic.Pointer[string]
}

// NewBot builds the bot. appURL is the HTTPS address the Mini App opens at;
// config guarantees it is set and https when polling is enabled. moderation may
// be nil, and chatID zero — the queues then only pile up in the database.
func NewBot(client *Client, appURL string, log *slog.Logger, moderation Moderation, chatID int64) *Bot {
	return &Bot{
		client:           client,
		appURL:           appURL,
		log:              log,
		moderation:       moderation,
		moderationChatID: chatID,
	}
}

// Username is the bot's @name, without the @. Empty until getMe answers — the
// Mini App then falls back to the name it was built with.
func (b *Bot) Username() string {
	if name := b.username.Load(); name != nil {
		return *name
	}
	return ""
}

// Run polls for updates until ctx is cancelled.
//
// Errors are logged and retried rather than returned: a Telegram hiccup must
// not take the API down with it. Run returns only on shutdown, or when the
// problem is permanent enough that retrying is pointless.
func (b *Bot) Run(ctx context.Context) error {
	info, err := b.client.GetMe(ctx)
	if err != nil {
		return err
	}
	b.username.Store(&info.Username)
	b.log.Info("bot polling started", "username", "@"+info.Username, "app_url", b.appURL)
	if !b.moderationEnabled() {
		b.log.Warn("moderation chat is not connected, verdict buttons will not arrive anywhere")
	}

	// Starting at offset 0 replays whatever arrived while the process was
	// down. That is the behaviour we want: somebody who wrote to the bot
	// during a deploy should get their button, not silence. Duplicates are not
	// a risk — the offset advances past every update that has been handled,
	// whether or not the reply went through.
	var offset int64

	for {
		if ctx.Err() != nil {
			b.log.Info("bot polling stopped")
			return nil
		}

		updates, err := b.client.GetUpdates(ctx, offset)
		if err != nil {
			if isCancelled(err) {
				b.log.Info("bot polling stopped")
				return nil
			}
			// 409 means another process is polling the same bot. Retrying
			// forever would have the two fight over every update, so say so
			// loudly and keep the message clear.
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.Code == http.StatusConflict {
				return err
			}
			b.log.Error("poll for updates failed, retrying", "error", err, "retry_in", retryDelay)
			if !sleep(ctx, retryDelay) {
				return nil
			}
			continue
		}

		for _, update := range updates {
			b.handle(ctx, update)
			// Advance only past what has been handled: the offset is also the
			// acknowledgement that lets Telegram forget the update.
			offset = update.UpdateID + 1
		}
	}
}

// handle dispatches one update: a moderation verdict, the moderation chat's
// /queue, or a message that gets the door to the Mini App.
func (b *Bot) handle(ctx context.Context, update Update) {
	if update.CallbackQuery != nil {
		b.handleCallback(ctx, update.CallbackQuery, update.UpdateID)
		return
	}

	msg := update.Message
	if msg == nil {
		return
	}

	log := b.log.With("chat_id", msg.Chat.ID, "update_id", update.UpdateID)

	if b.moderationEnabled() && msg.Chat.ID == b.moderationChatID &&
		strings.TrimSpace(msg.Text) == queueCommand {
		if err := b.moderation.SendQueue(ctx); err != nil && !isCancelled(err) {
			log.Error("could not re-send the moderation queue", "error", err)
		}
		return
	}

	if msg.Chat.Type != "private" {
		return
	}

	// Every message gets the same answer. There is nothing else to say: the
	// product lives in the Mini App, and the bot is the door to it.
	command, _, _ := strings.Cut(msg.Text, " ")
	if command != "/start" {
		log.Debug("message is not /start, answering with the same button", "text", msg.Text)
	}

	err := b.client.SendMessage(ctx, SendMessageRequest{
		ChatID: msg.Chat.ID,
		Text:   startMessage,
		ReplyMarkup: &InlineKeyboard{Rows: [][]InlineKeyboardButton{{
			{Text: openButtonText, Web: &WebApp{URL: b.appURL}},
		}}},
	})
	if err != nil && !isCancelled(err) {
		log.Error("could not answer message", "error", err)
	}
}

// handleCallback applies one moderation verdict. Taps from any other chat are
// ignored without a word: answering them would tell a stranger the button
// exists.
func (b *Bot) handleCallback(ctx context.Context, query *CallbackQuery, updateID int64) {
	if !b.moderationEnabled() || query.Message == nil || query.Message.Chat.ID != b.moderationChatID {
		return
	}
	log := b.log.With("chat_id", query.Message.Chat.ID, "update_id", updateID, "data", query.Data)

	if query.Data == verdictSeen {
		if err := b.client.AnswerCallback(ctx, query.ID, "уже обработано"); err != nil && !isCancelled(err) {
			log.Error("could not answer the callback", "error", err)
		}
		return
	}

	ack, verdictErr := b.moderation.HandleCallback(ctx, query.Data)
	if verdictErr != nil {
		if isCancelled(verdictErr) {
			return
		}
		log.Error("moderation verdict failed", "error", verdictErr)
		ack = ackFailed
	}

	if err := b.client.AnswerCallback(ctx, query.ID, ack); err != nil && !isCancelled(err) {
		log.Error("could not answer the callback", "error", err)
	}
	if verdictErr != nil {
		// Buttons stay: nothing was applied, and the tap has to be repeatable.
		return
	}

	// The card is done with: the buttons give way to a single verdict mark —
	// a toast is easy to miss, a labelled card is not. Tapping the mark answers
	// «уже обработано». «Message is not modified» means another tap already
	// drew the same mark — not an error.
	mark := &InlineKeyboard{Rows: [][]InlineKeyboardButton{{{Text: ack, Data: verdictSeen}}}}
	err := b.client.EditReplyMarkup(ctx, query.Message.Chat.ID, query.Message.MessageID, mark)
	if err != nil && !isCancelled(err) && !strings.Contains(err.Error(), "message is not modified") {
		log.Error("could not mark the moderation card", "error", err)
	}
}

func (b *Bot) moderationEnabled() bool {
	return b.moderation != nil && b.moderationChatID != 0
}

// sleep waits for d and reports whether it finished rather than being
// cancelled.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func isCancelled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
