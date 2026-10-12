package tgbot

import (
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"github.com/celestix/gotgproto/ext"
)

const (
	maxCommandAge = 2 * time.Minute
	dedupeTTL     = 30 * time.Minute
)

var seen = struct {
	sync.Mutex
	m map[string]time.Time
}{m: map[string]time.Time{}}

func firstTime(key string) bool {
	seen.Lock()
	defer seen.Unlock()
	now := time.Now()
	for k, t := range seen.m {
		if now.Sub(t) > dedupeTTL {
			delete(seen.m, k)
		}
	}
	if _, ok := seen.m[key]; ok {
		return false
	}
	seen.m[key] = now
	return true
}

type Handler func(ctx *ext.Context, update *ext.Update) error

func Guard(name string, fn func(ctx *ext.Context, update *ext.Update) error) func(ctx *ext.Context, update *ext.Update) error {
	return func(ctx *ext.Context, update *ext.Update) error {
		msg := update.EffectiveMessage
		if msg == nil {
			return nil
		}
		chatID := update.EffectiveChat().GetID()
		userID := update.EffectiveUser().ID
		age := time.Since(time.Unix(int64(msg.Date), 0)).Round(time.Second)
		tag := fmt.Sprintf("[%s chat=%d msg=%d]", name, chatID, msg.ID)

		log.Printf("%s received from user=%d, age=%s", tag, userID, age)

		if age > maxCommandAge {
			log.Printf("%s skipped: stale command (%s old), likely a replay after restart", tag, age)
			return nil
		}
		if !firstTime(fmt.Sprintf("%d:%d", chatID, msg.ID)) {
			log.Printf("%s skipped: duplicate delivery", tag)
			return nil
		}

		go func() {
			start := time.Now()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("%s PANIC: %v\n%s", tag, r, debug.Stack())
				}
			}()
			if err := fn(ctx, update); err != nil {
				log.Printf("%s failed after %s: %v", tag, time.Since(start).Round(time.Millisecond), err)
				return
			}
			log.Printf("%s done in %s", tag, time.Since(start).Round(time.Millisecond))
		}()
		return nil
	}
}
