package tgbot

import (
	"log"
	"sync"
	"time"

	"github.com/celestix/gotgproto/ext"
	"github.com/gotd/td/tg"

	"github.com/Geergon/Chronicler-tg-bot/internal/renderclient"
)

type roleInfo struct {
	Type renderclient.TagType // owner | admin | member
	Rank string               // custom tag ("" = not specified)
}

type rolesEntry struct {
	at    time.Time
	roles map[int64]roleInfo
}

var (
	rolesCache   = map[int64]rolesEntry{}
	rolesCacheMu sync.Mutex
)

const rolesTTL = 5 * time.Minute

func getChatRoles(ctx *ext.Context, chatID int64) map[int64]roleInfo {
	rolesCacheMu.Lock()
	if e, ok := rolesCache[chatID]; ok && time.Since(e.at) < rolesTTL {
		rolesCacheMu.Unlock()
		log.Printf("roles: cache hit chat=%d (%d entries)", chatID, len(e.roles))
		return e.roles
	}
	rolesCacheMu.Unlock()

	var roles map[int64]roleInfo
	switch peer := ctx.PeerStorage.GetInputPeerById(chatID).(type) {
	case *tg.InputPeerChannel:
		roles = fetchChannelAdmins(ctx, peer)
	case *tg.InputPeerChat:
		roles = fetchBasicGroupRoles(ctx, peer.ChatID)
	}

	rolesCacheMu.Lock()
	rolesCache[chatID] = rolesEntry{at: time.Now(), roles: roles}
	rolesCacheMu.Unlock()
	log.Printf("roles: fetched chat=%d, %d privileged users", chatID, len(roles))
	return roles
}

func fetchChannelAdmins(ctx *ext.Context, peer *tg.InputPeerChannel) map[int64]roleInfo {
	res, err := ctx.Raw.ChannelsGetParticipants(ctx, &tg.ChannelsGetParticipantsRequest{
		Channel: &tg.InputChannel{ChannelID: peer.ChannelID, AccessHash: peer.AccessHash},
		Filter:  &tg.ChannelParticipantsAdmins{},
		Limit:   200,
	})
	if err != nil {
		log.Printf("fetchChannelAdmins: %v", err)
		return nil
	}
	data, ok := res.(*tg.ChannelsChannelParticipants)
	if !ok {
		return nil
	}

	out := make(map[int64]roleInfo, len(data.Participants))
	for _, p := range data.Participants {
		switch v := p.(type) {
		case *tg.ChannelParticipantCreator:
			rank, _ := v.GetRank()
			out[v.UserID] = roleInfo{renderclient.TagOwner, rank}
		case *tg.ChannelParticipantAdmin:
			rank, _ := v.GetRank()
			out[v.UserID] = roleInfo{renderclient.TagAdmin, rank}
		}
	}
	return out
}

func fetchBasicGroupRoles(ctx *ext.Context, chatID int64) map[int64]roleInfo {
	full, err := ctx.Raw.MessagesGetFullChat(ctx, chatID)
	if err != nil {
		log.Printf("fetchBasicGroupRoles: %v", err)
		return nil
	}
	chatFull, ok := full.FullChat.(*tg.ChatFull)
	if !ok {
		return nil
	}
	parts, ok := chatFull.Participants.(*tg.ChatParticipants)
	if !ok {
		return nil
	}

	out := make(map[int64]roleInfo, len(parts.Participants))
	for _, p := range parts.Participants {
		switch v := p.(type) {
		case *tg.ChatParticipantCreator:
			rank, _ := v.GetRank()
			out[v.UserID] = roleInfo{renderclient.TagOwner, rank}
		case *tg.ChatParticipantAdmin:
			rank, _ := v.GetRank()
			out[v.UserID] = roleInfo{renderclient.TagAdmin, rank}
		case *tg.ChatParticipant:
			if rank, ok := v.GetRank(); ok && rank != "" {
				out[v.UserID] = roleInfo{renderclient.TagMember, rank}
			}
		}
	}
	return out
}

// fromRank it's message.from_rank (only for supergroup).
func resolveTag(roles map[int64]roleInfo, userID int64, fromRank string) *renderclient.Tag {
	role, known := roles[userID]

	switch {
	case known && role.Type == renderclient.TagOwner:
		return &renderclient.Tag{TagText: firstNonEmpty(role.Rank, fromRank, "owner"), TagType: renderclient.TagOwner}
	case known && role.Type == renderclient.TagAdmin:
		return &renderclient.Tag{TagText: firstNonEmpty(role.Rank, fromRank, "admin"), TagType: renderclient.TagAdmin}
	}

	text := firstNonEmpty(fromRank, role.Rank) // member
	if text == "" {
		return nil
	}
	return &renderclient.Tag{TagText: text, TagType: renderclient.TagMember}
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}
