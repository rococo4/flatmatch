package tgbot

import (
	"bytes"
	"encoding/json"
	"fmt"
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
	files    map[string][]byte
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
			call.files = map[string][]byte{}
			for name, headers := range r.MultipartForm.File {
				file, err := headers[0].Open()
				if err != nil {
					a.t.Errorf("open uploaded %s: %v", name, err)
					continue
				}
				call.files[name], _ = io.ReadAll(file)
				file.Close()

				if name == "photo" {
					call.filename = headers[0].Filename
					call.file = call.files[name]
				}
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

func TestSendMediaGroupUploadsEveryPhoto(t *testing.T) {
	api := newBotAPI(t)
	dir := t.TempDir()

	var paths []string
	for _, name := range []string{"a.jpg", "b.jpg"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("\xff\xd8\xff\xe0"+name), 0o600); err != nil {
			t.Fatalf("write photo: %v", err)
		}
		paths = append(paths, path)
	}

	if err := api.client().SendMediaGroupFiles(t.Context(), -100500, paths); err != nil {
		t.Fatalf("SendMediaGroupFiles() failed: %v", err)
	}

	call := api.got("sendMediaGroup")
	if call == nil {
		t.Fatal("sendMediaGroup was never called")
	}
	if got := call.fields["chat_id"]; got != "-100500" {
		t.Errorf("chat_id = %q, want -100500", got)
	}

	var media []map[string]string
	if err := json.Unmarshal([]byte(call.fields["media"]), &media); err != nil {
		t.Fatalf("media %q is not an album: %v", call.fields["media"], err)
	}
	if len(media) != 2 {
		t.Fatalf("album holds %d items, want 2", len(media))
	}
	for i, item := range media {
		if item["type"] != "photo" {
			t.Errorf("item %d type = %q, want photo", i, item["type"])
		}
		if want := fmt.Sprintf("attach://photo%d", i); item["media"] != want {
			t.Errorf("item %d media = %q, want %q", i, item["media"], want)
		}
	}
	if len(call.files) != 2 || string(call.files["photo1"]) != "\xff\xd8\xff\xe0b.jpg" {
		t.Errorf("uploaded files = %v, want both photos attached by name", call.files)
	}
}

func TestSendMediaGroupWithAMissingFileTouchesNothing(t *testing.T) {
	api := newBotAPI(t)

	err := api.client().SendMediaGroupFiles(t.Context(), 1, []string{filepath.Join(t.TempDir(), "gone.jpg")})
	if err == nil {
		t.Fatal("SendMediaGroupFiles() accepted a missing file")
	}
	if api.count() != 0 {
		t.Errorf("%d calls made, want none", api.count())
	}
}

func TestModerationSenderKeepsOneVerdictPerCard(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 0, 3)
	for _, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("\xff\xd8\xff\xe0"), 0o600); err != nil {
			t.Fatalf("write photo: %v", err)
		}
		paths = append(paths, path)
	}
	buttons := [][2]string{{"ок", "mod:u:ok:1"}, {"фото не ок", "mod:u:photo:1"}}

	cases := []struct {
		name   string
		paths  []string
		photos int
		albums int
	}{
		{"one photo", paths[:1], 1, 0},
		{"two photos", paths[:2], 2, 0},
		{"three photos", paths, 1, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api := newBotAPI(t)
			api.replies["sendPhoto"] = `{"ok":true,"result":{"message_id":777}}`

			msgID, err := NewModerationSender(api.client()).
				SendModerationPhotos(t.Context(), -100500, c.paths, "Катя, 24", buttons)
			if err != nil {
				t.Fatalf("SendModerationPhotos() failed: %v", err)
			}
			if msgID != 777 {
				t.Errorf("message id = %d, want the card with the buttons", msgID)
			}

			var photos, albums, withButtons int
			for _, call := range api.calls {
				switch call.method {
				case "sendPhoto":
					photos++
					if call.fields["reply_markup"] != "" {
						withButtons++
					}
				case "sendMediaGroup":
					albums++
				}
			}
			if photos != c.photos || albums != c.albums {
				t.Errorf("sent %d photos and %d albums, want %d and %d",
					photos, albums, c.photos, c.albums)
			}
			if withButtons != 1 {
				t.Errorf("%d messages carry verdict buttons, want exactly one", withButtons)
			}
		})
	}
}
