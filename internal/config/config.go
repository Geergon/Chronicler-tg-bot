package config

import (
	"log"
	"sync"

	"github.com/spf13/viper"
)

type UIConfig struct {
	PaddingX       int `mapstructure:"padding_x"`
	PaddingY       int `mapstructure:"padding_y"`
	NameFontSize   int `mapstructure:"name_font_size"`
	ReplyFontSize  int `mapstructure:"reply_font_size"`
	TextFontSize   int `mapstructure:"text_font_size"`
	GapAfterName   int `mapstructure:"gap_after_name"`
	GapAfterReply  int `mapstructure:"gap_after_reply"`
	ReplyLineWidth int `mapstructure:"reply_line_width"`
	ReplyLineGap   int `mapstructure:"reply_line_gap"`
	CornerRadius   int `mapstructure:"corner_radius"`
	AvatarSize     int `mapstructure:"avatar_size"`
	AvatarGap      int `mapstructure:"avatar_gap"`
	VerticalGap    int `mapstructure:"vertical_gap"`
	MediaRadius    int `mapstructure:"media_radius"`
	MediaGroupGap  int `mapstructure:"media_group_gap"`
	GapAfterMedia  int `mapstructure:"gap_after_media"`
}

var (
	cfgMutex  sync.RWMutex
	currentUI UIConfig
)

func GetUI() UIConfig {
	cfgMutex.RLock()
	defer cfgMutex.RUnlock()
	return currentUI
}

func UpdateFromViper() {
	cfgMutex.Lock()
	defer cfgMutex.Unlock()

	var temp struct {
		UI UIConfig `mapstructure:"ui"`
	}

	if err := viper.Unmarshal(&temp); err != nil {
		log.Printf("Помилка анмаршалінгу конficу: %v", err)
		return
	}

	currentUI = temp.UI
}
