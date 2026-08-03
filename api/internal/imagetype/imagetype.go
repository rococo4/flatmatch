// Package imagetype recognises the picture formats the product accepts.
//
// It exists apart from any one upload because there is more than one: the
// profile photo today (§4 шаг 11), the screenshot of a listing and the photo of
// a room once points arrive (§6). What counts as an image should not be decided
// twice, in two handlers, slightly differently.
package imagetype

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

var accepted = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var ErrUnsupported = errors.New("unsupported image type")

func Sniff(file io.ReadSeeker) (contentType, ext string, err error) {
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", "", fmt.Errorf("read the head of the file: %w", err)
	}

	contentType = http.DetectContentType(head[:n])
	ext, ok := accepted[contentType]
	if !ok {
		return "", "", fmt.Errorf("%w: %s", ErrUnsupported, contentType)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", fmt.Errorf("rewind the file: %w", err)
	}
	return contentType, ext, nil
}
