package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/Geergon/Chronicler-tg-bot/internal/database"
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

var (
	chatStickerSetDb *sql.DB
	quotesDb         *sql.DB
	viperMutex       sync.RWMutex
)

func init() {
	log.SetOutput(&lumberjack.Logger{
		Filename:   "bot.log",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	})

	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}
}

func main() {
	a := os.Getenv("APP_ID")
	if a == "" {
		fmt.Printf("invalid APP_ID\n")
		log.Fatal("invalid APP_ID")
	}

	appId, err := strconv.Atoi(a)
	if err != nil {
		log.Fatalf("failed to parse APP_ID: %v\n", err)
	}

	apiHash := os.Getenv("API_HASH")
	if apiHash == "" {
		fmt.Printf("invalid API_HAS\n")
		log.Fatal("invalid API_HASH")
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		fmt.Printf("invalid BOT_TOKEN\n")
		log.Fatal("invalid BOT_TOKEN")
	}

	_ = os.MkdirAll("./db", 0755)

	chatStickerSetDb, err = database.InitDB("./db/chatStickerSet.db")
	if err != nil {
		log.Fatal(err)
	}
	chatStickerSetDb.SetMaxOpenConns(1)
	if _, err := chatStickerSetDb.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("failed to enable WAL: %v", err)
	}
	defer chatStickerSetDb.Close()

	quotesDb, err = database.InitQuotesDB("./db/quotes.db")
	if err != nil {
		log.Fatal(err)
	}
	quotesDb.SetMaxOpenConns(1)
	if _, err := quotesDb.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("failed to enable WAL: %v", err)
	}
	defer quotesDb.Close()

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

	if err := tgbot.InitRenderer(context.Background(), os.Getenv("RENDERER_URL")); err != nil {
		log.Fatalf("renderer: %v", err)
	}

	dispatcher := client.Dispatcher

	dispatcher.AddHandler(handlers.NewCommand("c", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleQuote(ctx, update, false); err != nil {
				log.Printf("quote error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("q", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleQuote(ctx, update, false); err != nil {
				log.Printf("quote error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("cr", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleQuote(ctx, update, true); err != nil {
				log.Printf("quote error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("qr", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleQuote(ctx, update, true); err != nil {
				log.Printf("quote error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("cs", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleSaveSticker(ctx, update, chatStickerSetDb, quotesDb, client.Self.Username, botToken); err != nil {
				log.Printf("sticker set error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("crand", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleRandomQuotes(ctx, update, quotesDb, botToken); err != nil {
				log.Printf("random quotes error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("crsave", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.SaveStickerSetInDB(ctx, update, quotesDb, botToken); err != nil {
				log.Printf("save stickerset in db error: %v", err)
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

	dispatcher.AddHandler(handlers.NewCommand("chelp", func(ctx *ext.Context, u *ext.Update) error {
		chatID := u.EffectiveChat().GetID()
		_, err := ctx.SendMessage(chatID, &tg.MessagesSendMessageRequest{
			Message: `
/q або /c - створення цитати.
/qr або /cr - створення цитати з реплаєм.
/crand - надіслати рандомний стікер.
/crsave - зберегти стікерпак в базу даних для рандомних стікерів.
`,
		})
		if err != nil {
			log.Printf("Помилка надсилання повідомлення: %v", err)
			return err
		}
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("qlogs", func(ctx *ext.Context, update *ext.Update) error {
		tgbot.SendLogs(ctx, update)
		return nil
	}))

	fmt.Printf("Bot (@%s) started...\n", client.Self.Username)
	client.Idle()
}
