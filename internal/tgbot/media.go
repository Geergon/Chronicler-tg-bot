package tgbot

import (
	"bytes"
	"fmt"
	"image"
	"log"
	"sort"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func fetchMediaGroup(ctx *ext.Context, chatID int64, msg *tg.Message) ([]image.Image, error) {
	if msg.GroupedID == 0 {
		return fetchMedia(ctx, msg)
	}

	const searchRange = 9
	ids := make([]tg.InputMessageClass, 0, searchRange*2+1)
	for i := msg.ID - searchRange; i <= msg.ID+searchRange; i++ {
		if i > 0 {
			ids = append(ids, &tg.InputMessageID{ID: i})
		}
	}

	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	var msgClass tg.MessagesMessagesClass
	var err error

	switch peer := inputPeer.(type) {
	case *tg.InputPeerChannel:
		msgClass, err = ctx.Raw.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{
				ChannelID:  peer.ChannelID,
				AccessHash: peer.AccessHash,
			},
			ID: ids,
		})
	default:
		msgClass, err = ctx.Raw.MessagesGetMessages(ctx, ids)
	}

	if err != nil {
		return nil, fmt.Errorf("fetchMediaGroup: %w", err)
	}

	var rawMessages []tg.MessageClass
	switch r := msgClass.(type) {
	case *tg.MessagesChannelMessages:
		rawMessages = r.Messages
	case *tg.MessagesMessages:
		rawMessages = r.Messages
	case *tg.MessagesMessagesSlice:
		rawMessages = r.Messages
	}

	var groupMsgs []*tg.Message
	for _, m := range rawMessages {
		gm, ok := m.(*tg.Message)
		if !ok {
			continue
		}
		if gm.GroupedID == msg.GroupedID {
			groupMsgs = append(groupMsgs, gm)
		}
	}

	sort.Slice(groupMsgs, func(i, j int) bool {
		return groupMsgs[i].ID < groupMsgs[j].ID
	})

	var images []image.Image
	for _, gm := range groupMsgs {
		imgs, err := fetchMedia(ctx, gm)
		if err != nil {
			log.Printf("fetchMediaGroup: skip msg %d: %v", gm.ID, err)
			continue
		}
		images = append(images, imgs...)
	}

	return images, nil
}

func fetchMedia(ctx *ext.Context, msg *tg.Message) ([]image.Image, error) {
	if msg.Media == nil {
		// log.Println("message doesn't contain media")
		return nil, nil
	}

	mediaPhoto, ok := msg.Media.(*tg.MessageMediaPhoto)
	if !ok {
		// log.Println("media not photo")
		return nil, nil
	}

	photo, ok := mediaPhoto.Photo.(*tg.Photo)
	if !ok {
		log.Println("failed to get photo object")
		return nil, nil
	}

	// log.Printf("fetchMedia: photo ID=%d sizes=%d", photo.ID, len(photo.Sizes))
	// for _, s := range photo.Sizes {
	// 	log.Printf(" media size type=%T %+v", s, s)
	// }

	bestSize := pickBestPhotoSize(photo.Sizes)
	// log.Printf("fetchMedia: bestSize=%s", bestSize)
	if bestSize == nil {
		log.Printf("fetchMedia: no suitable size found")
		return nil, nil
	}
	// log.Printf("fetchMedia: bestSize type=%s", bestSize.Type)

	location := &tg.InputPhotoFileLocation{
		ID:            photo.ID,
		AccessHash:    photo.AccessHash,
		FileReference: photo.FileReference,
		ThumbSize:     bestSize.Type,
	}

	var images []image.Image
	img, err := downloadFile(ctx, location)
	if err != nil {
		return nil, err
	}
	if img != nil {
		images = append(images, img)
	}
	return images, err
}

func pickBestPhotoSize(sizes []tg.PhotoSizeClass) *tg.PhotoSize {
	// preferred := []string{"m", "x"}
	preferred := []string{"x"}
	sizeMap := make(map[string]*tg.PhotoSize)
	for _, s := range sizes {
		switch ps := s.(type) {
		case *tg.PhotoSize:
			sizeMap[ps.Type] = ps
		case *tg.PhotoSizeProgressive:
			sizeMap[ps.Type] = &tg.PhotoSize{
				Type: ps.Type,
				W:    ps.W,
				H:    ps.H,
				Size: ps.Sizes[len(ps.Sizes)-1],
			}
		}
	}
	for _, t := range preferred {
		if s, ok := sizeMap[t]; ok {
			return s
		}
	}
	return nil
}

func fetchStickerFromMessage(ctx *ext.Context, msg *tg.Message) (*tg.InputDocumentFileLocation, string, error) {
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

	return location, mimeType, nil
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

func downloadFile(ctx *ext.Context, location tg.InputFileLocationClass) (image.Image, error) {
	var buf []byte
	offset := 0
	limit := 512 * 1024 // 512KB

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
		return nil, nil
	}

	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("image.Decode: %w", err)
	}

	return img, nil
}
