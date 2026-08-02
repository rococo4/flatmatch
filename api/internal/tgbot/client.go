// Package tgbot talks to the Telegram Bot API.
//
// It covers exactly what the product needs: sending messages (§7 delivers every
// notification this way) and receiving updates. There is no third-party bot
// framework — the surface is two methods.
package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// apiBase is Telegram's Bot API endpoint. The bot token goes in the path.
const apiBase = "https://api.telegram.org"

// Client is a Bot API client for one bot.
type Client struct {
	token string
	http  *http.Client
}

// NewClient builds a client.
//
// The HTTP timeout must outlast the longest getUpdates poll, or every poll
// would be cancelled client-side just before Telegram answers.
func NewClient(token string) *Client {
	return &Client{
		token: token,
		http:  &http.Client{Timeout: pollTimeout + 30*time.Second},
	}
}

// APIError is a Bot API call that Telegram itself rejected, as opposed to a
// transport failure.
type APIError struct {
	Method      string
	Code        int
	Description string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

// envelope is the shape every Bot API response takes.
type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

// call invokes a Bot API method and decodes result into out, which may be nil
// when the caller does not care about the payload.
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", method, err)
	}

	url := fmt.Sprintf("%s/bot%s/%s", apiBase, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

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

// InlineKeyboardButton is one button under a message.
//
// Only the fields the product actually sends are modelled — Telegram accepts a
// dozen more. Adding one when it is needed is a one-line change; carrying
// fields nobody sets only makes the type look like it does more than it does.
type InlineKeyboardButton struct {
	Text string  `json:"text"`
	Web  *WebApp `json:"web_app,omitempty"`
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

// SendMessage delivers a message. This is the transport behind every
// notification in §7.
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) error {
	return c.call(ctx, "sendMessage", req, nil)
}

// BotInfo is the subset of getMe worth logging at startup.
type BotInfo struct {
	Username string `json:"username"`
}

// GetMe verifies the token and reports which bot it belongs to. Calling it at
// startup turns "the button does nothing" into a clear error in the logs.
func (c *Client) GetMe(ctx context.Context) (BotInfo, error) {
	var info BotInfo
	err := c.call(ctx, "getMe", struct{}{}, &info)
	return info, err
}

// Update is an incoming event. The skeleton only reads messages.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

// Message is an incoming message.
//
// The sender is deliberately absent: the bot answers the chat, and identity
// comes from initData once the Mini App opens (§4) — never from a message,
// which would be a second, weaker way to decide who somebody is.
type Message struct {
	Chat Chat   `json:"chat"`
	Text string `json:"text"`
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
		AllowedUpdates: []string{"message"},
	}, &updates)
	return updates, err
}
