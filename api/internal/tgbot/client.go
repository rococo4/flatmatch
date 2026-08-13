// Package tgbot talks to the Telegram Bot API.
//
// It covers exactly what the product needs: sending messages (§7 delivers every
// notification this way), the moderation chat's photos and button taps (§5),
// and receiving updates. There is no third-party bot framework.
package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// apiBase is Telegram's Bot API endpoint. The bot token goes in the path.
const apiBase = "https://api.telegram.org"

// Client is a Bot API client for one bot.
type Client struct {
	token string
	base  string
	http  *http.Client
}

// APIError is a Bot API call that Telegram itself rejected, as opposed to a
// transport failure.
type APIError struct {
	Method      string
	Code        int
	Description string
}

// envelope is the shape every Bot API response takes.
type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

// InlineKeyboardButton is one button under a message.
//
// Only the fields the product actually sends are modelled — Telegram accepts a
// dozen more. Adding one when it is needed is a one-line change; carrying
// fields nobody sets only makes the type look like it does more than it does.
type InlineKeyboardButton struct {
	Text string  `json:"text"`
	Web  *WebApp `json:"web_app,omitempty"`
	// Data comes back verbatim in a callback_query and is what the moderation
	// verdict rides on (§5).
	Data string `json:"callback_data,omitempty"`
}

// WebApp points a button at a Mini App. Telegram requires an HTTPS URL.
type WebApp struct {
	URL string `json:"url"`
}

// InlineKeyboard is a grid of buttons: outer slice is rows.
type InlineKeyboard struct {
	Rows [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// SendMessageRequest is a sendMessage call.
type SendMessageRequest struct {
	ChatID      int64           `json:"chat_id"`
	Text        string          `json:"text"`
	ReplyMarkup *InlineKeyboard `json:"reply_markup,omitempty"`
}

// editReplyMarkupRequest is an editMessageReplyMarkup call. The markup is never
// omitted: dropping the field also clears the buttons, but then "убрать кнопки"
// and "не трогать кнопки" would look identical on the wire.
type editReplyMarkupRequest struct {
	ChatID      int64           `json:"chat_id"`
	MessageID   int             `json:"message_id"`
	ReplyMarkup *InlineKeyboard `json:"reply_markup"`
}

// answerCallbackRequest is an answerCallbackQuery call. Telegram shows the
// spinner on the tapped button until it arrives.
type answerCallbackRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
}

// BotInfo is the subset of getMe worth logging at startup.
type BotInfo struct {
	Username string `json:"username"`
}

// Update is an incoming event: a message to the bot or a tap on an inline
// button.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// Message is an incoming message.
//
// The sender is deliberately absent: the bot answers the chat, and identity
// comes from initData once the Mini App opens (§4) — never from a message,
// which would be a second, weaker way to decide who somebody is.
type Message struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// CallbackQuery is a tap on an inline button. Message is the message the button
// hangs under — absent when it is too old for Telegram to still know it, and
// then there are no buttons left to clear either.
type CallbackQuery struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *Message `json:"message"`
}

// Chat is where a message was sent.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// getUpdatesRequest is one long-polling call.
type getUpdatesRequest struct {
	Offset         int64    `json:"offset,omitempty"`
	Timeout        int      `json:"timeout"`
	AllowedUpdates []string `json:"allowed_updates"`
}

// NewClient builds a client.
//
// The HTTP timeout must outlast the longest getUpdates poll, or every poll
// would be cancelled client-side just before Telegram answers.
func NewClient(token string) *Client {
	return &Client{
		token: token,
		base:  apiBase,
		http:  &http.Client{Timeout: pollTimeout + 30*time.Second},
	}
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

// SendMessage delivers a message. This is the transport behind every
// notification in §7.
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) error {
	return c.call(ctx, "sendMessage", req, nil)
}

