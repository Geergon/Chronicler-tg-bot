package tgbot

import (
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
		log.Println("message doesn't contain media")
		return nil, nil
	}

	mediaPhoto, ok := msg.Media.(*tg.MessageMediaPhoto)
	if !ok {
		log.Println("media not photo")
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
	images = append(images, img)
	return images, err
}

func pickBestPhotoSize(sizes []tg.PhotoSizeClass) *tg.PhotoSize {
	preferred := []string{"m", "x"}
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
