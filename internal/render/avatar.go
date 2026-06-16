package render

import (
	"fmt"
	"image/color"
	"strings"

	"git.sr.ht/~sbinet/gg"
)

func initials(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "?"
	}
	words := strings.Fields(name)
	if len(words) >= 2 {
		r1 := []rune(words[0])
		r2 := []rune(words[1])
		if len(r1) > 0 && len(r2) > 0 {
			return strings.ToUpper(string(r1[0]) + string(r2[0]))
		}
	}
	r := []rune(words[0])
	if len(r) == 0 {
		return "?"
	}
	return strings.ToUpper(string(r[0]))
}

func avatarGradientColors(base color.Color) (color.RGBA, color.RGBA) {
	r, g, b, a := base.RGBA()
	c := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
	return c, darken(c, 0.55)
}

func darken(c color.RGBA, factor float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c.R) * factor),
		G: uint8(float64(c.G) * factor),
		B: uint8(float64(c.B) * factor),
		A: c.A,
	}
}

func avatarImageLetters(letters string, color1, color2 color.Color, size int) (*gg.Context, error) {
	dc := gg.NewContext(size, size)

	grad := gg.NewLinearGradient(0, 0, float64(size), float64(size))
	grad.AddColorStop(0, color1)
	grad.AddColorStop(1, color2)
	dc.SetFillStyle(grad)
	dc.DrawRectangle(0, 0, float64(size), float64(size))
	dc.Fill()

	letterCount := len([]rune(letters))
	var fontSize float64
	if letterCount > 1 {
		fontSize = float64(size) * 0.38
	} else {
		fontSize = float64(size) * 0.48
	}

	if err := dc.LoadFontFace("./fonts/NotoSans-Bold.ttf", fontSize); err != nil {
		return nil, fmt.Errorf("load font for avatar: %w", err)
	}

	dc.SetColor(color.White)
	dc.DrawStringAnchored(letters, float64(size)/2, float64(size)/2, 0.5, 0.5)

	return dc, nil
}
