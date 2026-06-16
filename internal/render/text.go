package render

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/forPelevin/gomoji"
	"github.com/tdewolff/canvas"
	"github.com/tdewolff/canvas/renderers/rasterizer"
)

const (
	emojiMarkerStart = '\uE000'
	emojiMarkerEnd   = '\uE001'
)

const (
	emojiDir      = "assets/emoji/apple/"
	emojiSourcePx = 160.0
	emojiScale    = 0.3 // emoji size = fontSize * emojiScale
)

var (
	emojiImgCache   = make(map[string]image.Image)
	emojiNotFound   = make(map[string]bool)
	emojiImgCacheMu sync.Mutex
)

type token struct {
	text     string
	emojiImg image.Image
	face     *canvas.FontFace
}

type markerPart struct {
	text       string
	emojiIndex int // -1, if it's regular text, not emoji
}

func splitMarkers(s string) []markerPart {
	var parts []markerPart
	var sb strings.Builder

	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == emojiMarkerStart {
			if sb.Len() > 0 {
				parts = append(parts, markerPart{text: sb.String(), emojiIndex: -1})
				sb.Reset()
			}
			j := i + 1
			for j < len(runes) && runes[j] != emojiMarkerEnd {
				j++
			}
			idx, _ := strconv.Atoi(string(runes[i+1 : j]))
			parts = append(parts, markerPart{emojiIndex: idx})
			i = j + 1
			continue
		}
		sb.WriteRune(runes[i])
		i++
	}
	if sb.Len() > 0 {
		parts = append(parts, markerPart{text: sb.String(), emojiIndex: -1})
	}
	return parts
}

func tokenizeSegments(segments []TextSegment, fontSize float64, family, monoFamily *canvas.FontFamily) []token {
	var tokens []token

	for _, seg := range segments {
		if seg.Text == "" {
			continue
		}

		style := canvas.FontRegular
		if seg.Bold {
			style |= canvas.FontBold
		}
		if seg.Italic {
			style |= canvas.FontItalic
		}
		f := family
		if seg.Mono {
			f = monoFamily
		}
		face := f.Face(fontSize, seg.Color, style)
		distinctFace := family.Face(fontSize, seg.Color, style)

		var found []gomoji.Emoji
		marked := gomoji.ReplaceEmojisWithFunc(seg.Text, func(e gomoji.Emoji) string {
			found = append(found, e)
			return fmt.Sprintf("%c%d%c", emojiMarkerStart, len(found)-1, emojiMarkerEnd)
		})

		for _, part := range splitMarkers(marked) {
			if part.emojiIndex >= 0 {
				e := found[part.emojiIndex]
				if img, ok := loadEmojiImage(e.CodePoint); ok {
					tokens = append(tokens, token{emojiImg: img})
					continue
				}
				tokens = append(tokens, token{text: e.Character, face: distinctFace})
				continue
			}
			if part.text != "" {
				tokens = append(tokens, token{text: part.text, face: face})
			}
		}
	}

	return tokens
}

