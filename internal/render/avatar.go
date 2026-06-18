package render

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"git.sr.ht/~sbinet/gg"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
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

	// Target text size relative to avatar
	var targetRatio float64
	if letterCount > 1 {
		targetRatio = 0.5 // the text takes 50% of the avatar height
	} else {
		targetRatio = 0.6 // 60% for one letter
	}
	targetH := float64(size) * targetRatio

	// Render with a large fontSize to avoid losing quality when scaling
	fontSize := float64(size) * 0.8
	img, w, h, err := renderAvatarText(letters, fontSize, float64(size))
	if err != nil {
		return nil, fmt.Errorf("avatarImageLetters: %w", err)
	}

	if w <= 0 || h <= 0 {
		return dc, nil
	}

	// Scale to targetH while preserving proportions
	scale := targetH / h
	newW := int(math.Round(w * scale))
	newH := int(math.Round(h * scale))

	scaled := resizeImage(img, newW, newH)

	// Center on canvas
	x := (size - newW) / 2
	y := (size - newH) / 2
	dc.DrawImage(scaled, x, y)

	return dc, nil
}

func renderAvatarText(letters string, fontSize float64, maxSize float64) (image.Image, float64, float64, error) {
	segments := []TextSegment{{Text: letters, Color: color.White, Bold: true}}
	tokens := tokenizeSegments(segments, fontSize, notoSansFamily, notoMonoFamily)
	rtFactory := buildRichTextFromTokens(tokens, fontSize)

	text := rtFactory().ToText(maxSize, 0, canvas.Center, canvas.Top, nil)
	bounds := text.Bounds()
	w, h := bounds.W(), bounds.H()

	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), 0, 0, nil
	}

	c := canvas.New(w, h)
	ctx := canvas.NewContext(c)
	ctx.DrawText(-bounds.X0, -bounds.Y0, text)

	img := rasterizer.Draw(c, canvas.DPMM(1.0), canvas.DefaultColorSpace)
	return img, w, h, nil
}
