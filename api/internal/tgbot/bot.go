package tgbot

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
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

// Bot answers messages sent to the bot.
//
// The skeleton has exactly one job: hand out the button that opens the Mini App
// (§13). Notification delivery (§7) is a later step and will use the same
// Client from the other direction.
type Bot struct {
	client *Client
	appURL string
	log    *slog.Logger
}

// NewBot builds the bot. appURL is the HTTPS address the Mini App opens at;
// config guarantees it is set and https when polling is enabled.
func NewBot(client *Client, appURL string, log *slog.Logger) *Bot {
	return &Bot{client: client, appURL: appURL, log: log}
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
	b.log.Info("bot polling started", "username", "@"+info.Username, "app_url", b.appURL)

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

// handle dispatches one update. Anything that is not a text message in a
// private chat is ignored — the bot has no other surface yet.
func (b *Bot) handle(ctx context.Context, update Update) {
	msg := update.Message
	if msg == nil || msg.Chat.Type != "private" {
		return
	}

	log := b.log.With("chat_id", msg.Chat.ID, "update_id", update.UpdateID)

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
