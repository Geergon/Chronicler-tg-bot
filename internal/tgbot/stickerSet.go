package tgbot

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/Geergon/Chronicler-tg-bot/internal/database"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

const botAPIBase = "https://api.telegram.org/bot"

type botAPIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}

func HandleSaveSticker(ctx *ext.Context, update *ext.Update, db *sql.DB, botUsername, botToken string) error {
	msg := update.EffectiveMessage
	if msg == nil {
		return nil
	}

	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok || replyHeader == nil {
		ctx.SendMessage(update.EffectiveChat().GetID(), &tg.MessagesSendMessageRequest{
			Message: "Використай /qs у відповідь на стікер",
		})
		return nil
	}

	chatID := update.EffectiveChat().GetID()
	chatName := resolveChatName(ctx, chatID)
	userID := update.EffectiveUser().ID
	userName := update.EffectiveUser().Username
	replyMsg, _, _, err := fetchMessage(ctx, chatID, replyHeader.ReplyToMsgID)
	if err != nil || replyMsg == nil {
		return fmt.Errorf("fetchMessage: %w", err)
	}

	location, mimeType, err := fetchStickerFromMessage(ctx, replyMsg)
	if err != nil {
		return err
	}
	stickerBytes, err := downloadFileBytes(ctx, location)
	if err != nil {
		log.Println("dowbload sticker error: ", err)
		_, sendErr := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: fmt.Sprintf("Помилка: %v", err),
		})
		if sendErr != nil {
			log.Printf("send message error: %v", sendErr)
		}
		return err
	}

	_ = mimeType // "image/webp"

	creatorID, found := database.GetCreatorIDFromDB(db, chatID)
	if !found {
		creatorID, err = database.GetCreatorID(ctx, chatID)
		if err != nil {
			log.Printf("getCreatorID failed: %v", err)
			creatorID = userID
		}
	}

	stickerLink, err := SendQuoteSticker(db, botToken, botUsername, chatName, userName, chatID, userID, creatorID, stickerBytes)
	if err != nil {
		return fmt.Errorf("SendQuoteStickerFromBytes: %w", err)
	}

	ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Стікер збережено: " + stickerLink,
	})

	return nil
}

func resolveChatName(ctx *ext.Context, chatID int64) string {
	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	if inputPeer == nil {
		return ""
	}

	switch peer := inputPeer.(type) {
	case *tg.InputPeerChannel:
		result, err := ctx.Raw.ChannelsGetChannels(ctx, []tg.InputChannelClass{
			&tg.InputChannel{
				ChannelID:  peer.ChannelID,
				AccessHash: peer.AccessHash,
			},
		})
		if err != nil {
			return ""
		}
		switch r := result.(type) {
		case *tg.MessagesChats:
			if len(r.Chats) > 0 {
				switch ch := r.Chats[0].(type) {
				case *tg.Channel:
					return ch.Title
				case *tg.Chat:
					return ch.Title
				}
			}
		case *tg.MessagesChatsSlice:
			if len(r.Chats) > 0 {
				switch ch := r.Chats[0].(type) {
				case *tg.Channel:
					return ch.Title
				case *tg.Chat:
					return ch.Title
				}
			}
		}

	case *tg.InputPeerChat:
		result, err := ctx.Raw.MessagesGetChats(ctx, []int64{peer.ChatID})
		if err != nil {
			return ""
		}
		switch r := result.(type) {
		case *tg.MessagesChats:
			if len(r.Chats) > 0 {
				if ch, ok := r.Chats[0].(*tg.Chat); ok {
					return ch.Title
				}
			}
		}
	}

	return ""
}

