package tgbot

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

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

func SaveStickerSetInDB(ctx *ext.Context, update *ext.Update, quoteDB *sql.DB, botToken string) error {
	chatID := update.EffectiveChat().GetID()
	var stickerSetName string

	args := strings.Fields(update.EffectiveMessage.Text)
	log.Printf("SaveStickerSetInDB: chatID=%d args=%v len=%d", chatID, args, len(args))

	if len(args) != 2 {
		log.Printf("SaveStickerSetInDB: expected 2 args, got %d", len(args))
		_, _ = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: "Використання: /qrsave https://t.me/addstickers/назва_паку",
		})
		return nil
	}
	stickerSetName = extractSetName(args[1])
	log.Printf("SaveStickerSetInDB: setName=%s", stickerSetName)

	fileIDs, err := GetStickerSetFileIDs(botToken, stickerSetName)
	if err != nil {
		log.Printf("GetStickerSetFileIDs error: %v", err)
		_, _ = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: fmt.Sprintf("Помилка отримання паку: %v", err),
		})
		return err
	}
	log.Printf("SaveStickerSetInDB: got %d stickers", len(fileIDs))

	saved := 0
	for _, fileID := range fileIDs {
		if err := database.SaveQuote(quoteDB, chatID, 0, fileID); err != nil {
			log.Printf("save quote %s: %v", fileID, err)
			continue
		}
		saved++
	}

	log.Printf("SaveStickerSetInDB: saved %d/%d stickers", saved, len(fileIDs))
	_, _ = ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: fmt.Sprintf("Збережено %d стікерів з паку %s", saved, stickerSetName),
	})
	return nil
}

func SendStickerByFileID(botToken string, chatID int64, fileID string) error {
	_, err := botAPIRequest(botToken, "sendSticker", map[string]string{
		"chat_id": fmt.Sprint(chatID),
		"sticker": fileID,
	}, "", "", nil)
	return err
}

func GetStickerSetFileIDs(botToken, setName string) ([]string, error) {
	result, err := botAPIRequest(botToken, "getStickerSet", map[string]string{
		"name": setName,
	}, "", "", nil)
	if err != nil {
		return nil, fmt.Errorf("getStickerSet: %w", err)
	}

	type stickerSetResult struct {
		Name     string `json:"name"`
		Title    string `json:"title"`
		Stickers []struct {
			FileID     string `json:"file_id"`
			Emoji      string `json:"emoji"`
			IsAnimated bool   `json:"is_animated"`
			IsVideo    bool   `json:"is_video"`
		} `json:"stickers"`
	}

	var stickerSet stickerSetResult
	if err := json.Unmarshal(result, &stickerSet); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	var fileIDs []string
	for _, s := range stickerSet.Stickers {
		fileIDs = append(fileIDs, s.FileID)
	}

	log.Printf("getStickerSet %s: %d stickers", setName, len(fileIDs))
	return fileIDs, nil
}

func extractSetName(input string) string {
	input = strings.TrimSpace(input)
	if strings.Contains(input, "t.me/addstickers/") {
		parts := strings.Split(input, "t.me/addstickers/")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return input
}
