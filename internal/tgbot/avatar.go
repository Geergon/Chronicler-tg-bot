package tgbot

import (
	"fmt"
	"image"
	"log"
	"net/http"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

func fetchAvatar(ctx *ext.Context, id int64, username string, chatMap map[int64]tg.ChatClass) (image.Image, error) {
	if chat, ok := chatMap[id]; ok {
		img, err := fetchChatAvatarFromChatClass(ctx, chat)
		if err != nil {
			log.Printf("fetchChatAvatarFromChatClass: %v", err)
		}
		if img != nil {
			return img, nil
		}
	}

	inputPeer := ctx.PeerStorage.GetInputPeerById(id)

	switch inputPeer.(type) {
	case nil, *tg.InputPeerEmpty:
		if username != "" {
			return downloadImageFromURL(
				fmt.Sprintf("https://telega.one/i/userpic/320/%s.jpg", username),
			)
		}
		return nil, nil

	case *tg.InputPeerUser, *tg.InputPeerSelf:
		return fetchUserAvatar(ctx, id, username)

	// case *tg.InputPeerChannel, *tg.InputPeerChat:
	// _ = peer
	// 	return fetchChatAvatar(ctx, id)

	default:
		log.Printf("fetchAvatar: unsupported peer type %T for id %d", inputPeer, id)
		return nil, nil
	}
}

func fetchChatAvatarFromChatClass(ctx *ext.Context, chat tg.ChatClass) (image.Image, error) {
	var photoClass tg.ChatPhotoClass

	switch ch := chat.(type) {
	case *tg.Channel:
		photoClass = ch.Photo
	case *tg.Chat:
		photoClass = ch.Photo
	default:
		return nil, nil
	}

	if photoClass == nil {
		return nil, nil
	}

	chatPhoto, ok := photoClass.(*tg.ChatPhoto)
	if !ok {
		return nil, nil
	}

	inputPeer := ctx.PeerStorage.GetInputPeerById(getPeerID(chat))
	if inputPeer == nil {
		return nil, nil
	}

	location := &tg.InputPeerPhotoFileLocation{
		Peer:    inputPeer,
		PhotoID: chatPhoto.PhotoID,
		Big:     true, // big version (800px), false = small (160px)
	}

	return downloadFile(ctx, location)
}

func getPeerID(chat tg.ChatClass) int64 {
	switch ch := chat.(type) {
	case *tg.Channel:
		return ch.ID
	case *tg.Chat:
		return ch.ID
	}
	return 0
}

func fetchUserAvatar(ctx *ext.Context, userID int64, username string) (image.Image, error) {
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
	if ok {
		fullUser, err := ctx.Raw.UsersGetFullUser(ctx, inputUser)
		if err == nil {
			if user, ok := fullUser.Users[0].(*tg.User); ok {
				if userPhoto, ok := user.Photo.(*tg.UserProfilePhoto); ok {
					location := &tg.InputPeerPhotoFileLocation{
						Peer:    inputPeer,
						PhotoID: userPhoto.PhotoID,
						Big:     true,
					}
					return downloadFile(ctx, location)
				}
			}
			if photo, ok := fullUser.FullUser.ProfilePhoto.(*tg.Photo); ok {
				bestSize := pickBestAvatarSize(photo.Sizes)
				if bestSize != nil {
					img, err := downloadFile(ctx, &tg.InputPhotoFileLocation{
						ID:            photo.ID,
						AccessHash:    photo.AccessHash,
						FileReference: photo.FileReference,
						ThumbSize:     bestSize.Type,
					})
					if err == nil && img != nil {
						return img, nil
					}
				}
			}
		}
	}

	if username != "" {
		url := fmt.Sprintf("https://telega.one/i/userpic/320/%s.jpg", username)
		img, err := downloadImageFromURL(url)
		if err == nil && img != nil {
			log.Printf("fetchUserAvatar: got avatar from telega.one for @%s", username)
			return img, nil
		}
	}

	return nil, nil
}

func downloadImageFromURL(url string) (image.Image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	img, _, err := image.Decode(resp.Body)
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
				Big:     true,
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
					Big:     true,
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
					Big:     true,
				}, nil
			}
		}
	}
	return nil, fmt.Errorf("no photo available for peer %d", peerID)
}