func loadEmojiImage(codePoint string) (image.Image, bool) {
	emojiImgCacheMu.Lock()
	defer emojiImgCacheMu.Unlock()

	if img, ok := emojiImgCache[codePoint]; ok {
		return img, true
	}
	if emojiNotFound[codePoint] {
		return nil, false
	}

	for _, name := range codePointToFilenames(codePoint) {
		f, err := os.Open(filepath.Join(emojiDir, name))
		if err != nil {
			continue
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		emojiImgCache[codePoint] = img
		return img, true
	}

	emojiNotFound[codePoint] = true
	log.Printf("emoji image not found for codepoint %q (tried: %v)", codePoint, codePointToFilenames(codePoint))
	return nil, false
}

func codePointToFilenames(codePoint string) []string {
	parts := strings.Fields(codePoint) // "1F1FA 1F1E6" -> ["1F1FA", "1F1E6"]

	build := func(stripFE0F bool) string {
		var hexParts []string
		for _, p := range parts {
			if stripFE0F && strings.EqualFold(p, "FE0F") {
				continue
			}
			hexParts = append(hexParts, strings.ToLower(p))
		}
		return strings.Join(hexParts, "-") + ".png"
	}

	noFE0F := build(true)
	withFE0F := build(false)
	if noFE0F == withFE0F {
		return []string{noFE0F}
	}
	return []string{noFE0F, withFE0F}
}

func InitFonts() error {
	notoSansFamily = canvas.NewFontFamily("noto-sans")
	if err := notoSansFamily.LoadFontFile("./fonts/NotoSans-Regular.ttf", canvas.FontRegular); err != nil {
		return err
	}
	if err := notoSansFamily.LoadFontFile("./fonts/NotoSans-Bold.ttf", canvas.FontBold); err != nil {
		return err
	}
	if err := notoSansFamily.LoadFontFile("./fonts/NotoSans-Italic.ttf", canvas.FontItalic); err != nil {
		return err
	}
	if err := notoSansFamily.LoadFontFile("./fonts/NotoSans-BoldItalic.ttf", canvas.FontBold|canvas.FontItalic); err != nil {
		return err
	}

	notoMonoFamily = canvas.NewFontFamily("noto-mono")
	if err := notoMonoFamily.LoadFontFile("./fonts/NotoSansMono-Regular.ttf", canvas.FontRegular); err != nil {
		return err
	}

	return nil
}

func buildRichTextFromTokens(tokens []token, fontSize float64) func() *canvas.RichText {
	emojiRes := canvas.DPMM(emojiSourcePx / (fontSize * emojiScale))

	var initialFace *canvas.FontFace
	for _, t := range tokens {
		if t.face != nil {
			initialFace = t.face
			break
		}
	}
	if initialFace == nil && len(tokens) > 0 {
		initialFace = notoSansFamily.Face(fontSize, color.RGBA{0, 0, 0, 255}, canvas.FontRegular)
	}

	return func() *canvas.RichText {
		if initialFace == nil {
			return canvas.NewRichText(notoSansFamily.Face(fontSize, color.RGBA{0, 0, 0, 255}, canvas.FontRegular))
		}
		rt := canvas.NewRichText(initialFace)
		for _, t := range tokens {
			if t.emojiImg != nil {
				rt.SetFace(notoMonoFamily.Face(fontSize, color.RGBA{0, 0, 0, 255}, canvas.FontRegular))
				rt.WriteImage(t.emojiImg, emojiRes, canvas.Baseline)
			} else {
				rt.WriteFace(t.face, t.text)
			}
		}
		return rt
	}
}

func RenderRichText(segments []TextSegment, maxWidth, fontSize float64, family, monoFamily *canvas.FontFamily) (image.Image, float64, float64, error) {
	hasContent := false
	for _, seg := range segments {
		if seg.Text != "" {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), 0, 0, nil
	}

	tokens := tokenizeSegments(segments, fontSize, family, monoFamily)
	rtFactory := buildRichTextFromTokens(tokens, fontSize)

	text := shrinkWrapText(rtFactory, maxWidth) // <- замість shrinkWrapWidth + другий ToText

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

func shrinkWrapText(rtFactory func() *canvas.RichText, maxWidth float64) *canvas.Text {
	build := func(w float64) *canvas.Text {
		return rtFactory().ToText(w, 0, canvas.Left, canvas.Top, nil)
	}

	initial := build(maxWidth)
	initialLines := initial.Lines()
	if initialLines <= 1 {
		return initial
	}

	initialWidth := initial.Bounds().W()

	lo := initialWidth / float64(initialLines) * 0.7
	hi := initialWidth
	target := initialLines

	for hi-lo > 2 {
		mid := (lo + hi) / 2
		trial := build(mid)
		if trial.Lines() <= target {
			hi = mid
		} else {
			lo = mid
		}
	}

	return build(math.Ceil(hi))
}
