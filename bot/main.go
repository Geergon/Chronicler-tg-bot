package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/Geergon/Chronicler-tg-bot/internal/database"
	"github.com/Geergon/Chronicler-tg-bot/internal/tgbot"
	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/glebarez/sqlite"
	"github.com/gotd/td/tg"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	chatStickerSetDb *sql.DB
	quotesDb         *sql.DB
)

func init() {
	log.SetOutput(io.MultiWriter(os.Stderr, &lumberjack.Logger{
		Filename:   "bot.log",
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}))
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

	for _, c := range []struct {
		name  string
		reply bool
	}{{"c", false}, {"q", false}, {"cr", true}, {"qr", true}} {
		c := c
		dispatcher.AddHandler(handlers.NewCommand(c.name, tgbot.Guard(c.name,
			func(ctx *ext.Context, u *ext.Update) error {
				return tgbot.HandleQuote(ctx, u, c.reply)
			})))
	}

	dispatcher.AddHandler(handlers.NewCommand("cs", tgbot.Guard("cs",
		func(ctx *ext.Context, u *ext.Update) error {
			if err := tgbot.HandleSaveSticker(ctx, u, chatStickerSetDb, quotesDb, client.Self.Username, botToken); err != nil {
				log.Printf("sticker set error: %v", err)
			}
			return nil
		})))

	dispatcher.AddHandler(handlers.NewCommand("crand", tgbot.Guard("cs",
		func(ctx *ext.Context, u *ext.Update) error {
			if err := tgbot.HandleRandomQuotes(ctx, u, quotesDb, botToken); err != nil {
				log.Printf("random quotes error: %v", err)
			}
			return nil
		})))

	dispatcher.AddHandler(handlers.NewCommand("crsave", tgbot.Guard("cs",
		func(ctx *ext.Context, u *ext.Update) error {
			log.Println("/crsave command processing")
			if err := tgbot.SaveStickerSetInDB(ctx, u, quotesDb, botToken); err != nil {
				log.Printf("save stickerset in db error: %v", err)
			}
			return nil
		})))

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

	log.Printf("bot starting: pid=%d renderer=%s", os.Getpid(), os.Getenv("RENDERER_URL"))
	log.Printf("Bot (@%s) started...\n", client.Self.Username)
	client.Idle()
}
