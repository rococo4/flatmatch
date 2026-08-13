package photostore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathResolvesStoredFile(t *testing.T) {
	dir, err := NewDir(t.TempDir(), "https://example.org/photos")
	if err != nil {
		t.Fatalf("NewDir() failed: %v", err)
	}

	const key = "photos/user.jpg"
	if err := dir.Put(t.Context(), key, strings.NewReader("jpeg"), "image/jpeg"); err != nil {
		t.Fatalf("Put() failed: %v", err)
	}

	name, err := dir.Path(key)
	if err != nil {
		t.Fatalf("Path() failed: %v", err)
	}
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if string(body) != "jpeg" {
		t.Errorf("file holds %q, want the stored bytes", body)
	}
	if !strings.HasPrefix(name, filepath.Clean(dir.Root())) {
		t.Errorf("Path() = %q, want a file inside %q", name, dir.Root())
	}
}

// Ключ приходит из базы, а путь по нему собирается на диске: модерационная
// отправка читает этот файл, поэтому ../ обязан остаться внутри хранилища.
func TestPathStaysInsideTheStore(t *testing.T) {
	root := t.TempDir()
	dir, err := NewDir(root, "https://example.org/photos")
	if err != nil {
		t.Fatalf("NewDir() failed: %v", err)
	}
	name, err := dir.Path("../../etc/passwd")
	if err != nil {
		t.Fatalf("Path() failed: %v", err)
	}
	if !strings.HasPrefix(name, filepath.Clean(root)+string(os.PathSeparator)) {
		t.Errorf("Path() = %q, want a file under %q", name, root)
	}
}
