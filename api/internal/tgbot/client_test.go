package tgbot

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

type apiCall struct {
	method   string
	body     map[string]any
	fields   map[string]string
	file     []byte
	filename string
}

type botAPI struct {
	t       *testing.T
	server  *httptest.Server
	replies map[string]string

	mu    sync.Mutex
	calls []apiCall
}

func newBotAPI(t *testing.T) *botAPI {
	t.Helper()
	api := &botAPI{t: t, replies: map[string]string{}}
	api.server = httptest.NewServer(http.HandlerFunc(api.serve))
	t.Cleanup(api.server.Close)
	return api
}

func (a *botAPI) client() *Client {
	c := NewClient("secret-token")
	c.base = a.server.URL
	return c
}

func (a *botAPI) serve(w http.ResponseWriter, r *http.Request) {
	call := apiCall{method: path.Base(r.URL.Path)}

	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			a.t.Errorf("parse multipart %s: %v", call.method, err)
		} else {
			call.fields = map[string]string{}
			for name, values := range r.MultipartForm.Value {
				call.fields[name] = values[0]
			}
			if file, header, err := r.FormFile("photo"); err == nil {
				defer file.Close()
				call.filename = header.Filename
				call.file, _ = io.ReadAll(file)
			}
		}
	} else if err := json.NewDecoder(r.Body).Decode(&call.body); err != nil {
		a.t.Errorf("decode %s body: %v", call.method, err)
	}

	a.mu.Lock()
	a.calls = append(a.calls, call)
	reply, ok := a.replies[call.method]
	a.mu.Unlock()

	if !ok {
		reply = `{"ok":true,"result":{}}`
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, reply)
}

func (a *botAPI) got(method string) *apiCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.calls {
		if a.calls[i].method == method {
			return &a.calls[i]
		}
	}
	return nil
}

func (a *botAPI) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

func TestSendPhotoFileUploadsTheFileItself(t *testing.T) {
	api := newBotAPI(t)
	api.replies["sendPhoto"] = `{"ok":true,"result":{"message_id":777}}`

	photo := filepath.Join(t.TempDir(), "room.jpg")
	want := []byte("\xff\xd8\xff\xe0 такая себе фотография")
	if err := os.WriteFile(photo, want, 0o600); err != nil {
		t.Fatalf("write photo: %v", err)
	}

	kb := keyboard([][2]string{{"ок", "mod:u:ok:1"}, {"фото не ок", "mod:u:photo:1"}})
	msgID, err := api.client().SendPhotoFile(t.Context(), -100500, photo, "Катя, 24", kb)
	if err != nil {
		t.Fatalf("SendPhotoFile() failed: %v", err)
	}
	if msgID != 777 {
		t.Errorf("message id = %d, want 777", msgID)
	}

	call := api.got("sendPhoto")
	if call == nil {
		t.Fatal("sendPhoto was never called")
	}
	if !bytes.Equal(call.file, want) {
		t.Errorf("uploaded %q, want %q", call.file, want)
	}
	if call.filename != "room.jpg" {
		t.Errorf("filename = %q, want room.jpg", call.filename)
	}
	if got := call.fields["chat_id"]; got != "-100500" {
		t.Errorf("chat_id = %q, want -100500", got)
	}
	if got := call.fields["caption"]; got != "Катя, 24" {
		t.Errorf("caption = %q, want %q", got, "Катя, 24")
	}

	var markup InlineKeyboard
	if err := json.Unmarshal([]byte(call.fields["reply_markup"]), &markup); err != nil {
		t.Fatalf("reply_markup %q is not a keyboard: %v", call.fields["reply_markup"], err)
	}
	if len(markup.Rows) != 1 || len(markup.Rows[0]) != 2 {
		t.Fatalf("keyboard = %+v, want one row of two", markup.Rows)
	}

	if got := markup.Rows[0][0].Data; got != "mod:u:ok:1" {
		t.Errorf("callback_data = %q, want mod:u:ok:1", got)
	}
}

func TestSendPhotoFileWithoutTheFileTouchesNothing(t *testing.T) {
	api := newBotAPI(t)

	_, err := api.client().SendPhotoFile(t.Context(), 1, filepath.Join(t.TempDir(), "gone.jpg"), "", nil)
	if err == nil {
		t.Fatal("SendPhotoFile() accepted a missing file")
	}
	if api.count() != 0 {
		t.Errorf("%d calls made, want none", api.count())
	}
}

func TestEditReplyMarkupClearsTheButtons(t *testing.T) {
	api := newBotAPI(t)

	if err := api.client().EditReplyMarkup(t.Context(), -100500, 777, nil); err != nil {
		t.Fatalf("EditReplyMarkup() failed: %v", err)
	}

	call := api.got("editMessageReplyMarkup")
	if call == nil {
		t.Fatal("editMessageReplyMarkup was never called")
	}
	if got := call.body["message_id"]; got != float64(777) {
		t.Errorf("message_id = %v, want 777", got)
	}

	markup, ok := call.body["reply_markup"].(map[string]any)
	if !ok {
		t.Fatalf("reply_markup = %v, want an object", call.body["reply_markup"])
	}
	if rows, ok := markup["inline_keyboard"].([]any); !ok || len(rows) != 0 {
		t.Errorf("inline_keyboard = %v, want an empty array", markup["inline_keyboard"])
	}
}

func TestAnswerCallbackCarriesTheAck(t *testing.T) {
	api := newBotAPI(t)

	if err := api.client().AnswerCallback(t.Context(), "q-1", "уже обработано"); err != nil {
		t.Fatalf("AnswerCallback() failed: %v", err)
	}

	call := api.got("answerCallbackQuery")
	if call == nil {
		t.Fatal("answerCallbackQuery was never called")
	}
	if got := call.body["callback_query_id"]; got != "q-1" {
		t.Errorf("callback_query_id = %v, want q-1", got)
	}
	if got := call.body["text"]; got != "уже обработано" {
		t.Errorf("text = %v, want %q", got, "уже обработано")
	}
}

func TestGetUpdatesSubscribesToButtonTaps(t *testing.T) {
	api := newBotAPI(t)
	api.replies["getUpdates"] = `{"ok":true,"result":[]}`

	if _, err := api.client().GetUpdates(t.Context(), 0); err != nil {
		t.Fatalf("GetUpdates() failed: %v", err)
	}

	call := api.got("getUpdates")
	if call == nil {
		t.Fatal("getUpdates was never called")
	}
	allowed, _ := call.body["allowed_updates"].([]any)
	var kinds []string
	for _, a := range allowed {
		kinds = append(kinds, a.(string))
	}
	if !slices.Contains(kinds, "callback_query") || !slices.Contains(kinds, "message") {
		t.Errorf("allowed_updates = %v, want message and callback_query", kinds)
	}
}
