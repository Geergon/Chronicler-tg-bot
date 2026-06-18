package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/Geergon/Chronicler-tg-bot/internal/render"
	"github.com/Geergon/Chronicler-tg-bot/internal/tgbot"
	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/glebarez/sqlite"
	"github.com/gotd/td/tg"
	"github.com/joho/godotenv"
	"gopkg.in/natefinch/lumberjack.v2"
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

	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}
}

func main() {
	if err := render.InitFonts(); err != nil {
		log.Fatal("render init fonts:", err)
	}

	a, isAppIdExist := os.LookupEnv("APP_ID")
	if !isAppIdExist {
		log.Fatal("invalid APP_ID")
	}
	appId, err := strconv.Atoi(a)
	if err != nil {
		fmt.Errorf("failed to get appID: %v", err)
	}

	apiHash, isHashExist := os.LookupEnv("API_HASH")
	if !isHashExist {
		log.Fatal("invalid  API_HASH")
	}

	botToken, isTokenExist := os.LookupEnv("TOKEN")
	if !isTokenExist {
		log.Fatal("invalid BOT_TOKEN")
	}

	client, err := gotgproto.NewClient(
		// Get AppID from https://my.telegram.org/apps
		appId,
		// Get ApiHash from https://my.telegram.org/apps
		apiHash,
		// ClientType, as we defined above
		gotgproto.ClientTypeBot(botToken),
		// Optional parameters of client
		&gotgproto.ClientOpts{
			Session:               sessionMaker.SqlSession(sqlite.Open("./db/session")),
			AutoFetchReply:        true,
			FetchEntireReplyChain: true,
		},
	)
	if err != nil {
		log.Fatalln("failed to start bot:", err)
	}

	dispatcher := client.Dispatcher

	dispatcher.AddHandler(handlers.NewCommand("q", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleQuote(ctx, update); err != nil {
				log.Printf("quote error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("start", func(ctx *ext.Context, u *ext.Update) error {
		chatID := u.EffectiveChat().GetID()
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: `
Вас вітає бот літописець для створення цитат (стікерів) з повідомлень.
Напишіть /q для створення цитати.
`,
		})
		if err != nil {
			log.Printf("Помилка надсилання повідомлення: %v", err)
			return err
		}
		return nil
	}))

	fmt.Printf("Bot (@%s) started...\n", client.Self.Username)
	client.Idle()
}
