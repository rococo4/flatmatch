package tma

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
)

func Sign(botToken string, params url.Values) string {
	signed := url.Values{}
	for key, values := range params {
		if key == "hash" {
			continue
		}
		signed[key] = values
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))

	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(dataCheckString(signed)))

	signed.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return signed.Encode()
}
