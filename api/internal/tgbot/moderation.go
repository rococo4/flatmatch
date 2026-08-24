package tgbot

import (
	"context"
	"errors"
)

type ModerationSender struct{ client *Client }

var errNoModerationPhoto = errors.New("a moderation card needs at least one photo")

func NewModerationSender(client *Client) *ModerationSender {
	return &ModerationSender{client: client}
}

func (s *ModerationSender) SendModerationPhotos(
	ctx context.Context, chatID int64, photoPaths []string, caption string, buttons [][2]string,
) (int, error) {
	if len(photoPaths) == 0 {
		return 0, errNoModerationPhoto
	}

	messageID, err := s.client.SendPhotoFile(ctx, chatID, photoPaths[0], caption, keyboard(buttons))
	if err != nil {
		return 0, err
	}

	switch rest := photoPaths[1:]; {
	case len(rest) == 1:
		_, err = s.client.SendPhotoFile(ctx, chatID, rest[0], "", nil)
	case len(rest) > 1:
		err = s.client.SendMediaGroupFiles(ctx, chatID, rest)
	}
	return messageID, err
}

func keyboard(buttons [][2]string) *InlineKeyboard {
	if len(buttons) == 0 {
		return nil
	}
	row := make([]InlineKeyboardButton, 0, len(buttons))
	for _, b := range buttons {
		row = append(row, InlineKeyboardButton{Text: b[0], Data: b[1]})
	}
	return &InlineKeyboard{Rows: [][]InlineKeyboardButton{row}}
}
