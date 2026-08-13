// Package photostore keeps profile photos.
//
// The implementation here writes to a directory on disk. It exists so the rest
// of the product can be built before object storage is set up: the interface it
// satisfies is declared by service/users, and the eventual S3 version replaces
// it by changing one line in cmd/api/main.go.
//
// Nothing outside this package knows where a photo lives. The database holds a
// key; turning that into an address is this package's job, which is what keeps
// a local host name out of the rows.
package photostore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Dir struct {
	root    string
	baseURL string
}

func NewDir(root, baseURL string) (*Dir, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, fmt.Errorf("create photo directory %s: %w", root, err)
	}
	return &Dir{root: root, baseURL: strings.TrimSuffix(baseURL, "/")}, nil
}

func (d *Dir) Put(_ context.Context, key string, r io.Reader, _ string) error {
	name, err := d.Path(key)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		return fmt.Errorf("create directory for %s: %w", key, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(name), ".upload-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", key, err)
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()

	if _, err := io.Copy(tmp, r); err != nil {
		return fmt.Errorf("write %s: %w", key, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", key, err)
	}
	if err := os.Rename(tmp.Name(), name); err != nil {
		return fmt.Errorf("publish %s: %w", key, err)
	}
	return nil
}

func (d *Dir) Delete(_ context.Context, key string) error {
	name, err := d.Path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}

func (d *Dir) URL(key string) string {
	if key == "" {
		return ""
	}
	return d.baseURL + "/" + key
}

func (d *Dir) Root() string { return d.root }

// Path is the local file behind a key. Telegram cannot fetch a photo from a
// стенд that is not on the public internet, so the moderation chat uploads the
// bytes instead of sending a URL (§5).
func (d *Dir) Path(key string) (string, error) {
	clean := path.Clean("/" + key)
	name := filepath.Join(d.root, filepath.FromSlash(clean))

	if !strings.HasPrefix(name, filepath.Clean(d.root)+string(os.PathSeparator)) {
		return "", fmt.Errorf("photo key %q escapes the store", key)
	}
	return name, nil
}