// SendPhotoFile uploads a photo and returns the id of the sent message.
//
// The file goes up as multipart instead of a URL: Telegram fetches URLs itself,
// and on a local stand the photos are not reachable from the internet.
func (c *Client) SendPhotoFile(ctx context.Context, chatID int64, path, caption string, kb *InlineKeyboard) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open photo for sendPhoto: %w", err)
	}
	defer file.Close()

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	fields := [][2]string{{"chat_id", strconv.FormatInt(chatID, 10)}, {"caption", caption}}
	if kb != nil {
		markup, err := json.Marshal(kb)
		if err != nil {
			return 0, fmt.Errorf("encode sendPhoto keyboard: %w", err)
		}
		fields = append(fields, [2]string{"reply_markup", string(markup)})
	}
	for _, f := range fields {
		if err := form.WriteField(f[0], f[1]); err != nil {
			return 0, fmt.Errorf("build sendPhoto request: %w", err)
		}
	}

	part, err := form.CreateFormFile("photo", filepath.Base(path))
	if err != nil {
		return 0, fmt.Errorf("build sendPhoto request: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return 0, fmt.Errorf("read photo for sendPhoto: %w", err)
	}
	if err := form.Close(); err != nil {
		return 0, fmt.Errorf("build sendPhoto request: %w", err)
	}

	var sent Message
	if err := c.do(ctx, "sendPhoto", form.FormDataContentType(), body.Bytes(), &sent); err != nil {
		return 0, err
	}
	return sent.MessageID, nil
}

// EditReplyMarkup replaces the buttons under a message; a nil keyboard removes
// them, which is how a handled moderation card stops being tappable (§5).
func (c *Client) EditReplyMarkup(ctx context.Context, chatID int64, messageID int, kb *InlineKeyboard) error {
	if kb == nil {
		kb = &InlineKeyboard{Rows: [][]InlineKeyboardButton{}}
	}
	return c.call(ctx, "editMessageReplyMarkup", editReplyMarkupRequest{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: kb,
	}, nil)
}

// AnswerCallback closes a callback_query. Telegram expects one within seconds,
// and until it arrives the button keeps spinning.
func (c *Client) AnswerCallback(ctx context.Context, callbackQueryID, text string) error {
	return c.call(ctx, "answerCallbackQuery", answerCallbackRequest{
		CallbackQueryID: callbackQueryID,
		Text:            text,
	}, nil)
}

// GetMe verifies the token and reports which bot it belongs to. Calling it at
// startup turns "the button does nothing" into a clear error in the logs.
func (c *Client) GetMe(ctx context.Context) (BotInfo, error) {
	var info BotInfo
	err := c.call(ctx, "getMe", struct{}{}, &info)
	return info, err
}

// GetUpdates long-polls for events newer than offset.
//
// Telegram treats the offset as an acknowledgement: asking for N+1 confirms
// everything up to N and lets it drop them, so the offset must only advance
// once an update has actually been handled.
func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", getUpdatesRequest{
		Offset:         offset,
		Timeout:        int(pollTimeout / time.Second),
		AllowedUpdates: []string{"message", "callback_query"},
	}, &updates)
	return updates, err
}

// call invokes a Bot API method with JSON parameters and decodes result into
// out, which may be nil when the caller does not care about the payload.
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", method, err)
	}
	return c.do(ctx, method, "application/json", body, out)
}

// do sends an already encoded request body.
func (c *Client) do(ctx context.Context, method, contentType string, body []byte, out any) error {
	url := fmt.Sprintf("%s/bot%s/%s", c.base, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		// The URL contains the bot token, and transport errors quote the URL.
		return fmt.Errorf("call %s: %w", method, redactToken(err, c.token))
	}
	defer resp.Body.Close()

	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode %s response (http %d): %w", method, resp.StatusCode, err)
	}
	if !env.OK {
		return &APIError{Method: method, Code: env.ErrorCode, Description: env.Description}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(env.Result, out); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}
	return nil
}

// redactToken keeps the bot token out of error strings, which are logged.
func redactToken(err error, token string) error {
	if token == "" {
		return err
	}
	if msg := err.Error(); strings.Contains(msg, token) {
		return fmt.Errorf("%s", strings.ReplaceAll(msg, token, "<token>"))
	}
	return err
}
