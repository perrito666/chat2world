package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/perrito666/chat2world/im"
)

const apiTelegramFileURL = "https://api.telegram.org/file"

func getFileContents(ctx context.Context, b *bot.Bot, fileID string) ([]byte, error) {
	fLink, err := b.GetFile(ctx, &bot.GetFileParams{
		FileID: fileID,
	})
	if err != nil {
		return nil, fmt.Errorf("telegram get photo file: %w", err)
	}
	if fLink.FilePath == "" {
		return nil, fmt.Errorf("telegram get photo file path is empty")
	}
	tgURI, _ := url.Parse(apiTelegramFileURL)
	tgURI = tgURI.JoinPath("bot"+b.Token(), fLink.FilePath)

	// https://api.telegram.org/file/bot<token>/<file_path>
	res, err := http.Get(tgURI.String())
	if err != nil {
		return nil, fmt.Errorf("telegram GET photo file: %w", err)
	}
	defer res.Body.Close()
	return io.ReadAll(res.Body)
}

func messageFromTelegramMessage(ctx context.Context, b *bot.Bot, u *models.Update) (*im.Message, error) {
	msg := im.Message{
		ChatID: u.Message.Chat.ID,
		UserID: uint64(u.Message.From.ID),
		MsgID:  uint64(u.Message.ID),
	}
	msg.Text = u.Message.Text
	// Append photo content (if any).
	if len(u.Message.Photo) > 0 {
		// Use the largest photo available (the last element).
		photo := u.Message.Photo[len(u.Message.Photo)-1]
		rawPhotoBytes, err := getFileContents(ctx, b, photo.FileID)
		if err != nil {
			return nil, fmt.Errorf("telegram getting file: %w", err)
		}
		msg.Images = make([]*im.Image, 1)
		msg.Images[0] = &im.Image{
			Data:    rawPhotoBytes,
			Caption: u.Message.Caption,
		}
	}

	return &msg, nil
}
