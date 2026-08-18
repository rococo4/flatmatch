package tgbot

import "context"

type ModerationSender struct{ client *Client }

func NewModerationSender(client *Client) *ModerationSender {
	return &ModerationSender{client: client}
}

func (s *ModerationSender) SendModerationPhoto(
	ctx context.Context, chatID int64, photoPath, caption string, buttons [][2]string,
) (int, error) {
	return s.client.SendPhotoFile(ctx, chatID, photoPath, caption, keyboard(buttons))
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