func botAPIRequest(botToken, method string, fields map[string]string, fileField, fileName string, fileData []byte) (json.RawMessage, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	for k, v := range fields {
		w.WriteField(k, v)
	}

	if fileData != nil {
		fw, err := w.CreateFormFile(fileField, fileName)
		if err != nil {
			return nil, err
		}
		fw.Write(fileData)
	}
	w.Close()

	resp, err := http.Post(
		botAPIBase+botToken+"/"+method,
		w.FormDataContentType(),
		&body,
	)
	if err != nil {
		return nil, fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	var apiResp botAPIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	if !apiResp.OK {
		return nil, fmt.Errorf("bot api error: %s", apiResp.Description)
	}
	return apiResp.Result, nil
}

func SendQuoteSticker(db *sql.DB, botToken, botUsername, chatName, userName string, chatID, userID, creatorID int64, stickerBytes []byte) (string, error) {
	absID := chatID
	if absID < 0 {
		absID = -absID
	}
	if chatName == "" {
		chatName = userName
	}

	packInfo, err := database.GetOrCreatePackInfo(db, chatID, botUsername, chatName)
	if err != nil {
		return "", fmt.Errorf("db error: %w", err)
	}

	setName := packInfo.Name
	stickerLink := "https://t.me/addstickers/" + setName

	_, addErr := botAPIRequest(botToken, "addStickerToSet", map[string]string{
		"user_id": fmt.Sprint(userID),
		"name":    setName,
		"emojis":  "🖼",
	}, "png_sticker", "sticker.webp", stickerBytes)

	if addErr != nil {
		errStr := addErr.Error()

		if strings.Contains(errStr, "PEER_ID_INVALID") ||
			strings.Contains(errStr, "bot was blocked") {
			return "", fmt.Errorf("user %d never started the bot: %w", userID, addErr)
		}

		// if pack doesn't exist or full create new
		if strings.Contains(errStr, "TOO_MUCH") || strings.Contains(errStr, "STICKERSET_INVALID") {
			packInfo.PackIndex++
			packInfo.Name = fmt.Sprintf("quotes_%d_v%d_by_%s", absID, packInfo.PackIndex, botUsername)
			packInfo.Title = fmt.Sprintf("%s | v%d by @%s", chatName, packInfo.PackIndex, botUsername)
			if err := database.SaveOrUpdatePackInfo(db, chatID, packInfo); err != nil {
				return "", fmt.Errorf("savePackInfo: %w", err)
			}
			setName = packInfo.Name
			stickerLink = "https://t.me/addstickers/" + setName

			_, err = botAPIRequest(botToken, "createNewStickerSet", map[string]string{
				"user_id": fmt.Sprint(creatorID),
				"name":    setName,
				"title":   packInfo.Title,
				"emojis":  "🖼",
			}, "png_sticker", "sticker.webp", stickerBytes)
			if err != nil {
				if strings.Contains(err.Error(), "PEER_ID_INVALID") {
					return "", fmt.Errorf("власник чату має написати боту /start: @%s", botUsername)
				}
				return "", fmt.Errorf("createNewStickerSet: %w", err)
			}

			if err := database.SaveCreatorID(db, chatID, creatorID); err != nil {
				log.Printf("saveCreatorID warning: %v", err)
			}

		} else {
			return "", fmt.Errorf("addStickerToSet: %w", addErr)
		}
	}

	type stickerSetResult struct {
		Stickers []struct {
			FileID string `json:"file_id"`
		} `json:"stickers"`
	}

	result, err := botAPIRequest(botToken, "getStickerSet", map[string]string{
		"name": setName,
	}, "", "", nil)
	if err != nil {
		return stickerLink, fmt.Errorf("getStickerSet: %w", err)
	}

	var stickerSet stickerSetResult
	if err := json.Unmarshal(result, &stickerSet); err != nil || len(stickerSet.Stickers) == 0 {
		return stickerLink, fmt.Errorf("parse sticker set: %w", err)
	}

	lastSticker := stickerSet.Stickers[len(stickerSet.Stickers)-1]

	_, err = botAPIRequest(botToken, "sendSticker", map[string]string{
		"chat_id": fmt.Sprint(chatID),
		"sticker": lastSticker.FileID,
	}, "", "", nil)
	if err != nil {
		return stickerLink, fmt.Errorf("sendSticker: %w", err)
	}

	return stickerLink, nil
}
