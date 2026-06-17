package tgbot

import (
	"image"
	"log"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"
)

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

// func refreshAndFetchMedia(ctx *ext.Context, chatID int64, msg *tg.Message) ([]image.Image, error) {
// 	// Робимо свіжий запит до API, щоб Telegram дав актуальний FileReference
// 	refreshedMsg, _, err := fetchMessage(ctx, chatID, msg.ID) // ваша функція з попереднього кроку
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to refresh message for media: %w", err)
// 	}
//
// 	// Тепер викликаємо ваш fetchMedia, але передаємо туди ОНОВЛЕНЕ повідомлення
// 	return fetchMedia(ctx, refreshedMsg)
// }
