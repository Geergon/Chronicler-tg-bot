package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
	"github.com/tdewolff/canvas"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	notoSansFamily *canvas.FontFamily
	notoMonoFamily *canvas.FontFamily
	emojiFamily    *canvas.FontFamily
)

func init() {
	logFile, err := os.OpenFile("bot.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatalf("error opening log file: %v", err)
	}
	log.SetOutput(&lumberjack.Logger{
		Filename:   "bot.log",
		MaxSize:    10, // МБ
		MaxBackups: 3,
		MaxAge:     28, // дні
		Compress:   true,
	})
	log.SetOutput(logFile)
}

func main() {
	botToken := os.Getenv("TOKEN")

	// TODO: remove later telego.With Default DebugLogger()
	bot, err := telego.NewBot(botToken, telego.WithDefaultDebugLogger())
	if err != nil {
		log.Fatalf("failed to initialize bot: %v", err)
	}

	ctx := context.Background()
	updates, _ := bot.UpdatesViaLongPolling(ctx, nil)

	bh, _ := th.NewBotHandler(bot, updates)
	defer func() { _ = bh.Stop() }()

	bh.Handle(func(ctx *th.Context, update telego.Update) error {
		_, _ = ctx.Bot().SendMessage(ctx, tu.Message(
			tu.ID(update.Message.Chat.ID),
			fmt.Sprint("Вас вітає бот літописець для збереження цитат в стікери.", update.Message.From.FirstName),
		))
		return nil
	}, th.CommandEqual("start"))
	bh.Handle(func(ctx *th.Context, update telego.Update) error {
		return nil
	}, th.CommandEqual("q"))
}
