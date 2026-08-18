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
	pollTimeout = 30 * time.Second

	retryDelay = 5 * time.Second
)

const (
	startMessage = "FlatMatch — поиск со-съёмщика в Москве.\n\n" +
		"Квартиру найдёшь где угодно — мы найдём того, с кем её снимать."
	openButtonText = "Открыть FlatMatch"
)

const queueCommand = "/queue"

const verdictSeen = "mod:seen"

const pushSeen = "push:seen"

const (
	ackFailed = "не вышло — попробуй ещё раз"
	ackSeen   = "уже обработано"
)

type Moderation interface {
	HandleCallback(ctx context.Context, data string) (string, error)
	SendQueue(ctx context.Context) error
}

type Pushes interface {
	HandleSearchCallback(ctx context.Context, tgID int64, data string) (string, error)
	HandlePollCallback(ctx context.Context, tgID int64, data string) (string, error)
	HandlePointCallback(ctx context.Context, tgID int64, data string) (string, error)
	ClearPushBlocked(ctx context.Context, tgID int64) error
}

type Bot struct {
	client *Client
	appURL string
	log    *slog.Logger

	moderation Moderation

	moderationChatID int64

	pushes Pushes

	username atomic.Pointer[string]
}

func NewBot(client *Client, appURL string, log *slog.Logger,
	moderation Moderation, chatID int64, pushes Pushes) *Bot {
	return &Bot{
		client:           client,
		appURL:           appURL,
		log:              log,
		moderation:       moderation,
		moderationChatID: chatID,
		pushes:           pushes,
	}
}

func (b *Bot) Username() string {
	if name := b.username.Load(); name != nil {
		return *name
	}
	return ""
}

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

			offset = update.UpdateID + 1
		}
	}
}

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

	command, _, _ := strings.Cut(msg.Text, " ")
	if command != "/start" {
		log.Debug("message is not /start, answering with the same button", "text", msg.Text)
	}

	if b.pushes != nil {
		if err := b.pushes.ClearPushBlocked(ctx, msg.Chat.ID); err != nil && !isCancelled(err) {
			log.Error("could not clear the push block", "error", err)
		}
	}

	_, err := b.client.SendMessage(ctx, SendMessageRequest{
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

func (b *Bot) handleCallback(ctx context.Context, query *CallbackQuery, updateID int64) {
	if query.Message == nil {
		return
	}
	log := b.log.With("chat_id", query.Message.Chat.ID, "update_id", updateID, "data", query.Data)

	if query.Data == pushSeen {
		b.answer(ctx, query.ID, ackSeen, log)
		return
	}

	prefix, _, _ := strings.Cut(query.Data, ":")
	switch prefix {
	case ModerationCallbackPrefix:
		b.handleVerdict(ctx, query, log)
	case SearchCallbackPrefix, PollCallbackPrefix, PointCallbackPrefix:
		b.handlePushAnswer(ctx, query, log)
	}
}

func (b *Bot) handlePushAnswer(ctx context.Context, query *CallbackQuery, log *slog.Logger) {
	if b.pushes == nil || query.Message.Chat.Type != "private" {
		return
	}

	prefix, _, _ := strings.Cut(query.Data, ":")
	var handle func(context.Context, int64, string) (string, error)
	switch prefix {
	case PollCallbackPrefix:
		handle = b.pushes.HandlePollCallback
	case PointCallbackPrefix:
		handle = b.pushes.HandlePointCallback
	default:
		handle = b.pushes.HandleSearchCallback
	}

	ack, answerErr := handle(ctx, query.Message.Chat.ID, query.Data)
	if answerErr != nil {
		if isCancelled(answerErr) {
			return
		}
		log.Error("push answer failed", "error", answerErr)
		ack = ackFailed
	}
	b.answer(ctx, query.ID, ack, log)
	if answerErr != nil {
		return
	}
	b.markAnswered(ctx, query, ack, pushSeen, log)
}

func (b *Bot) handleVerdict(ctx context.Context, query *CallbackQuery, log *slog.Logger) {
	if !b.moderationEnabled() || query.Message.Chat.ID != b.moderationChatID {
		return
	}

	if query.Data == verdictSeen {
		b.answer(ctx, query.ID, ackSeen, log)
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

	b.answer(ctx, query.ID, ack, log)
	if verdictErr != nil {
		return
	}
	b.markAnswered(ctx, query, ack, verdictSeen, log)
}

func (b *Bot) answer(ctx context.Context, queryID, ack string, log *slog.Logger) {
	if err := b.client.AnswerCallback(ctx, queryID, ack); err != nil && !isCancelled(err) {
		log.Error("could not answer the callback", "error", err)
	}
}

func (b *Bot) markAnswered(ctx context.Context, query *CallbackQuery, ack, seenData string, log *slog.Logger) {
	mark := &InlineKeyboard{Rows: [][]InlineKeyboardButton{{{Text: ack, Data: seenData}}}}
	err := b.client.EditReplyMarkup(ctx, query.Message.Chat.ID, query.Message.MessageID, mark)
	if err != nil && !isCancelled(err) && !notModified(err) {
		log.Error("could not mark the answered message", "error", err)
	}
}

func (b *Bot) moderationEnabled() bool {
	return b.moderation != nil && b.moderationChatID != 0
}

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
