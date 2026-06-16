package render

import (
	"fmt"
	"image/color"
)

var (
	topColors    = []uint32{0xFFFF845E, 0xFFFEBB5B, 0xFFB694F9, 0xFF9AD164, 0xFF5BCBE3, 0xFF5CAFFA, 0xFFFF8AAC}
	bottomColors = []uint32{0xFFD45246, 0xFFF68136, 0xFF6C61DF, 0xFF46BA43, 0xFF359AD4, 0xFF408ACF, 0xFFD95574}
	// nameColors   = []uint32{0xFFCA5650, 0xFFD87B29, 0xFF9B66DC, 0xFF50B232, 0xFF379EB8, 0xFF4E92CC, 0xFFCF5C95}
	nameColors = []uint32{
		0xFFCA5650, // #ca5650
		0xFFFFA500, // #ffa500
		0xFF9B66DC, // #9b66dc
		0xFF50B232, // #50b232
		0xFF379EB8, // #379eb8
		0xFF4E92CC, // #4e92cc
		0xFFCF5C95, // #cf5c95
	}
)

func AvatarColorFromID(userID int64) (color.RGBA, color.RGBA) {
	if userID < 0 {
		userID = -userID
	}

	colorIndex := userID % 7
	return hexToRGBA(topColors[colorIndex]), hexToRGBA(bottomColors[colorIndex])
}

func nameColorFromID(userID int64) color.RGBA {
	if userID < 0 {
		userID = -userID
	}

	colorIndex := userID % 7

	return hexToRGBA(nameColors[colorIndex])
}

func RgbaToHex(c color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

func hexToRGBA(hex uint32) color.RGBA {
	return color.RGBA{
		R: uint8((hex >> 16) & 0xFF),
		G: uint8((hex >> 8) & 0xFF),
		B: uint8(hex & 0xFF),
		A: uint8((hex >> 24) & 0xFF),
	}
}

func ColorToRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}
