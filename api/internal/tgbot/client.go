package tgbot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const apiBase = "https://api.telegram.org"

type Client struct {
	token string
	base  string
	http  *http.Client
}

type APIError struct {
	Method      string
	Code        int
	Description string
}

type envelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
}

type InlineKeyboardButton struct {
	Text string  `json:"text"`
	Web  *WebApp `json:"web_app,omitempty"`

	Data string `json:"callback_data,omitempty"`
}

type WebApp struct {
	URL string `json:"url"`
}

type InlineKeyboard struct {
	Rows [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type SendMessageRequest struct {
	ChatID      int64           `json:"chat_id"`
	Text        string          `json:"text"`
	ReplyMarkup *InlineKeyboard `json:"reply_markup,omitempty"`
}

type editMessageTextRequest struct {
	ChatID      int64           `json:"chat_id"`
	MessageID   int             `json:"message_id"`
	Text        string          `json:"text"`
	ReplyMarkup *InlineKeyboard `json:"reply_markup,omitempty"`
}

type editReplyMarkupRequest struct {
	ChatID      int64           `json:"chat_id"`
	MessageID   int             `json:"message_id"`
	ReplyMarkup *InlineKeyboard `json:"reply_markup"`
}

type answerCallbackRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
}

type BotInfo struct {
	Username string `json:"username"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID   int             `json:"message_id"`
	Chat        Chat            `json:"chat"`
	Text        string          `json:"text"`
	ReplyMarkup *InlineKeyboard `json:"reply_markup"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	Data    string   `json:"data"`
	Message *Message `json:"message"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type getUpdatesRequest struct {
	Offset         int64    `json:"offset,omitempty"`
	Timeout        int      `json:"timeout"`
	AllowedUpdates []string `json:"allowed_updates"`
}

type inputMediaPhoto struct {
	Type  string `json:"type"`
	Media string `json:"media"`
}

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

func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (int, error) {
	var sent Message
	if err := c.call(ctx, "sendMessage", req, &sent); err != nil {
		return 0, err
	}
	return sent.MessageID, nil
}

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

func (c *Client) SendMediaGroupFiles(ctx context.Context, chatID int64, paths []string) error {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)

	media := make([]inputMediaPhoto, 0, len(paths))
	for i, p := range paths {
		name := fmt.Sprintf("photo%d", i)
		if err := attachFile(form, name, p); err != nil {
			return err
		}
		media = append(media, inputMediaPhoto{Type: "photo", Media: "attach://" + name})
	}

	encoded, err := json.Marshal(media)
	if err != nil {
		return fmt.Errorf("encode sendMediaGroup album: %w", err)
	}
	for _, f := range [][2]string{
		{"chat_id", strconv.FormatInt(chatID, 10)},
		{"media", string(encoded)},
	} {
		if err := form.WriteField(f[0], f[1]); err != nil {
			return fmt.Errorf("build sendMediaGroup request: %w", err)
		}
	}
	if err := form.Close(); err != nil {
		return fmt.Errorf("build sendMediaGroup request: %w", err)
	}
	return c.do(ctx, "sendMediaGroup", form.FormDataContentType(), body.Bytes(), nil)
}

func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int, text string, kb *InlineKeyboard) error {
	err := c.call(ctx, "editMessageText", editMessageTextRequest{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ReplyMarkup: kb,
	}, nil)
	if notModified(err) {
		return nil
	}
	return err
}

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

func (c *Client) AnswerCallback(ctx context.Context, callbackQueryID, text string) error {
	return c.call(ctx, "answerCallbackQuery", answerCallbackRequest{
		CallbackQueryID: callbackQueryID,
		Text:            text,
	}, nil)
}

func (c *Client) GetMe(ctx context.Context) (BotInfo, error) {
	var info BotInfo
	err := c.call(ctx, "getMe", struct{}{}, &info)
	return info, err
}

func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", getUpdatesRequest{
		Offset:         offset,
		Timeout:        int(pollTimeout / time.Second),
		AllowedUpdates: []string{"message", "callback_query"},
	}, &updates)
	return updates, err
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("encode %s request: %w", method, err)
	}
	return c.do(ctx, method, "application/json", body, out)
}

func (c *Client) do(ctx context.Context, method, contentType string, body []byte, out any) error {
	url := fmt.Sprintf("%s/bot%s/%s", c.base, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
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

func attachFile(form *multipart.Writer, field, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open photo for the album: %w", err)
	}
	defer file.Close()

	part, err := form.CreateFormFile(field, filepath.Base(path))
	if err != nil {
		return fmt.Errorf("build the album request: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("read photo for the album: %w", err)
	}
	return nil
}

func notModified(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && strings.Contains(apiErr.Description, "message is not modified")
}

func redactToken(err error, token string) error {
	if token == "" {
		return err
	}
	if msg := err.Error(); strings.Contains(msg, token) {
		return fmt.Errorf("%s", strings.ReplaceAll(msg, token, "<token>"))
	}
	return err
}
