package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"

	"flatmatch/api/internal/config"
	"flatmatch/api/internal/tma"
)

func main() {
	var (
		tgID     = flag.Int64("id", 777000001, "Telegram user id to impersonate")
		name     = flag.String("name", "Арсений", "first name")
		lastName = flag.String("last-name", "", "last name")
		username = flag.String("username", "dev_user", "Telegram @username, without the @")
		photoURL = flag.String("photo", "", "photo URL")
		age      = flag.Duration("age", 0, "how long ago the data was issued, e.g. 25h to test expiry")
	)
	flag.Parse()

	if err := config.LoadDotEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "BOT_TOKEN is not set — put it in .env or the environment")
		os.Exit(1)
	}

	// Built from the same struct the validator parses, so the two cannot drift.
	user, err := json.Marshal(tma.User{
		ID:           *tgID,
		FirstName:    *name,
		LastName:     *lastName,
		Username:     *username,
		PhotoURL:     *photoURL,
		LanguageCode: "ru",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build user payload: %v\n", err)
		os.Exit(1)
	}

	params := url.Values{
		"user":          {string(user)},
		"chat_instance": {"-1234567890123456789"},
		"chat_type":     {"private"},
		"auth_date":     {strconv.FormatInt(time.Now().Add(-*age).Unix(), 10)},

		// Telegram signs launch data twice: HMAC in `hash`, which is what this
		// tool and the API actually check, and Ed25519 in `signature`, which
		// only Telegram's private key can produce.
		//
		// The value here is therefore a placeholder — but the field has to be
		// present, because @telegram-apps/sdk-react refuses launch params
		// without it, and browser development runs on exactly this data. It
		// also lands inside the data-check-string, so `hash` covers it and the
		// two sides stay consistent.
		"signature": {"devinitdata-is-not-signed-by-telegram"},
	}

	fmt.Println(tma.Sign(token, params))
}
