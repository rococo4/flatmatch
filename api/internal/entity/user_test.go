package entity

import "testing"

func TestParseStatus(t *testing.T) {
	tests := []struct {
		raw  string
		want Status
	}{
		{"draft", StatusDraft},
		{"pending", StatusPending},
		{"active", StatusActive},
		{"banned", StatusBanned},

		// A value from a schema newer than this binary.
		{"something_new", StatusUnknown},
		{"", StatusUnknown},
		// 'unknown' is a code-side marker, never something the database holds,
		// so reading it back is just as much of a stranger as any other value.
		{"unknown", StatusUnknown},
		// Case matters — the column stores lower case.
		{"Active", StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := ParseStatus(tt.raw); got != tt.want {
				t.Errorf("ParseStatus(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParsePhotoStatus(t *testing.T) {
	tests := []struct {
		raw  string
		want PhotoStatus
	}{
		{"pending", PhotoPending},
		{"ok", PhotoOK},
		{"rejected", PhotoRejected},
		{"something_new", PhotoUnknown},
		{"", PhotoUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			if got := ParsePhotoStatus(tt.raw); got != tt.want {
				t.Errorf("ParsePhotoStatus(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestVisibleIsAWhitelist(t *testing.T) {
	// Only an approved account is visible. Everything else — including a status
	// this code cannot interpret — is hidden, because wrongly showing a banned
	// account is worse than wrongly hiding anyone.
	visible := map[Status]bool{
		StatusActive:  true,
		StatusDraft:   false,
		StatusPending: false,
		StatusBanned:  false,
		StatusUnknown: false,
		"invented":    false,
	}

	for status, want := range visible {
		if got := (User{Status: status}).Visible(); got != want {
			t.Errorf("User{Status: %q}.Visible() = %v, want %v", status, got, want)
		}
	}
}
