package authmw

import (
	"net/http"
	"strings"
)

func initDataFromHeader(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}

	prefix, value, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(prefix, scheme) {
		return "", false
	}

	value = strings.TrimSpace(value)
	return value, value != ""
}
