package tgbot

import (
	"fmt"
	"image"
	"log"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func fetchAvatar(ctx *ext.Context, id int64) (image.Image, error) {
	inputPeer := ctx.PeerStorage.GetInputPeerById(id)
	if inputPeer == nil {
		return nil, fmt.Errorf("cannot resolve peer %d", id)
	}

	switch inputPeer.(type) {
	case *tg.InputPeerUser, *tg.InputPeerSelf:
		return fetchUserAvatar(ctx, id)
	case *tg.InputPeerChannel, *tg.InputPeerChat:
		return fetchChatAvatar(ctx, id)
	default:
		return nil, fmt.Errorf("unsupported peer type: %T", inputPeer)
	}
}

func fetchChatAvatar(ctx *ext.Context, chatID int64) (image.Image, error) {
	inputPeer := ctx.PeerStorage.GetInputPeerById(chatID)
	if inputPeer == nil {
		return nil, fmt.Errorf("cannot resolve chat %d", chatID)
	}

	var photo *tg.Photo

	switch peer := inputPeer.(type) {
	case *tg.InputPeerChannel:
		full, err := ctx.Raw.ChannelsGetFullChannel(ctx, &tg.InputChannel{
			ChannelID:  peer.ChannelID,
			AccessHash: peer.AccessHash,
		})
		if err != nil {
			return nil, fmt.Errorf("ChannelsGetFullChannel: %w", err)
		}

		fullChat, ok := full.FullChat.(*tg.ChannelFull)
		if !ok {
			log.Println("failed to get full info about channel")
			return nil, nil
		}

		photo, ok = fullChat.ChatPhoto.(*tg.Photo)
		if !ok {
			log.Println("failed to get channel photo")
			return nil, nil
		}

	case *tg.InputPeerChat:
		full, err := ctx.Raw.MessagesGetFullChat(ctx, peer.ChatID)
		if err != nil {
			return nil, fmt.Errorf("MessagesGetFullChat: %w", err)
		}

		fullChat, ok := full.FullChat.(*tg.ChatFull)
		if !ok {
			log.Println("failed to get full info about chat")
			return nil, nil
		}

		photo, ok = fullChat.ChatPhoto.(*tg.Photo)
		if !ok {
			log.Println("failed to get chat photo")
			return nil, nil
		}

	default:
		return nil, fmt.Errorf("unsupported peer type: %T", inputPeer)
	}

	if photo == nil {
		log.Println("channel/chat avatar doesn't exist")
		return nil, nil
	}

	bestSize := pickBestAvatarSize(photo.Sizes)
	if bestSize == nil {
		log.Println("failed to get bestsize for channel avatar")
		return nil, nil
	}

	location := &tg.InputPhotoFileLocation{
		ID:            photo.ID,
		AccessHash:    photo.AccessHash,
		FileReference: photo.FileReference,
		ThumbSize:     bestSize.Type,
	}

	return downloadFile(ctx, location)
}

func fetchUserAvatar(ctx *ext.Context, userID int64) (image.Image, error) {
	inputPeer := ctx.PeerStorage.GetInputPeerById(userID)
	log.Printf("fetchUserAvatar: userID=%d inputPeer=%T %+v", userID, inputPeer, inputPeer)
	if inputPeer == nil {
		log.Println("input peer is empty")
		return nil, fmt.Errorf("input peer is empty")
	}
	inputUser, ok := toInputUser(inputPeer)
	if !ok {
		return nil, fmt.Errorf("cannot resolve user %d", userID)
	}

	photos, err := ctx.Raw.PhotosGetUserPhotos(ctx, &tg.PhotosGetUserPhotosRequest{
		UserID: inputUser,
		Offset: 0,
		Limit:  1,
	})
	if err != nil {
		return nil, fmt.Errorf("PhotosGetUserPhotos: %w", err)
	}

	var photoList []tg.PhotoClass
	switch p := photos.(type) {
	case *tg.PhotosPhotos:
		photoList = p.Photos
	case *tg.PhotosPhotosSlice:
		photoList = p.Photos
	}

	if len(photoList) == 0 {
		log.Printf("fetchUserAvatar: no photos for user %d", userID)
		return nil, nil
	}

	photo, ok := photoList[0].(*tg.Photo)
	if !ok {
		log.Printf("fetchUserAvatar: photo[0] is %T, not *tg.Photo", photoList[0])
		return nil, nil
	}

	log.Printf("fetchUserAvatar: photo ID=%d sizes=%d", photo.ID, len(photo.Sizes))

	for _, s := range photo.Sizes {
		log.Printf("  size type=%T %+v", s, s)
	}

	bestSize := pickBestAvatarSize(photo.Sizes)
	if bestSize == nil {
		log.Printf("fetchUserAvatar: no suitable size found, available types: %v",
			func() []string {
				var types []string
				for _, s := range photo.Sizes {
					switch ps := s.(type) {
					case *tg.PhotoSize:
						types = append(types, ps.Type)
					case *tg.PhotoSizeProgressive:
						types = append(types, ps.Type+"(progressive)")
					}
				}
				return types
			}())
		return nil, nil
	}
	// if bestSize == nil {
	// 	log.Printf("fetchUserAvatar: no suitable size found")
	// 	return nil, nil
	// }
	log.Printf("fetchUserAvatar: bestSize type=%s", bestSize.Type)

	location := &tg.InputPhotoFileLocation{
		ID:            photo.ID,
		AccessHash:    photo.AccessHash,
		FileReference: photo.FileReference,
		ThumbSize:     bestSize.Type,
	}

	img, err := downloadFile(ctx, location)
	if err != nil {
		log.Printf("downloadFile failed user=%d photoID=%d err=%v", userID, photo.ID, err)
	}
	return img, err
}

func toInputUser(peer tg.InputPeerClass) (tg.InputUserClass, bool) {
	switch p := peer.(type) {
	case *tg.InputPeerUser:
		return &tg.InputUser{
			UserID:     p.UserID,
			AccessHash: p.AccessHash,
		}, true
	case *tg.InputPeerSelf:
		return &tg.InputUserSelf{}, true
	}
	return nil, false
}

func pickBestAvatarSize(sizes []tg.PhotoSizeClass) *tg.PhotoSize {
	preferred := []string{"a", "b", "c"}
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

func GetAvatarLocationFromPeer(userMap map[int64]*tg.User, chatMap map[int64]tg.ChatClass, ctx *ext.Context, peerID int64) (tg.InputFileLocationClass, error) {
	if u, ok := userMap[peerID]; ok && u.Photo != nil {
		if userPhoto, ok := u.Photo.(*tg.UserProfilePhoto); ok {
			inputPeer := ctx.PeerStorage.GetInputPeerById(peerID)
			if !ok {
				return nil, fmt.Errorf("failed to convert peer to input user")
			}
			if inputPeer == nil {
				return nil, fmt.Errorf("peer not found")
			}

			return &tg.InputPeerPhotoFileLocation{
				Peer:    inputPeer,
				PhotoID: userPhoto.PhotoID,
				Big:     false,
			}, nil
		}
	}

	if chat, ok := chatMap[peerID]; ok {
		switch c := chat.(type) {
		case *tg.Chat:
			if photo, ok := c.Photo.(*tg.ChatPhoto); ok {
				peer := ctx.PeerStorage.GetInputPeerById(peerID)
				if peer == nil {
					return nil, fmt.Errorf("peer not found")
				}

				return &tg.InputPeerPhotoFileLocation{
					Peer:    peer,
					PhotoID: photo.PhotoID,
					Big:     false,
				}, nil
			}
		case *tg.Channel:
			if photo, ok := c.Photo.(*tg.ChatPhoto); ok {

				peer := ctx.PeerStorage.GetInputPeerById(peerID)
				if peer == nil {
					return nil, fmt.Errorf("peer not found")
				}

				return &tg.InputPeerPhotoFileLocation{
					Peer:    peer,
					PhotoID: photo.PhotoID,
					Big:     false,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("no photo available for peer %d", peerID)
}
