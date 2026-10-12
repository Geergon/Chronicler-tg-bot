package tgbot

import (
	"log"
	"sync"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"github.com/Geergon/Chronicler-tg-bot/internal/renderclient"
)

const (
	maxCustomEmojiPerQuote = 20
	maxCachedEmoji         = 500
)

type emojiCacheT struct {
	mu sync.Mutex
	m  map[int64][]byte
}

var emojiCache = &emojiCacheT{m: map[int64][]byte{}}

func (c *emojiCacheT) get(id int64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.m[id]
	return b, ok
}

func (c *emojiCacheT) put(id int64, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.m) >= maxCachedEmoji {
		clear(c.m)
	}
	c.m[id] = b
}

func attachCustomEmoji(ctx *ext.Context, segs []renderclient.Segment) {
	var ids []int64
	seen := map[int64]bool{}
	for _, s := range segs {
		if id := s.CustomEmojiID; id != 0 && !seen[id] && len(seen) < maxCustomEmojiPerQuote {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	imgs := loadCustomEmoji(ctx, ids)
	for i := range segs {
		if b, ok := imgs[segs[i].CustomEmojiID]; ok {
			segs[i].EmojiImage = b
		}
	}
}

func bestThumbSize(doc *tg.Document) string {
	best, bestArea := "", 0
	for _, t := range doc.Thumbs {
		ps, ok := t.(*tg.PhotoSize)
		if !ok {
			continue
		}
		if area := ps.W * ps.H; area > bestArea {
			best, bestArea = ps.Type, area
		}
	}
	return best
}

func statusEmojiID(u *tg.User) int64 {
	st, ok := u.GetEmojiStatus()
	if !ok {
		return 0
	}
	now := int(time.Now().Unix())
	switch s := st.(type) {
	case *tg.EmojiStatus:
		if until, ok := s.GetUntil(); ok && until != 0 && until < now {
			return 0
		}
		return s.DocumentID
	case *tg.EmojiStatusCollectible:
		if until, ok := s.GetUntil(); ok && until != 0 && until < now {
			return 0
		}
		return s.DocumentID
	}
	return 0
}

func loadCustomEmoji(ctx *ext.Context, ids []int64) map[int64][]byte {
	out := map[int64][]byte{}
	var need []int64
	for _, id := range ids {
		if b, cached := emojiCache.get(id); cached {
			if len(b) > 0 {
				out[id] = b
			}
			continue
		}
		need = append(need, id)
	}
	if len(need) > 0 {
		docs, err := ctx.Raw.MessagesGetCustomEmojiDocuments(ctx, need)
		if err != nil {
			log.Printf("getCustomEmojiDocuments: %v", err)
			return nil
		}

		var wg sync.WaitGroup
		sem := make(chan struct{}, 4) // no more than 4 downloads at a time

		got := map[int64]bool{}
		for _, dc := range docs {
			doc, ok := dc.(*tg.Document)
			if !ok {
				continue
			}
			got[doc.ID] = true
			log.Printf("custom emoji %d: mime=%s dc=%d size=%d thumbs=%d",
				doc.ID, doc.MimeType, doc.DCID, doc.Size, len(doc.Thumbs))
			for _, t := range doc.Thumbs {
				log.Printf("  thumb %T %+v", t, t)
			}

			thumb := ""
			if doc.MimeType != "image/webp" {
				thumb = bestThumbSize(doc)
				if thumb == "" {
					emojiCache.put(doc.ID, nil)
					continue
				}
			}

			wg.Add(1)
			sem <- struct{}{}
			go func(doc *tg.Document, thumb string) {
				defer wg.Done()
				defer func() { <-sem }()

				b, err := downloadFile(ctx, &tg.InputDocumentFileLocation{
					ID:            doc.ID,
					AccessHash:    doc.AccessHash,
					FileReference: doc.FileReference,
					ThumbSize:     thumb,
				})
				if err != nil {
					log.Printf("custom emoji %d download: %v", doc.ID, err)
					return
				}
				emojiCache.put(doc.ID, b)
			}(doc, thumb)
		}
		for _, id := range need {
			if !got[id] {
				log.Printf("custom emoji %d: not returned by Telegram", id)
			}
		}

		wg.Wait()
		for _, id := range need {
			if b, ok := emojiCache.get(id); ok && len(b) > 0 {
				out[id] = b
			}
		}
	}
	return out
}
