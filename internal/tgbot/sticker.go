package tgbot

import (
	"bytes"
	"fmt"
	"image"
	"log"
	"time"

	"github.com/HugoSmits86/nativewebp"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

// func stickerSetName(userID int64, botUsername string) string {
// 	// botUsername = strings.TrimPrefix(botUsername, "@")
// 	return fmt.Sprintf("quotes_%d_by_%s", userID, botUsername)
// }

func imageToWebP(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	err := nativewebp.Encode(&buf, img, nil)
	if err != nil {
		log.Fatalf("Error encoding image to WebP: %v", err)
	}
	return buf.Bytes(), nil
}

func uploadMedia(ctx *ext.Context, chatID int64, fileBytes []byte, mimeType string) (tg.DocumentClass, error) {
	fileID := time.Now().UnixNano()
	const partSize = 512 * 1024 // 512KB

	for i := 0; i*partSize < len(fileBytes); i++ {
		start := i * partSize
		end := start + partSize
		if end > len(fileBytes) {
			end = len(fileBytes)
		}

		saved, err := ctx.Raw.UploadSaveFilePart(ctx, &tg.UploadSaveFilePartRequest{
			FileID:   fileID,
			FilePart: i,
			Bytes:    fileBytes[start:end],
		})
		if err != nil || !saved {
			return nil, fmt.Errorf("UploadSaveFilePart part %d: %w", i, err)
		}
	}

	parts := (len(fileBytes) + partSize - 1) / partSize
	inputFile := &tg.InputFile{
		ID:    fileID,
		Parts: parts,
		Name:  "sticker.webp",
	}

	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)

	result, err := ctx.Raw.MessagesUploadMedia(ctx, &tg.MessagesUploadMediaRequest{
		Peer: inputPeer,
		Media: &tg.InputMediaUploadedDocument{
			File:     inputFile,
			MimeType: mimeType,
			Attributes: []tg.DocumentAttributeClass{
				&tg.DocumentAttributeSticker{
					Alt:        "🖼",
					Stickerset: &tg.InputStickerSetEmpty{},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("MessagesUploadMedia: %w", err)
	}

	mediaDoc, ok := result.(*tg.MessageMediaDocument)
	if !ok {
		return nil, fmt.Errorf("unexpected media type: %T", result)
	}

	return mediaDoc.Document, nil
}

func sendStickerToChat(ctx *ext.Context, chatID int64, doc *tg.Document) error {
	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	_, err := ctx.Raw.MessagesSendMedia(ctx, &tg.MessagesSendMediaRequest{
		Peer: inputPeer,
		Media: &tg.InputMediaDocument{
			ID: &tg.InputDocument{
				ID:            doc.ID,
				AccessHash:    doc.AccessHash,
				FileReference: doc.FileReference,
			},
		},
		RandomID: time.Now().UnixNano(),
	})
	return err
}
