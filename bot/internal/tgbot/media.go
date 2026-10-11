package tgbot

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/Geergon/Chronicler-tg-bot/internal/renderclient"
	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

var ffmpegSem = make(chan struct{}, 2)

const maxVideoForFFmpeg = 30 << 20 // 30 mb

func fetchMediaGroup(ctx *ext.Context, chatID int64, msg *tg.Message) ([]renderclient.MediaItem, error) {
	if msg.GroupedID == 0 {
		item, err := fetchMedia(ctx, msg)
		if err != nil || len(item.Data) == 0 {
			return nil, err
		}
		return []renderclient.MediaItem{item}, nil
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

	var images []renderclient.MediaItem
	for _, gm := range groupMsgs {
		item, err := fetchMedia(ctx, gm)
		if err != nil {
			log.Printf("fetchMediaGroup: skip msg %d: %v", gm.ID, err)
			continue
		}
		if len(item.Data) == 0 {
			continue
		}
		images = append(images, item)
	}
	return images, nil
}

func fetchMedia(ctx *ext.Context, msg *tg.Message) (renderclient.MediaItem, error) {
	switch m := msg.Media.(type) {
	case *tg.MessageMediaPhoto:
		return fetchPhoto(ctx, m)
	case *tg.MessageMediaDocument:
		return fetchVideoFrame(ctx, m)
	}
	return renderclient.MediaItem{}, nil
}

func fetchPhoto(ctx *ext.Context, mediaPhoto *tg.MessageMediaPhoto) (renderclient.MediaItem, error) {
	photo, ok := mediaPhoto.Photo.(*tg.Photo)
	if !ok {
		log.Println("failed to get photo object")
		return renderclient.MediaItem{}, nil
	}

	bestSize := pickBestPhotoSize(photo.Sizes)
	if bestSize == nil {
		log.Printf("fetchPhoto: no suitable size found")
		return renderclient.MediaItem{}, nil
	}

	data, err := downloadFile(ctx, &tg.InputPhotoFileLocation{
		ID:            photo.ID,
		AccessHash:    photo.AccessHash,
		FileReference: photo.FileReference,
		ThumbSize:     bestSize.Type,
	})
	if err != nil {
		return renderclient.MediaItem{}, err
	}
	return renderclient.MediaItem{Data: data, Spoiler: mediaPhoto.Spoiler}, nil
}

func isVideoLike(doc *tg.Document) bool {
	for _, a := range doc.Attributes {
		switch a.(type) {
		case *tg.DocumentAttributeSticker, *tg.DocumentAttributeCustomEmoji:
			return false // стікери й емодзі обробляються окремо
		}
	}
	return strings.HasPrefix(doc.MimeType, "video/") || doc.MimeType == "image/gif"
}

func fetchVideoFrame(ctx *ext.Context, m *tg.MessageMediaDocument) (renderclient.MediaItem, error) {
	doc, ok := m.Document.(*tg.Document)
	if !ok || !isVideoLike(doc) {
		return renderclient.MediaItem{}, nil
	}

	size := bestThumbSize(doc)
	if size == "" {
		log.Printf("video %d has no usable thumbnail", doc.ID)
		if doc.Size > maxVideoForFFmpeg {
			log.Printf("video %d is too large for ffmpeg (%d bytes), skipped", doc.ID, doc.Size)
			return renderclient.MediaItem{}, nil
		}
		loc := func(thumb string) *tg.InputDocumentFileLocation {
			return &tg.InputDocumentFileLocation{
				ID:            doc.ID,
				AccessHash:    doc.AccessHash,
				FileReference: doc.FileReference,
				ThumbSize:     thumb,
			}
		}
		video, err := downloadFile(ctx, loc(""))
		if err != nil {
			return renderclient.MediaItem{}, err
		}
		frame, err := firstFrame(ctx, video)
		if err != nil {
			log.Printf("ffmpeg first frame %d: %v", doc.ID, err)
			return renderclient.MediaItem{}, nil
		}
		return renderclient.MediaItem{Data: frame, Spoiler: m.Spoiler, Video: true}, nil
	}

	data, err := downloadFile(ctx, &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
		ThumbSize:     size,
	})
	if err != nil {
		return renderclient.MediaItem{}, err
	}
	return renderclient.MediaItem{Data: data, Spoiler: m.Spoiler, Video: true, Round: isRoundVideo(doc)}, nil
}

func pickBestPhotoSize(sizes []tg.PhotoSizeClass) *tg.PhotoSize {
	preferred := []string{"x", "m", "s"}
	// preferred := []string{"x"}
	sizeMap := make(map[string]*tg.PhotoSize)
	for _, s := range sizes {
		switch ps := s.(type) {
		case *tg.PhotoSize:
			sizeMap[ps.Type] = ps
		case *tg.PhotoSizeProgressive:
			if len(ps.Sizes) == 0 {
				return nil
			}
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

	thumb := ""
	mimeType := doc.MimeType
	if mimeType != "image/webp" {
		thumb = bestThumbSize(doc)
		if thumb == "" {
			return nil, doc.MimeType, fmt.Errorf("animated sticker %s has no usable thumbnail", doc.MimeType)
		}
	}

	location := &tg.InputDocumentFileLocation{
		ID:            doc.ID,
		AccessHash:    doc.AccessHash,
		FileReference: doc.FileReference,
		ThumbSize:     thumb,
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

func downloadFile(ctx *ext.Context, location tg.InputFileLocationClass) ([]byte, error) {
	return downloadFileBytes(ctx, location)
}

func firstFrame(ctx context.Context, video []byte) ([]byte, error) {
	ffmpegSem <- struct{}{}
	defer func() { <-ffmpegSem }()

	f, err := os.CreateTemp("", "vid-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(video); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()

	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cctx, "ffmpeg",
		"-v", "error",
		"-i", f.Name(),
		"-frames:v", "1",
		"-vf", "scale='min(1280,iw)':-2",
		"-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "3",
		"pipe:1",
	)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	if out.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg returned no data")
	}
	return out.Bytes(), nil
}

func isRoundVideo(doc *tg.Document) bool {
	for _, a := range doc.Attributes {
		if v, ok := a.(*tg.DocumentAttributeVideo); ok && v.RoundMessage {
			return true
		}
	}
	return false
}
