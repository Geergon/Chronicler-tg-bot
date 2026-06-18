package tgbot

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math/big"
	"mime/multipart"
	"net/http"

	"github.com/HugoSmits86/nativewebp"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

const botAPIBase = "https://api.telegram.org/bot"

type botAPIResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
}

func HandleStickerSetCommand(ctx *ext.Context, update *ext.Update, botUsername, botToken string) error {
	msg := update.EffectiveMessage
	if msg == nil {
		return nil
	}
	// chatID := update.EffectiveChat().GetID()

	// replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	// if !ok || replyHeader == nil {
	// 	log.Println("reply header is empty")
	// 	return nil
	// }

	// creatorID, err := getCreatorID(ctx, chatID)
	// if err != nil {
	// 	log.Printf("failder to get channle/supergroup creator ID: %v", err)
	// 	return err
	// }
	//
	// stickerLink, err := SendQuoteSticker(botToken, botUsername, chatID, creatorID, sticker.Image())
	// if err != nil {
	// 	log.Printf("SendQuoteSticker error: %v", err)
	// 	return nil
	// }
	//
	// ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
	// 	Message: fmt.Sprintf("Стікер збережено в пак: %s", stickerLink),
	// })

	return nil
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

func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "pack"
		}
		b[i] = charset[num.Int64()]
	}
	return string(b)
}

func stickerSetShortName(chatID int64, botUsername string) string {
	absID := chatID
	if absID < 0 {
		absID = -absID
	}
	salt := generateRandomString(6)
	return fmt.Sprintf("g%s_%d_by_@%s", salt, absID, botUsername)
}

func SendQuoteSticker(botToken, botUsername string, chatID, userID int64, img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := nativewebp.Encode(&buf, img, nil); err != nil {
		return "", fmt.Errorf("webp encode: %w", err)
	}
	stickerBytes := buf.Bytes()

	absID := chatID
	if absID < 0 {
		absID = -absID
	}
	number := 1

	// setName := stickerSetShortName(chatID, botUsername)
	setName := fmt.Sprintf("quotes_%d_%d_by_%s", absID, number, botUsername)
	stickerLink := "https://t.me/addstickers/" + setName

	_, err := botAPIRequest(botToken, "addStickerToSet", map[string]string{
		"user_id": fmt.Sprint(userID),
		"name":    setName,
		"emojis":  "🖼",
	}, "png_sticker", "sticker.webp", stickerBytes)
	if err != nil {
		// if pack doesn't exist or full create new
		if isPackFull(err) {
			// generate new name
			setName = fmt.Sprintf("quotes_%d_%d_by_%s", absID, number, botUsername)
			stickerLink = "https://t.me/addstickers/" + setName
		}

		packTitle := fmt.Sprintf("Chat %d pack by @%s", chatID, botUsername)
		fmt.Println(packTitle)
		fmt.Println(setName)
		_, err = botAPIRequest(botToken, "createNewStickerSet", map[string]string{
			"user_id": fmt.Sprint(userID),
			"name":    setName,
			"title":   packTitle,
			"emojis":  "🖼",
		}, "png_sticker", "sticker.webp", stickerBytes)
		if err != nil {
			return "", fmt.Errorf("createNewStickerSet: %w", err)
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

func isPackFull(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "STICKERSET_INVALID") ||
		contains(errStr, "TOO_MUCH") ||
		contains(errStr, "invalid sticker set")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func getCreatorID(ctx *ext.Context, chatID int64) (int64, error) {
	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	inputChannel, ok := inputPeer.(*tg.InputPeerChannel)
	if !ok {
		return 0, fmt.Errorf("getCreatorID: it's not a channel or supergroup")
	}

	res, err := ctx.Raw.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{
			ChannelID:  inputChannel.ChannelID,
			AccessHash: inputChannel.AccessHash,
		},
		Filter: &tg.ChannelParticipantsAdmins{},
		Offset: 0,
		Limit:  100,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get participants: %w", err)
	}

	switch data := res.(type) {
	case *tg.ChannelsChannelParticipants:
		for _, participant := range data.Participants {
			switch p := participant.(type) {
			case *tg.ChannelParticipantCreator:
				return p.UserID, nil
			}
		}
	}

	return 0, fmt.Errorf("creator not found in admins list")
}
