package notifications

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"flatmatch/api/internal/entity"
	"flatmatch/api/internal/tgbot"
)

const (
	opStillSearching = "yes"
	opFound          = "found"
)

const (
	ackSearching     = "✓ Ищу дальше"
	ackRevived       = "✓ Поиск снова активен"
	ackNoSlots       = "Свободных поисков нет — закрой один в „Моих поисках\""
	ackClosed        = "✓ Поиск закрыт — удачи!"
	ackAnswered      = "уже ответил"
	ackNotYourSearch = "не твой поиск"
	ackNotYourMatch  = "не твой мэтч"
)

type SearchAnswers interface {
	ByID(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
	ClearTTLAsked(ctx context.Context, id uuid.UUID) error
	Close(ctx context.Context, userID, id uuid.UUID, reason entity.CloseReason) (entity.Search, error)
	Reactivate(ctx context.Context, userID, id uuid.UUID) (entity.Search, error)
}

type PollAnswers interface {
	SavePollAnswer(ctx context.Context, matchID, userID uuid.UUID, answer entity.PollAnswer) (bool, error)
}

type Accounts interface {
	ByTelegramID(ctx context.Context, tgID int64) (entity.User, error)
	ClearPushBlocked(ctx context.Context, id uuid.UUID) error
}

type Callbacks struct {
	searches SearchAnswers
	polls    PollAnswers
	users    Accounts
}

func NewCallbacks(searches SearchAnswers, polls PollAnswers, users Accounts) *Callbacks {
	return &Callbacks{searches: searches, polls: polls, users: users}
}

func (c *Callbacks) HandleSearchCallback(ctx context.Context, tgID int64, data string) (string, error) {
	op, searchID, err := parseSearchCallback(data)
	if err != nil {
		return "", err
	}

	user, err := c.users.ByTelegramID(ctx, tgID)
	if errors.Is(err, entity.ErrNotFound) {
		return ackNotYourSearch, nil
	}
	if err != nil {
		return "", err
	}

	switch op {
	case opStillSearching:
		search, err := c.searches.ByID(ctx, user.ID, searchID)
		if errors.Is(err, entity.ErrNotFound) {
			return ackNotYourSearch, nil
		}
		if err != nil {
			return "", err
		}
		switch search.Status {
		case entity.SearchActive:
			if err := c.searches.ClearTTLAsked(ctx, searchID); err != nil {
				return "", err
			}
			return ackSearching, nil
		case entity.SearchPaused:

			_, err := c.searches.Reactivate(ctx, user.ID, searchID)
			switch {
			case errors.Is(err, entity.ErrSearchLimit):
				return ackNoSlots, nil
			case err != nil:
				return "", err
			}
			return ackRevived, nil
		default:
			return ackAnswered, nil
		}

	case opFound:

		_, err := c.searches.Close(ctx, user.ID, searchID, entity.CloseFoundHere)
		switch {
		case errors.Is(err, entity.ErrSearchClosed):
			return ackAnswered, nil
		case errors.Is(err, entity.ErrNotFound):
			return ackNotYourSearch, nil
		case err != nil:
			return "", err
		}
		return ackClosed, nil

	default:
		return "", fmt.Errorf("search callback %q: unknown answer", data)
	}
}

func (c *Callbacks) HandlePollCallback(ctx context.Context, tgID int64, data string) (string, error) {
	matchID, raw, err := parsePollCallback(data)
	if err != nil {
		return "", err
	}
	answer := entity.ParsePollAnswer(raw)
	if answer == entity.PollAnswerUnknown {
		return "", fmt.Errorf("poll callback %q: unknown answer", data)
	}

	user, err := c.users.ByTelegramID(ctx, tgID)
	if errors.Is(err, entity.ErrNotFound) {
		return ackNotYourMatch, nil
	}
	if err != nil {
		return "", err
	}

	saved, err := c.polls.SavePollAnswer(ctx, matchID, user.ID, answer)
	if errors.Is(err, entity.ErrNotFound) {
		return ackNotYourMatch, nil
	}
	if err != nil {
		return "", err
	}
	if !saved {
		return ackAnswered, nil
	}
	return "✓ " + pollLabel(answer), nil
}

func (c *Callbacks) ClearPushBlocked(ctx context.Context, tgID int64) error {
	user, err := c.users.ByTelegramID(ctx, tgID)
	if errors.Is(err, entity.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return c.users.ClearPushBlocked(ctx, user.ID)
}

func stillSearchingButtons(searchID uuid.UUID) []tgbot.PushButton {
	return []tgbot.PushButton{
		{Text: "Да, ищу", CallbackData: searchCallback(opStillSearching, searchID)},
		{Text: "Нашёл", CallbackData: searchCallback(opFound, searchID)},
	}
}

func postMatchButtons(matchID uuid.UUID) []tgbot.PushButton {
	answers := []entity.PollAnswer{entity.PollMoving, entity.PollTalking, entity.PollFailed}
	out := make([]tgbot.PushButton, 0, len(answers))
	for _, answer := range answers {
		out = append(out, tgbot.PushButton{
			Text:         pollLabel(answer),
			CallbackData: pollCallback(matchID, answer),
		})
	}
	return out
}

func searchCallback(op string, searchID uuid.UUID) string {
	return tgbot.SearchCallbackPrefix + ":" + op + ":" + searchID.String()
}

func pollCallback(matchID uuid.UUID, answer entity.PollAnswer) string {
	return tgbot.PollCallbackPrefix + ":" + matchID.String() + ":" + string(answer)
}

func parseSearchCallback(data string) (string, uuid.UUID, error) {
	parts, err := splitCallback(tgbot.SearchCallbackPrefix, data)
	if err != nil {
		return "", uuid.Nil, err
	}
	id, err := uuid.Parse(parts[2])
	if err != nil {
		return "", uuid.Nil, fmt.Errorf("search callback %q: %w", data, err)
	}
	return parts[1], id, nil
}

func parsePollCallback(data string) (uuid.UUID, string, error) {
	parts, err := splitCallback(tgbot.PollCallbackPrefix, data)
	if err != nil {
		return uuid.Nil, "", err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("poll callback %q: %w", data, err)
	}
	return id, parts[2], nil
}

func splitCallback(prefix, data string) ([]string, error) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 || parts[0] != prefix {
		return nil, fmt.Errorf("%s callback %q: unexpected shape", prefix, data)
	}
	return parts, nil
}

func pollLabel(answer entity.PollAnswer) string {
	switch answer {
	case entity.PollMoving:
		return "Съезжаемся"
	case entity.PollTalking:
		return "Общаемся"
	case entity.PollFailed:
		return "Не сложилось"
	default:
		return ""
	}
}
