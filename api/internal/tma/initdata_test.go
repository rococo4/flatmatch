package tma

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testToken = "7000000000:AAHtesttesttesttesttesttesttesttest0"

var fixedNow = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

func newTestValidator(t *testing.T, ttl time.Duration) *Validator {
	t.Helper()
	v := NewValidator(testToken, ttl)
	v.now = func() time.Time { return fixedNow }
	return v
}

func validParams() url.Values {
	return url.Values{
		"user":          {`{"id":42,"first_name":"Арсений","last_name":"Лебедев","username":"arsenii","language_code":"ru","photo_url":"https://t.me/i/userpic/320/arsenii.jpg"}`},
		"chat_instance": {"-1234567890123456789"},
		"chat_type":     {"private"},
		"auth_date":     {strconv.FormatInt(fixedNow.Unix(), 10)},
	}
}

func TestValidateAcceptsGenuineInitData(t *testing.T) {
	v := newTestValidator(t, 24*time.Hour)
	raw := Sign(testToken, validParams())

	data, err := v.Validate(raw)
	if err != nil {
		t.Fatalf("Validate() returned error for genuine initData: %v", err)
	}

	if got, want := data.User.ID, int64(42); got != want {
		t.Errorf("User.ID = %d, want %d", got, want)
	}

	if got, want := data.User.FirstName, "Арсений"; got != want {
		t.Errorf("User.FirstName = %q, want %q", got, want)
	}
	if got, want := data.User.LastName, "Лебедев"; got != want {
		t.Errorf("User.LastName = %q, want %q", got, want)
	}
	if got, want := data.User.Username, "arsenii"; got != want {
		t.Errorf("User.Username = %q, want %q", got, want)
	}
	if got, want := data.User.PhotoURL, "https://t.me/i/userpic/320/arsenii.jpg"; got != want {
		t.Errorf("User.PhotoURL = %q, want %q", got, want)
	}
	if !data.AuthDate.Equal(fixedNow) {
		t.Errorf("AuthDate = %s, want %s", data.AuthDate, fixedNow)
	}
	if data.Raw != raw {
		t.Errorf("Raw was not preserved verbatim")
	}
}

const (
	externallySignedInitData = "auth_date=1785672000" +
		"&chat_instance=-1234567890123456789" +
		"&chat_type=private" +
		"&hash=56b5a9bc61d71e95749f1d0f65277317d95f457cd9c21ae8915640c8003a2068" +
		"&query_id=AAE80EMLAAAAADzQQws6Dwtk" +
		"&signature=EV1DQf_OTaQ1Mr3hMfh0JdlvCWjDVYii2clsYvaTk4HW318Olj4cngnSVLpDpPJ_BRv13rXSUupOHadYXzxeCA" +
		"&user=%7B%22id%22%3A188993596%2C%22first_name%22%3A%22%D0%A1%D0%B5%D0%BD%D1%8F%22%2C" +
		"%22last_name%22%3A%22%22%2C%22username%22%3A%22rococo_4%22%2C%22language_code%22%3A%22ru%22%2C" +
		"%22is_premium%22%3Atrue%2C%22allows_write_to_pm%22%3Atrue%2C" +
		"%22photo_url%22%3A%22https%3A%2F%2Ft.me%2Fi%2Fuserpic%2F320%2Fx.svg%22%7D"

	externallySignedUserID int64 = 188993596
)

func TestValidateAcceptsExternallySignedInitData(t *testing.T) {
	data, err := newTestValidator(t, 24*time.Hour).Validate(externallySignedInitData)
	if err != nil {
		t.Fatalf("Validate() rejected initData signed outside this package: %v", err)
	}

	if got := data.User.ID; got != externallySignedUserID {
		t.Errorf("User.ID = %d, want %d", got, externallySignedUserID)
	}
	if got, want := data.User.FirstName, "Сеня"; got != want {
		t.Errorf("User.FirstName = %q, want %q", got, want)
	}
	if got, want := data.QueryID, "AAE80EMLAAAAADzQQws6Dwtk"; got != want {
		t.Errorf("QueryID = %q, want %q", got, want)
	}
	if !data.User.IsPremium {
		t.Error("IsPremium was not decoded")
	}
}

func TestValidateRejectsATamperedSignature(t *testing.T) {
	tampered := strings.Replace(externallySignedInitData, "&signature=EV1D", "&signature=XXXX", 1)
	if tampered == externallySignedInitData {
		t.Fatal("the fixture no longer contains the signature this test edits")
	}

	if _, err := newTestValidator(t, 24*time.Hour).Validate(tampered); !errors.Is(err, ErrBadHash) {
		t.Fatalf("Validate() error = %v, want ErrBadHash", err)
	}
}

