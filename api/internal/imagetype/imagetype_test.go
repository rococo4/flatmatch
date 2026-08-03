package imagetype

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

var (
	jpeg = []byte("\xff\xd8\xff\xe0")
	png  = []byte("\x89PNG\r\n\x1a\n")
	webp = []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	gif  = []byte("GIF87a")
)

func TestSniffAcceptsWhatBrowsersCanRender(t *testing.T) {
	cases := []struct {
		name        string
		head        []byte
		contentType string
		ext         string
	}{
		{"jpeg", jpeg, "image/jpeg", ".jpg"},
		{"png", png, "image/png", ".png"},
		{"webp", webp, "image/webp", ".webp"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			contentType, ext, err := Sniff(bytes.NewReader(c.head))
			if err != nil {
				t.Fatalf("Sniff() failed: %v", err)
			}
			if contentType != c.contentType {
				t.Errorf("contentType = %q, want %q", contentType, c.contentType)
			}
			if ext != c.ext {
				t.Errorf("ext = %q, want %q", ext, c.ext)
			}
		})
	}
}

func TestSniffRejectsAnythingElse(t *testing.T) {
	// A gif is an image and still refused: the set is what the product stores,
	// not what happens to be a picture. Text is the ordinary case — a renamed
	// document, or an upload that never was an image.
	for name, body := range map[string][]byte{
		"gif":   gif,
		"text":  []byte("это не картинка"),
		"empty": nil,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Sniff(bytes.NewReader(body)); !errors.Is(err, ErrUnsupported) {
				t.Fatalf("Sniff() error = %v, want ErrUnsupported", err)
			}
		})
	}
}

func TestSniffRewindsSoTheFileCanStillBeStored(t *testing.T) {
	// Without the rewind the first 512 bytes would be missing from whatever is
	// written to storage, and the stored photo would be corrupt.
	body := append(jpeg, bytes.Repeat([]byte("x"), 600)...)
	file := bytes.NewReader(body)

	if _, _, err := Sniff(file); err != nil {
		t.Fatalf("Sniff() failed: %v", err)
	}

	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("reading the file after Sniff() failed: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("read %d bytes after Sniff(), want all %d", len(got), len(body))
	}
}
