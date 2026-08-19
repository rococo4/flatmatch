package tgbot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	ModerationCallbackPrefix = "mod"
	SearchCallbackPrefix     = "srch"
	PollCallbackPrefix       = "poll"
	PointCallbackPrefix      = "pnt"
)

var ErrBlocked = errors.New("the bot cannot reach this chat")

var blockedDescriptions = []string{
	"bot was blocked by the user",
	"chat not found",
	"user is deactivated",
	"bot can't initiate conversation with a user",
}

type PushButton struct {
	Text string

	StartParam   string
	CallbackData string
}

type PushSender struct {
	client *Client

	appURL string
}

func NewPushSender(client *Client, appURL string) *PushSender {
	return &PushSender{client: client, appURL: appURL}
}

func (s *PushSender) SendPush(ctx context.Context, chatID int64, text string, buttons []PushButton) (int, error) {
	msgID, err := s.client.SendMessage(ctx, SendMessageRequest{
		ChatID:      chatID,
		Text:        text,
		ReplyMarkup: s.keyboard(buttons),
	})
	return msgID, asBlocked(err)
}

func (s *PushSender) EditPushText(ctx context.Context, chatID int64, messageID int, text string, buttons []PushButton) error {
	return asBlocked(s.client.EditMessageText(ctx, chatID, messageID, text, s.keyboard(buttons)))
}

func (s *PushSender) keyboard(buttons []PushButton) *InlineKeyboard {
	if len(buttons) == 0 {
		return nil
	}
	var answers, links []InlineKeyboardButton
	for _, b := range buttons {
		if b.StartParam != "" {
			links = append(links, InlineKeyboardButton{Text: b.Text, Web: &WebApp{URL: s.deepLink(b.StartParam)}})
			continue
		}
		answers = append(answers, InlineKeyboardButton{Text: b.Text, Data: b.CallbackData})
	}
	rows := make([][]InlineKeyboardButton, 0, 2)
	for _, row := range [][]InlineKeyboardButton{answers, links} {
		if len(row) > 0 {
			rows = append(rows, row)
		}
	}
	return &InlineKeyboard{Rows: rows}
}

func (s *PushSender) deepLink(startParam string) string {
	separator := "?"
	if strings.Contains(s.appURL, "?") {
		separator = "&"
	}
	return s.appURL + separator + "startapp=" + url.QueryEscape(startParam)
}

func asBlocked(err error) error {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return err
	}
	description := strings.ToLower(apiErr.Description)
	for _, blocked := range blockedDescriptions {
		if strings.Contains(description, blocked) {
			return fmt.Errorf("%w: %w", ErrBlocked, err)
		}
	}
	return err
}
