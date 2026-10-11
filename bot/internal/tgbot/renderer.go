package tgbot

import (
	"context"
	"fmt"
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

	buffs, err := renderer.Render(rctx, msgs)
	if err != nil {
		return fmt.Errorf("render: %w", err)
	}

	docClass, err := uploadMedia(ctx, chatID, buffs, "image/webp")
	if err != nil {
		return fmt.Errorf("uploadMedia: %w", err)
	}
	doc, ok := docClass.(*tg.Document)
	if !ok {
		return fmt.Errorf("unexpected doc type: %T", docClass)
	}
	return sendStickerToChat(ctx, chatID, doc)
}
