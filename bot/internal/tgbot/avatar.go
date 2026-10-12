package tgbot

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

type avatarCache struct {
	mu sync.Mutex
	m  map[int64][]byte // key: PhotoID
}

var avatars = &avatarCache{m: map[int64][]byte{}}

const maxCachedAvatars = 300

func (c *avatarCache) get(k int64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[k]
	return b, ok
}

func (c *avatarCache) put(k int64, v []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxCachedAvatars {
		clear(c.m)
	}
	c.m[k] = v
}

func avatarPhotoID(users map[int64]*tg.User, chats map[int64]tg.ChatClass, id int64) int64 {
	if u, ok := users[id]; ok {
		if p, ok := u.Photo.(*tg.UserProfilePhoto); ok {
			return p.PhotoID
		}
	}
	switch c := chats[id].(type) {
	case *tg.Channel:
		if p, ok := c.Photo.(*tg.ChatPhoto); ok {
			return p.PhotoID
		}
	case *tg.Chat:
		if p, ok := c.Photo.(*tg.ChatPhoto); ok {
			return p.PhotoID
		}
	}
	return 0
}

// single entry point for both modes (/q and /q N)
func getAvatar(ctx *ext.Context, id int64, username string,
	users map[int64]*tg.User, chats map[int64]tg.ChatClass) []byte {

	pid := avatarPhotoID(users, chats, id)
	if pid != 0 {
		if b, ok := avatars.get(pid); ok {
			return b
		}
	}

	var avatar []byte
	if loc, err := GetAvatarLocationFromPeer(users, chats, ctx, id); err == nil && loc != nil {
		if b, err := downloadFile(ctx, loc); err == nil {
			avatar = b
		} else {
			log.Printf("avatar download failed for %d: %v", id, err)
		}
	}
	if avatar == nil {
		b, err := fetchAvatar(ctx, id, username, users, chats)
		if err != nil {
			log.Printf("fetchAvatar error: %v", err)
		}
		avatar = b
	}

	if avatar != nil && pid != 0 {
		avatars.put(pid, avatar)
	}
	log.Printf("avatar user=%d bytes=%d", id, len(avatar)) // cache | telegram | telega.one | none
	return avatar
}

func fetchAvatar(ctx *ext.Context, id int64, username string, users map[int64]*tg.User, chatMap map[int64]tg.ChatClass) ([]byte, error) {
	if chat, ok := chatMap[id]; ok {
		img, err := fetchChatAvatarFromChatClass(ctx, chat, chatMap)
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
		log.Printf("username == %s", username)
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

func fetchChatAvatarFromChatClass(ctx *ext.Context, chat tg.ChatClass, chatMap map[int64]tg.ChatClass) ([]byte, error) {
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

	inputPeer := inputPeerFor(getPeerID(chat), nil, chatMap)

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

func fetchUserAvatar(ctx *ext.Context, userID int64, username string) ([]byte, error) {
	inputPeer := ctx.PeerStorage.GetInputPeerById(userID)
	log.Printf("fetchUserAvatar: userID=%d inputPeer=%T", userID, inputPeer)
	if inputPeer == nil {
		log.Println("input peer is empty")
		return nil, fmt.Errorf("input peer is empty")
	}
	inputUser, ok := toInputUser(inputPeer)
	if !ok {
		return nil, fmt.Errorf("cannot resolve user %d", userID)
	}

	fullUser, err := ctx.Raw.UsersGetFullUser(ctx, inputUser)
	if err == nil {
		var user *tg.User
		for _, u := range fullUser.Users {
			if uu, ok := u.(*tg.User); ok && uu.ID == userID {
				user = uu
				break
			}
		}
		if user != nil {
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

func downloadImageFromURL(url string) ([]byte, error) {
	var httpClient = &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/") {
		return nil, fmt.Errorf("unexpected content type %q", ct)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 5<<20))
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

func GetAvatarLocationFromPeer(userMap map[int64]*tg.User, chatMap map[int64]tg.ChatClass, ctx *ext.Context, peerID int64) (tg.InputFileLocationClass, error) {
	if u, ok := userMap[peerID]; ok && u.Photo != nil {
		if userPhoto, ok := u.Photo.(*tg.UserProfilePhoto); ok {
			inputPeer := inputPeerFor(peerID, userMap, chatMap)
			if inputPeer == nil {
				inputPeer = ctx.PeerStorage.GetInputPeerById(peerID)
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
				inputPeer := inputPeerFor(peerID, userMap, chatMap)
				if inputPeer == nil {
					inputPeer = ctx.PeerStorage.GetInputPeerById(peerID)
				}

				return &tg.InputPeerPhotoFileLocation{
					Peer:    inputPeer,
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

func inputPeerFor(id int64, users map[int64]*tg.User, chats map[int64]tg.ChatClass) tg.InputPeerClass {
	if u, ok := users[id]; ok {
		return &tg.InputPeerUser{UserID: u.ID, AccessHash: u.AccessHash}
	}
	switch c := chats[id].(type) {
	case *tg.Channel:
		return &tg.InputPeerChannel{ChannelID: c.ID, AccessHash: c.AccessHash}
	case *tg.Chat:
		return &tg.InputPeerChat{ChatID: c.ID}
	}
	return nil
}
