package tgbot

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/Geergon/Chronicler-tg-bot/internal/database"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func HandleRandomQuotes(ctx *ext.Context, update *ext.Update, db *sql.DB, botToken string) error {
	msg := update.EffectiveMessage
	if msg == nil {
		return fmt.Errorf("msg is empty")
	}

	chatID := update.EffectiveChat().GetID()
	fileID, err := database.GetRandomQuote(db, chatID)
	if err == sql.ErrNoRows {
		_, _ = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Цитат в базі даних ще немає. Зберігай їх через /qs, щоб бот міг їх надсилати",
		})
		return err
	}
	if err != nil {
		return fmt.Errorf("failed to get random sticker from database: %v", err)
	}

	err = SendStickerByFileID(botToken, chatID, fileID)
	if err != nil {
		log.Println("SendStickerByFileID error: ", err)
		return err
	}
	return nil
}

func SendStickerByFileID(botToken string, chatID int64, fileID string) error {
	_, err := botAPIRequest(botToken, "sendSticker", map[string]string{
		"chat_id": fmt.Sprint(chatID),
		"sticker": fileID,
	}, "", "", nil)
	return err
}
