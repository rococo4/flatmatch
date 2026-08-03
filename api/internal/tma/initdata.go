package tma

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const clockSkew = 5 * time.Minute

type User struct {
	ID              int64  `json:"id"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name,omitempty"`
	Username        string `json:"username,omitempty"`
	LanguageCode    string `json:"language_code,omitempty"`
	PhotoURL        string `json:"photo_url,omitempty"`
	IsPremium       bool   `json:"is_premium,omitempty"`
	AllowsWriteToPM bool   `json:"allows_write_to_pm,omitempty"`
}

type InitData struct {
	User       User
	AuthDate   time.Time
	QueryID    string
	StartParam string
	Raw        string
}

type Validator struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

var excludedFromHash = []string{"hash"}

// Sentinel errors. Callers branch on these; the surrounding text is for logs.
var (
	// ErrMalformed means the input is not usable initData at all.
	ErrMalformed = errors.New("initData is malformed")
	// ErrMissingHash means the `hash` field is absent — nothing to verify.
	ErrMissingHash = errors.New("initData has no hash")
	// ErrBadHash means the signature does not match. Either the payload was
	// tampered with, or it was signed by a different bot.
	ErrBadHash = errors.New("initData signature does not match")
	// ErrExpired means auth_date is outside the accepted window. The remedy is
	// the same in both directions: reopen the Mini App.
	ErrExpired = errors.New("initData auth_date is outside the accepted window")
	// ErrMissingUser means there is no identifiable user in the payload.
	ErrMissingUser = errors.New("initData has no user")
)

func NewValidator(botToken string, ttl time.Duration) *Validator {
	mac := hmac.New(sha256.New, []byte("WebAppData"))
	mac.Write([]byte(botToken))

	return &Validator{
		secret: mac.Sum(nil),
		ttl:    ttl,
		now:    time.Now,
	}
}

func (v *Validator) Validate(raw string) (InitData, error) {
	if raw == "" {
		return InitData{}, fmt.Errorf("%w: empty", ErrMalformed)
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return InitData{}, fmt.Errorf("%w: not a query string", ErrMalformed)
	}

	for key, vals := range values {
		if len(vals) > 1 {
			return InitData{}, fmt.Errorf("%w: field %q appears %d times", ErrMalformed, key, len(vals))
		}
	}

	wantHash := values.Get("hash")
	if wantHash == "" {
		return InitData{}, ErrMissingHash
	}
	wantMAC, err := hex.DecodeString(wantHash)
	if err != nil {
		return InitData{}, fmt.Errorf("%w: hash is not hex", ErrMalformed)
	}

	if !hmac.Equal(wantMAC, v.sign(values)) {
		return InitData{}, ErrBadHash
	}

	authDate, err := parseAuthDate(values.Get("auth_date"))
	if err != nil {
		return InitData{}, err
	}
	if err := v.checkFreshness(authDate); err != nil {
		return InitData{}, err
	}

	rawUser := values.Get("user")
	if rawUser == "" {
		return InitData{}, ErrMissingUser
	}
	var user User
	if err := json.Unmarshal([]byte(rawUser), &user); err != nil {
		return InitData{}, fmt.Errorf("%w: user is not valid JSON", ErrMalformed)
	}
	if user.ID == 0 {
		return InitData{}, fmt.Errorf("%w: user has no id", ErrMissingUser)
	}

	return InitData{
		User:       user,
		AuthDate:   authDate,
		QueryID:    values.Get("query_id"),
		StartParam: values.Get("start_param"),
		Raw:        raw,
	}, nil
}

func (v *Validator) sign(values url.Values) []byte {
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(dataCheckString(values)))
	return mac.Sum(nil)
}

func (v *Validator) checkFreshness(authDate time.Time) error {
	now := v.now()
	if age := now.Sub(authDate); age > v.ttl {
		return fmt.Errorf("%w: issued %s ago, limit is %s", ErrExpired, age.Round(time.Second), v.ttl)
	}
	if authDate.After(now.Add(clockSkew)) {
		return fmt.Errorf("%w: issued in the future (%s ahead)", ErrExpired, authDate.Sub(now).Round(time.Second))
	}
	return nil
}

func parseAuthDate(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, fmt.Errorf("%w: no auth_date", ErrMalformed)
	}
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: auth_date is not a unix timestamp", ErrMalformed)
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func dataCheckString(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if !slices.Contains(excludedFromHash, key) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)

	var b strings.Builder
	for i, key := range keys {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(values.Get(key))
	}
	return b.String()
}
