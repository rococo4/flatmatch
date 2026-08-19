package tgbot

import (
	"errors"
	"fmt"
	"testing"
)

func newTestPushSender(api *botAPI) *PushSender {
	return NewPushSender(api.client(), "https://app.example")
}

func TestSendPushOpensTheMiniAppWithAStartParam(t *testing.T) {
	api := newBotAPI(t)
	api.replies["sendMessage"] = `{"ok":true,"result":{"message_id":501}}`

	msgID, err := newTestPushSender(api).SendPush(t.Context(), 42,
		"В Хамовниках ищет квартиру ещё один человек",
		[]PushButton{{Text: "Открыть пул", StartParam: "pool_d-khamovniki"}})
	if err != nil {
		t.Fatalf("SendPush() failed: %v", err)
	}
	if msgID != 501 {
		t.Errorf("SendPush() = %d, want the id Telegram answered (501)", msgID)
	}

	call := api.got("sendMessage")
	if call == nil {
		t.Fatal("nothing was sent")
	}
	if got := call.body["chat_id"]; got != float64(42) {
		t.Errorf("chat_id = %v, want 42", got)
	}
	if got := call.body["text"]; got != "В Хамовниках ищет квартиру ещё один человек" {
		t.Errorf("text = %v", got)
	}

	button := onlyButton(t, call.body)
	if got := button["text"]; got != "Открыть пул" {
		t.Errorf("button text = %v, want «Открыть пул»", got)
	}
	webApp, ok := button["web_app"].(map[string]any)
	if !ok {
		t.Fatalf("button = %v, want a web_app one", button)
	}
	if got, want := webApp["url"], "https://app.example?startapp=pool_d-khamovniki"; got != want {
		t.Errorf("web_app url = %v, want %q", got, want)
	}
	if _, has := button["callback_data"]; has {
		t.Errorf("a deep-link button also carries callback_data: %v", button)
	}
}

func TestSendPushSendsAnswerButtonsAsCallbacks(t *testing.T) {
	api := newBotAPI(t)

	_, err := newTestPushSender(api).SendPush(t.Context(), 42, "Всё ещё ищешь в Хамовниках?",
		[]PushButton{{Text: "Да, ищу", CallbackData: "srch:yes:1"}})
	if err != nil {
		t.Fatalf("SendPush() failed: %v", err)
	}

	button := onlyButton(t, api.got("sendMessage").body)
	if got := button["callback_data"]; got != "srch:yes:1" {
		t.Errorf("callback_data = %v, want srch:yes:1", got)
	}
	if _, has := button["web_app"]; has {
		t.Errorf("an answer button opens the Mini App: %v", button)
	}
}

func TestEditPushTextRewritesTheSentMessage(t *testing.T) {
	api := newBotAPI(t)

	err := newTestPushSender(api).EditPushText(t.Context(), 42, 501, "И ещё 4 лайка за сегодня",
		[]PushButton{{Text: "Посмотреть", StartParam: "incoming"}})
	if err != nil {
		t.Fatalf("EditPushText() failed: %v", err)
	}

	call := api.got("editMessageText")
	if call == nil {
		t.Fatal("editMessageText was never called")
	}
	if got := call.body["message_id"]; got != float64(501) {
		t.Errorf("message_id = %v, want 501", got)
	}
	if got := call.body["text"]; got != "И ещё 4 лайка за сегодня" {
		t.Errorf("text = %v", got)
	}
	if api.got("sendMessage") != nil {
		t.Error("the tail was sent as a new message instead of an edit")
	}
}

func TestEditPushTextIgnoresAnUnchangedMessage(t *testing.T) {
	api := newBotAPI(t)
	api.replies["editMessageText"] =
		`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`

	err := newTestPushSender(api).EditPushText(t.Context(), 42, 501, "И ещё 4 лайка за сегодня", nil)
	if err != nil {
		t.Errorf("EditPushText() = %v, want nil for an unmodified message", err)
	}
}

func TestBlockedChatsAreRecognisedByDescription(t *testing.T) {
	cases := []struct {
		name    string
		code    int
		reason  string
		blocked bool
	}{
		{"blocked", 403, "Forbidden: bot was blocked by the user", true},
		{"unknown chat", 400, "Bad Request: chat not found", true},
		{"deactivated", 403, "Forbidden: user is deactivated", true},
		{"too many requests", 429, "Too Many Requests: retry after 5", false},
		{"broken request", 400, "Bad Request: message text is empty", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newBotAPI(t)
			api.replies["sendMessage"] = fmt.Sprintf(`{"ok":false,"error_code":%d,"description":%q}`,
				tc.code, tc.reason)

			_, err := newTestPushSender(api).SendPush(t.Context(), 42, "Тебя лайкнул Марк", nil)
			if err == nil {
				t.Fatal("SendPush() succeeded although Telegram refused")
			}
			if got := errors.Is(err, ErrBlocked); got != tc.blocked {
				t.Errorf("errors.Is(err, ErrBlocked) = %v for %q, want %v", got, tc.reason, tc.blocked)
			}
		})
	}
}

func TestMixedButtonsSplitIntoAnswerAndLinkRows(t *testing.T) {
	api := newBotAPI(t)
	api.replies["sendMessage"] = `{"ok":true,"result":{"message_id":7}}`

	_, err := newTestPushSender(api).SendPush(t.Context(), 42, "Комната ещё свободна?", []PushButton{
		{Text: "Да", CallbackData: "pnt:alive:x"},
		{Text: "Нашёл соседа", CallbackData: "pnt:gone:x"},
		{Text: "Открыть", StartParam: "mine"},
	})
	if err != nil {
		t.Fatalf("SendPush() failed: %v", err)
	}

	markup, ok := api.got("sendMessage").body["reply_markup"].(map[string]any)
	if !ok {
		t.Fatal("the message has no keyboard")
	}
	rows, ok := markup["inline_keyboard"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("keyboard = %v, want the answers row and the link row", markup)
	}
	if answers, ok := rows[0].([]any); !ok || len(answers) != 2 {
		t.Errorf("first row = %v, want the two callback answers", rows[0])
	}
	link, _ := rows[1].([]any)
	if len(link) != 1 {
		t.Fatalf("second row = %v, want the single link", rows[1])
	}
	button, _ := link[0].(map[string]any)
	if button["web_app"] == nil || button["callback_data"] != nil {
		t.Errorf("link button = %v, want web_app without callback_data", button)
	}
}

func onlyButton(t *testing.T, body map[string]any) map[string]any {
	t.Helper()

	markup, ok := body["reply_markup"].(map[string]any)
	if !ok {
		t.Fatalf("the message has no keyboard: %v", body)
	}
	rows, ok := markup["inline_keyboard"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("keyboard = %v, want one row", markup)
	}
	row, ok := rows[0].([]any)
	if !ok || len(row) != 1 {
		t.Fatalf("row = %v, want one button", rows[0])
	}
	button, ok := row[0].(map[string]any)
	if !ok {
		t.Fatalf("button = %v", row[0])
	}
	return button
}
