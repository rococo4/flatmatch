package tgbot

import "context"

// ModerationSender adapts the Bot API client to what the moderation service
// asks of a transport: a photo file, a caption and buttons as (text,
// callback_data) pairs. The service must not know about keyboards, and the
// client must not know about verdicts.
type ModerationSender struct{ client *Client }

func NewModerationSender(client *Client) *ModerationSender {
	return &ModerationSender{client: client}
}

func (s *ModerationSender) SendModerationPhoto(
	ctx context.Context, chatID int64, photoPath, caption string, buttons [][2]string,
) (int, error) {
	return s.client.SendPhotoFile(ctx, chatID, photoPath, caption, keyboard(buttons))
}

// keyboard puts every verdict in one row: the queue has two or three short
// labels, and one row keeps the tap targets where the eye already is.
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
