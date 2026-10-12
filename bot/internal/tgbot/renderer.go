package tgbot

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"github.com/Geergon/Chronicler-tg-bot/internal/renderclient"
)

var renderer *renderclient.Client

func InitRenderer(ctx context.Context, url string) error {
	renderer = renderclient.New(url)

	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for {
		err := renderer.Ping(waitCtx)
		if err == nil {
			return nil
		}
		select {
		case <-time.After(2 * time.Second):
		case <-waitCtx.Done():
			return fmt.Errorf("renderer is not ready: %w (last: %v)", waitCtx.Err(), err)
		}
	}
}

func renderAndSend(ctx *ext.Context, chatID int64, msgs []renderclient.Message) error {
	rctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()

	t0 := time.Now()
	log.Println("send request to renderer backend")
	buffs, err := renderer.Render(rctx, msgs)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}
	log.Printf("[chat=%d] rendered: %d messages -> %d bytes in %s",
		chatID, len(msgs), len(buffs), time.Since(t0).Round(time.Millisecond))

	t1 := time.Now()
	log.Println("upload media")
	docClass, err := uploadMedia(ctx, chatID, buffs, "image/webp")
	if err != nil {
		return fmt.Errorf("uploadMedia: %w", err)
	}
	log.Printf("[chat=%d] uploaded in %s", chatID, time.Since(t1).Round(time.Millisecond))

	t2 := time.Now()
	doc, ok := docClass.(*tg.Document)
	if !ok {
		return fmt.Errorf("unexpected doc type: %T", docClass)
	}
	log.Printf("send sticker to chat %d", chatID)
	err = sendStickerToChat(ctx, chatID, doc)
	log.Printf("[chat=%d] sent doc=%d in %s (err=%v)", chatID, doc.ID, time.Since(t2).Round(time.Millisecond), err)
	return err
}
