package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	"github.com/Geergon/Chronicler-tg-bot/internal/config"
	"github.com/Geergon/Chronicler-tg-bot/internal/database"
	"github.com/Geergon/Chronicler-tg-bot/internal/render"
	"github.com/Geergon/Chronicler-tg-bot/internal/tgbot"
	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/fsnotify/fsnotify"
	"github.com/glebarez/sqlite"
	"github.com/gotd/td/tg"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	chatStickerSetDb *sql.DB
	quotesDb         *sql.DB
	viperMutex       sync.RWMutex
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
	initViper()

	if err := render.InitFonts(); err != nil {
		fmt.Printf("render init fonts: %v\n", err)
		log.Fatal("render init fonts:", err)
	}

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
		fmt.Printf("invalid APP_ID\n")
		log.Fatal("invalid APP_ID")
	}

	botToken := os.Getenv("BOT_TOKEN")
	if botToken == "" {
		fmt.Printf("invalid APP_ID\n")
		log.Fatal("invalid APP_ID")
	}

	_ = os.MkdirAll("./db", 0755)

	chatStickerSetDb, err = database.InitQuotesDB("./db/chatStickerSet.db")
	if err != nil {
		log.Fatal(err)
	}
	chatStickerSetDb.SetMaxOpenConns(1)
	if _, err := chatStickerSetDb.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("failed to enable WAL: %v", err)
	}
	defer chatStickerSetDb.Close()

	quotesDb, err = database.InitDB("./db/quotes.db")
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

	dispatcher := client.Dispatcher

	dispatcher.AddHandler(handlers.NewCommand("q", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleQuote(ctx, update); err != nil {
				log.Printf("quote error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("qs", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleSaveSticker(ctx, update, chatStickerSetDb, client.Self.Username, botToken); err != nil {
				log.Printf("sticker set error: %v", err)
			}
		}()
		return nil
	}))

	dispatcher.AddHandler(handlers.NewCommand("qrand", func(ctx *ext.Context, update *ext.Update) error {
		go func() {
			if err := tgbot.HandleRandomQuotes(ctx, update, chatStickerSetDb, botToken); err != nil {
				log.Printf("random quotes error: %v", err)
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

	dispatcher.AddHandler(handlers.NewCommand("qlogs", func(ctx *ext.Context, update *ext.Update) error {
		tgbot.SendLogs(ctx, update)
		return nil
	}))

	fmt.Printf("Bot (@%s) started...\n", client.Self.Username)
	client.Idle()
}

func initViper() {
	viperMutex.Lock()
	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath("./config")

	_ = os.MkdirAll("./config", 0755)

	viper.SetDefault("ui.padding_x", 10)
	viper.SetDefault("ui.padding_y", 12)
	viper.SetDefault("ui.name_font_size", 46)
	viper.SetDefault("ui.reply_font_size", 36)
	viper.SetDefault("ui.text_font_size", 58)
	viper.SetDefault("ui.gap_after_name", 2)
	viper.SetDefault("ui.gap_after_reply", 6)
	viper.SetDefault("ui.reply_line_width", 5)
	viper.SetDefault("ui.reply_line_gap", 1)
	viper.SetDefault("ui.corner_radius", 18)
	viper.SetDefault("ui.avatar_size", 45)
	viper.SetDefault("ui.avatar_gap", 8)
	viper.SetDefault("ui.vertical_gap", 4)
	viper.SetDefault("ui.media_radius", 12)
	viper.SetDefault("ui.media_group_gap", 6)
	viper.SetDefault("ui.gap_after_media", 10)

	err := viper.SafeWriteConfig()
	if _, ok := err.(viper.ConfigFileAlreadyExistsError); !ok {
		log.Println("save config error: ", err)
	}

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			log.Printf("config file error: %v", err)
		}
	} else {
		config.UpdateFromViper()
	}

	viperMutex.Unlock()
	viper.WatchConfig()

	viper.OnConfigChange(func(e fsnotify.Event) {
		viperMutex.Lock()
		defer viperMutex.Unlock()

		log.Printf("configuration changed: %s", e.Name)

		if err := viper.ReadInConfig(); err != nil {
			log.Printf("configuration reading error: %v", err)
			return
		}

		config.UpdateFromViper()
	})
}