func TestValidateRejectsTamperedPayload(t *testing.T) {
	params := validParams()
	raw := Sign(testToken, params)

	tampered, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("ParseQuery(%q) failed: %v", raw, err)
	}
	tampered.Set("user", `{"id":999,"first_name":"Mallory"}`)

	if _, err := newTestValidator(t, 24*time.Hour).Validate(tampered.Encode()); !errors.Is(err, ErrBadHash) {
		t.Fatalf("Validate() error = %v, want ErrBadHash", err)
	}
}

func TestValidateRejectsForeignToken(t *testing.T) {
	raw := Sign("7000000001:AAHotherotherotherotherotherotherot", validParams())

	if _, err := newTestValidator(t, 24*time.Hour).Validate(raw); !errors.Is(err, ErrBadHash) {
		t.Fatalf("Validate() error = %v, want ErrBadHash", err)
	}
}

func TestValidateRejectsExpiredAuthDate(t *testing.T) {
	params := validParams()
	params.Set("auth_date", strconv.FormatInt(fixedNow.Add(-25*time.Hour).Unix(), 10))

	_, err := newTestValidator(t, 24*time.Hour).Validate(Sign(testToken, params))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Validate() error = %v, want ErrExpired", err)
	}
}

func TestValidateAcceptsAuthDateJustInsideTTL(t *testing.T) {
	params := validParams()
	params.Set("auth_date", strconv.FormatInt(fixedNow.Add(-23*time.Hour).Unix(), 10))

	if _, err := newTestValidator(t, 24*time.Hour).Validate(Sign(testToken, params)); err != nil {
		t.Fatalf("Validate() rejected initData still inside its TTL: %v", err)
	}
}

func TestValidateRejectsFutureAuthDate(t *testing.T) {
	params := validParams()
	params.Set("auth_date", strconv.FormatInt(fixedNow.Add(time.Hour).Unix(), 10))

	_, err := newTestValidator(t, 24*time.Hour).Validate(Sign(testToken, params))
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("Validate() error = %v, want ErrExpired", err)
	}
}

func TestValidateToleratesSmallClockSkew(t *testing.T) {
	params := validParams()
	params.Set("auth_date", strconv.FormatInt(fixedNow.Add(time.Minute).Unix(), 10))

	if _, err := newTestValidator(t, 24*time.Hour).Validate(Sign(testToken, params)); err != nil {
		t.Fatalf("Validate() rejected initData within the skew allowance: %v", err)
	}
}

func TestValidateRejectsMalformedInput(t *testing.T) {
	withoutHash := validParams()

	badUserJSON := validParams()
	badUserJSON.Set("user", "{not json")

	noUser := validParams()
	noUser.Del("user")

	noAuthDate := validParams()
	noAuthDate.Del("auth_date")

	textAuthDate := validParams()
	textAuthDate.Set("auth_date", "вчера")

	zeroUserID := validParams()
	zeroUserID.Set("user", `{"id":0,"first_name":"Nobody"}`)

	tests := []struct {
		name string
		raw  string
		want error
	}{
		{"empty string", "", ErrMalformed},
		{"not a query string", "%%%", ErrMalformed},
		{"no hash field", withoutHash.Encode(), ErrMissingHash},
		{"hash is not hex", "auth_date=1&hash=zzzz", ErrMalformed},
		{"duplicate field", Sign(testToken, validParams()) + "&auth_date=1", ErrMalformed},
		{"user is not JSON", Sign(testToken, badUserJSON), ErrMalformed},
		{"no user field", Sign(testToken, noUser), ErrMissingUser},
		{"user has no id", Sign(testToken, zeroUserID), ErrMissingUser},
		{"no auth_date field", Sign(testToken, noAuthDate), ErrMalformed},
		{"auth_date is not a number", Sign(testToken, textAuthDate), ErrMalformed},
	}

	v := newTestValidator(t, 24*time.Hour)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := v.Validate(tt.raw); !errors.Is(err, tt.want) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestValidateDoesNotLeakTheToken(t *testing.T) {
	_, err := newTestValidator(t, 24*time.Hour).Validate(Sign("7000000001:AAHwrong", validParams()))
	if err == nil {
		t.Fatal("Validate() unexpectedly succeeded")
	}
	if got := err.Error(); got == "" || strings.Contains(got, testToken) {
		t.Fatalf("error message leaks the bot token: %q", got)
	}
}
