package tgbot

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

const botAPIBase = "https://api.telegram.org/bot"

type ChatPackInfo struct {
	Name      string
	Title     string
	PackIndex int
}

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
	replyMsg, _, _, err := fetchMessage(ctx, chatID, replyHeader.ReplyToMsgID)
	if err != nil || replyMsg == nil {
		return fmt.Errorf("fetchMessage: %w", err)
	}

	stickerBytes, mimeType, err := fetchStickerFromMessage(ctx, replyMsg)
	if err != nil {
		ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: fmt.Sprintf("Помилка: %v", err),
		})
		return nil
	}

	_ = mimeType // "image/webp"

	stickerLink, err := SendQuoteSticker(db, botToken, botUsername, chatName, chatID, userID, stickerBytes)
	if err != nil {
		return fmt.Errorf("SendQuoteStickerFromBytes: %w", err)
	}

	ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: "Стікер збережено: " + stickerLink,
	})

	return nil
}

func fetchStickerFromMessage(ctx *ext.Context, msg *tg.Message) ([]byte, string, error) {
	if msg.Media == nil {
		return nil, "", fmt.Errorf("no media in message")
	}

	mediaDoc, ok := msg.Media.(*tg.MessageMediaDocument)
	if !ok {
		return nil, "", fmt.Errorf("media is not a document: %T", msg.Media)
	}

	doc, ok := mediaDoc.Document.(*tg.Document)
	if !ok {
		return nil, "", fmt.Errorf("document is not *tg.Document: %T", mediaDoc.Document)
	}

	isSticker := false
	for _, attr := range doc.Attributes {
		if _, ok := attr.(*tg.DocumentAttributeSticker); ok {
			isSticker = true
			break
		}
	}
	if !isSticker {
		return nil, "", fmt.Errorf("document is not a sticker")
	}

	mimeType := doc.MimeType
	if mimeType != "image/webp" {
		return nil, mimeType, fmt.Errorf("unsupported sticker type: %s (only static webp supported)", mimeType)
	}

	location := &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
		ThumbSize:     "", // порожньо = оригінал
	}

	data, err := downloadFileBytes(ctx, location)
	if err != nil {
		return nil, mimeType, fmt.Errorf("download sticker: %w", err)
	}

	return data, mimeType, nil
}

func downloadFileBytes(ctx *ext.Context, location tg.InputFileLocationClass) ([]byte, error) {
	var buf []byte
	offset := 0
	limit := 512 * 1024

	for {
		result, err := ctx.Raw.UploadGetFile(ctx, &tg.UploadGetFileRequest{
			Location: location,
			Offset:   int64(offset),
			Limit:    limit,
		})
		if err != nil {
			return nil, fmt.Errorf("UploadGetFile: %w", err)
		}

		file, ok := result.(*tg.UploadFile)
		if !ok {
			break
		}

		buf = append(buf, file.Bytes...)

		if len(file.Bytes) < limit {
			break
		}
		offset += len(file.Bytes)
	}

	if len(buf) == 0 {
		return nil, fmt.Errorf("empty file")
	}

	return buf, nil
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

func getOrCreatePackInfo(db *sql.DB, chatID int64, botUsername, chatName string) (ChatPackInfo, error) {
	absID := chatID
	if absID < 0 {
		absID = -absID
	}

	var info ChatPackInfo
	query := `SELECT current_pack_name, current_pack_title, pack_index FROM chat_stickers WHERE chat_id = ?`
	err := db.QueryRow(query, chatID).Scan(&info.Name, &info.Title, &info.PackIndex)

	if err == sql.ErrNoRows {
		info.PackIndex = 1
		info.Name = fmt.Sprintf("quotes_%d_v%d_by_%s", absID, info.PackIndex, botUsername)
		info.Title = fmt.Sprintf("%s | v%d pack by @%s", chatName, info.PackIndex, botUsername)

		if err := saveOrUpdatePackInfo(db, chatID, info); err != nil {
			return info, fmt.Errorf("saveOrUpdatePackInfo on create: %w", err)
		}
		return info, nil
	}

	if err != nil {
		return info, fmt.Errorf("query pack info: %w", err)
	}

	return info, nil
}

func saveOrUpdatePackInfo(db *sql.DB, chatID int64, info ChatPackInfo) error {
	query := `
		INSERT INTO chat_stickers (chat_id, current_pack_name, current_pack_title, pack_index)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			current_pack_name  = excluded.current_pack_name,
			current_pack_title = excluded.current_pack_title,
			pack_index         = excluded.pack_index
	`
	_, err := db.Exec(query, chatID, info.Name, info.Title, info.PackIndex)
	if err != nil {
		return fmt.Errorf("saveOrUpdatePackInfo: %w", err)
	}
	return nil
}

func SendQuoteSticker(db *sql.DB, botToken, botUsername, chatName string, chatID, userID int64, stickerBytes []byte) (string, error) {
	absID := chatID
	if absID < 0 {
		absID = -absID
	}

	packInfo, err := getOrCreatePackInfo(db, chatID, botUsername, chatName)
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
			if strings.Contains(errStr, "TOO_MUCH") {
				packInfo.PackIndex++
			}
			packInfo.Name = fmt.Sprintf("quotes_%d_v%d_by_%s", absID, packInfo.PackIndex, botUsername)
			packInfo.Title = fmt.Sprintf("%s | v%d by @%s", chatName, packInfo.PackIndex, botUsername)
			if err := saveOrUpdatePackInfo(db, chatID, packInfo); err != nil {
				return "", fmt.Errorf("savePackInfo: %w", err)
			}
			setName = packInfo.Name
			stickerLink = "https://t.me/addstickers/" + setName

			_, err = botAPIRequest(botToken, "createNewStickerSet", map[string]string{
				"user_id": fmt.Sprint(userID),
				"name":    setName,
				"title":   packInfo.Title,
				"emojis":  "🖼",
			}, "png_sticker", "sticker.webp", stickerBytes)
			if err != nil {
				return "", fmt.Errorf("createNewStickerSet: %w", err)
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
