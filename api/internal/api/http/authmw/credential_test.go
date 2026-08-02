package authmw

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInitDataFromHeader(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		wantOK bool
	}{
		{"our scheme", "tma auth_date=1&hash=ab", "auth_date=1&hash=ab", true},
		// RFC 9110 makes the scheme case-insensitive, and clients do vary.
		{"scheme upper case", "TMA auth_date=1", "auth_date=1", true},
		{"scheme mixed case", "TmA auth_date=1", "auth_date=1", true},
		{"padded value", "tma   auth_date=1  ", "auth_date=1", true},

		{"no header at all", "", "", false},
		{"another scheme", "Bearer eyJhbGciOi", "", false},
		{"scheme without a value", "tma", "", false},
		{"scheme with a blank value", "tma   ", "", false},
		// A bare credential is not something to salvage — accepting it would
		// mean the scheme is decoration rather than part of the contract.
		{"value without a scheme", "auth_date=1&hash=ab", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}

			got, ok := initDataFromHeader(r)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Errorf("value = %q, want %q", got, tt.want)
			}
		})
	}
}
