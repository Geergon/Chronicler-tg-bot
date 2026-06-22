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

var faceDecorations sync.Map

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
		baseFamily := family
		if seg.Mono {
			baseFamily = monoFamily
		}

		fallbackFace := baseFamily.Face(fontSize, seg.Color, style)

		if seg.Underline || seg.Strikethrough || seg.Spoiler {
			faceDecorations.Store(fallbackFace, faceDecoration{
				Underline:     seg.Underline,
				Strikethrough: seg.Strikethrough,
				Spoiler:       seg.Spoiler,
				Color:         seg.Color,
			})
		}

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
				tokens = append(tokens, token{text: e.Character, face: fallbackFace})
				continue
			}

			if part.text == "" {
				continue
			}

			runes := []rune(part.text)
			groupStart := 0
			currentFamily, currentScale := familyForRune(runes[0], baseFamily)

			for i := 1; i <= len(runes); i++ {
				var nextFamily *canvas.FontFamily
				var nextScale float64
				if i < len(runes) {
					nextFamily, nextScale = familyForRune(runes[i], baseFamily)
				}

				if i == len(runes) || nextFamily != currentFamily {
					chunk := string(runes[groupStart:i])
					face := currentFamily.Face(fontSize*currentScale, seg.Color, style)

					if seg.Underline || seg.Strikethrough || seg.Spoiler {
						faceDecorations.Store(face, faceDecoration{
							Underline:     seg.Underline,
							Strikethrough: seg.Strikethrough,
							Spoiler:       seg.Spoiler,
							Color:         seg.Color,
						})
					}

					tokens = append(tokens, token{text: chunk, face: face})
					groupStart = i
					if i < len(runes) {
						currentFamily = nextFamily
						currentScale = nextScale
					}
				}
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

	text := shrinkWrapText(rtFactory, maxWidth)

	bounds := text.Bounds()
	w, h := bounds.W(), bounds.H()

	if w <= 0 || h <= 0 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), 0, 0, nil
	}

	c := canvas.New(w, h)
	ctx := canvas.NewContext(c)
	ctx.DrawText(-bounds.X0, -bounds.Y0, text)

	drawSpanDecorations(ctx, text, -bounds.X0, -bounds.Y0, fontSize)

	img := rasterizer.Draw(c, canvas.DPMM(1.0), canvas.DefaultColorSpace)

	return img, w, h, nil
}

func drawSpanDecorations(ctx *canvas.Context, text *canvas.Text, offsetX, offsetY, fontSize float64) {
	for _, line := range text.GetLines() {
		baselineY := offsetY - line.Y

		for _, span := range line.Spans {
			dec, ok := faceDecorations.Load(span.Face)
			if !ok {
				continue
			}
			d := dec.(faceDecoration)

			spanX := offsetX + span.X
			spanW := span.Width

			if spanW <= 0 {
				continue
			}

			if d.Strikethrough {
				strikeY := baselineY - fontSize*(-0.07)
				ctx.SetFillColor(d.Color)
				ctx.DrawPath(spanX, strikeY, canvas.Rectangle(spanW, fontSize*0.04))
				ctx.Fill()
			}

			if d.Underline {
				underY := baselineY - fontSize*0.08
				ctx.SetFillColor(d.Color)
				ctx.DrawPath(spanX, underY, canvas.Rectangle(spanW, fontSize*0.06))
				ctx.Fill()
			}

			if d.Spoiler {
				r, g, b, _ := d.Color.RGBA()
				spoilerCol := color.RGBA{
					R: uint8(r >> 8),
					G: uint8(g >> 8),
					B: uint8(b >> 8),
					A: 220, // ~85% opacity
				}
				blockH := fontSize * 1.1
				blockY := baselineY - fontSize*0.85
				ctx.SetFillColor(spoilerCol)
				ctx.DrawPath(spanX, blockY, canvas.Rectangle(spanW, blockH))
				ctx.Fill()
			}
		}
	}
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

func isCuneiform(r rune) bool {
	return r >= 0x12000 && r <= 0x123FF
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0xAC00 && r <= 0xD7AF) || // Hangul
		(r >= 0x3000 && r <= 0x303F) // CJK Symbols and Punctuation
}

func familyForRune(r rune, defaultFamily *canvas.FontFamily) (*canvas.FontFamily, float64) {
	switch {
	case isCuneiform(r):
		return cuneiformFamily, 0.7
	case isCJK(r):
		return cjkFamily, 1.0
	default:
		return defaultFamily, 1.0
	}
}
