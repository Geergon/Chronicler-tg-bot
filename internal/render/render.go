package render

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"git.sr.ht/~sbinet/gg"
	"github.com/tdewolff/canvas"
	"golang.org/x/image/draw"
)

var (
	notoSansFamily  *canvas.FontFamily
	notoMonoFamily  *canvas.FontFamily
	cuneiformFamily *canvas.FontFamily
	cjkFamily       *canvas.FontFamily
)

const (
	maxScale       = 3.0
	stickerMaxSide = 512.0
)

func NewSizes(scale float64) Sizes {
	return Sizes{
		Scale:               scale,
		PaddingX:            10 * scale,
		PaddingY:            12 * scale,
		NameFontSize:        46 * scale,
		ReplyFontSize:       36 * scale,
		TextFontSize:        58 * scale,
		GapAfterName:        6 * scale,
		GapAfterReply:       10 * scale,
		ReplyLineWidth:      5 * scale,
		ReplyLineGap:        1 * scale,
		CornerRadius:        18 * scale,
		AvatarSize:          45 * scale,
		AvatarGap:           8 * scale,
		VerticalGap:         4 * scale,
		MediaRadius:         12 * scale,
		MediaGroupGap:       6 * scale,
		MediaMaxHeightRatio: 1.3,
		GapAfterMedia:       10 * scale,
	}
}

func BuildQuoteBubble(p QuoteParams, s Sizes) (*Node, error) {
	maxContentWidth := p.MaxBubbleWidth - s.PaddingX*2

	var children []*Node

	if p.AuthorName != "" {
		nameSegs := []TextSegment{{Text: p.AuthorName, Bold: true, Color: nameColorFromID(p.AuthorID)}}
		nameImg, _, _, err := RenderRichText(nameSegs, maxContentWidth, s.NameFontSize, notoSansFamily, notoMonoFamily)
		if err != nil {
			return nil, err
		}
		children = append(children, Leaf(nameImg, LeafOpts{}))
		children = append(children, spacer(s.GapAfterName))
	}

	if p.Reply != nil {
		replyTextMaxWidth := maxContentWidth - s.ReplyLineWidth - s.ReplyLineGap
		replySegs := []TextSegment{
			{Text: p.Reply.AuthorName + "\n", Bold: true, Color: nameColorFromID(p.Reply.AuthorID)},
			{Text: p.Reply.Text, Color: replyTextColor},
		}
		replyImg, _, replyH, err := RenderRichText(replySegs, replyTextMaxWidth, s.ReplyFontSize, notoSansFamily, notoMonoFamily)
		if err != nil {
			return nil, err
		}

		lineImg := drawReplyLine(s.ReplyLineWidth, replyH, nameColorFromID(p.Reply.AuthorID))

		children = append(children, Box(BoxOpts{
			Dir: "row",
			Gap: s.ReplyLineGap,
			Children: []*Node{
				Leaf(lineImg.Image(), LeafOpts{}),
				Leaf(replyImg, LeafOpts{}),
			},
		}))
		children = append(children, spacer(s.GapAfterReply))
	}

	textImg, textW, _, err := RenderRichText(p.Segments, maxContentWidth, s.TextFontSize, notoSansFamily, notoMonoFamily)
	if err != nil {
		return nil, err
	}
	hasText := textW > 0

	if len(p.Media) > 0 {
		mediaNode := buildMediaNode(p.Media, maxContentWidth, s)
		children = append(children, mediaNode)
		if hasText {
			children = append(children, spacer(s.GapAfterMedia))
		}
	}

	if hasText {
		children = append(children, Leaf(textImg, LeafOpts{}))
	}

	bubble := Box(BoxOpts{
		Dir:  "col",
		Pad:  Pad{T: s.PaddingY, R: s.PaddingX, B: s.PaddingY, L: s.PaddingX},
		MaxW: p.MaxBubbleWidth,
		Bg: func(ctx *gg.Context, n *Node) {
			bg := drawRoundRect(p.BubbleColor, int(n.W), int(n.H), s.CornerRadius)
			ctx.DrawImage(bg.Image(), int(n.X), int(n.Y))
		},
		Children: children,
	})

	return bubble, nil
}

func resizeImage(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	// draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

func drawReplyLine(lineWidth float64, h float64, col color.Color) *gg.Context {
	dc := gg.NewContext(20, int(h))

	dc.SetColor(col)
	dc.SetLineWidth(lineWidth)
	dc.DrawLine(1, 0, 1, float64(h))
	dc.Stroke()

	return dc
}

func BuildStickerChatStack(messages []ChatMessage) (*gg.Context, error) {
	if isStandaloneMedia(messages) {
		return buildStandaloneMedia(messages[0].Media[0])
	}

	root1, err := buildRoot(messages, NewSizes(1))
	if err != nil {
		return nil, err
	}
	Measure(root1)

	naturalW, naturalH := root1.W, root1.H
	maxSide := math.Max(naturalW, naturalH)

	scale := stickerMaxSide / maxSide
	if scale > maxScale {
		scale = maxScale
	}

	root2, err := buildRoot(messages, NewSizes(scale))
	if err != nil {
		return nil, err
	}
	Measure(root2)
	Place(root2, 0, 0, 0)

	dc := gg.NewContext(int(root2.W), int(root2.H))
	Render(dc, root2)

	return finalizeSize(dc), nil
}

func finalizeSize(dc *gg.Context) *gg.Context {
	w, h := dc.Width(), dc.Height()

	var targetW, targetH int
	if w >= h {
		targetW = int(stickerMaxSide)
		targetH = int(math.Round(float64(h) * stickerMaxSide / float64(w)))
	} else {
		targetH = int(stickerMaxSide)
		targetW = int(math.Round(float64(w) * stickerMaxSide / float64(h)))
	}

	if targetW == w && targetH == h {
		return dc
	}

	resized := resizeImage(dc.Image(), targetW, targetH)
	out := gg.NewContext(targetW, targetH)
	out.DrawImage(resized, 0, 0)
	return out
}

func buildRoot(messages []ChatMessage, s Sizes) (*Node, error) {
	maxBubbleWidth := (stickerMaxSide - 80 - 16) * s.Scale // 80/16 = базові AvatarSize/AvatarGap при scale=1

	var rows []*Node
	for i, msg := range messages {
		isFirst := i == 0 || messages[i-1].AuthorID != msg.AuthorID
		block := MessageBlock{AuthorID: msg.AuthorID, Segments: msg.Segments, BubbleColor: msg.BubbleColor, Reply: msg.Reply, Media: msg.Media}
		if isFirst {
			block.AuthorName = msg.AuthorName
			block.AvatarImg = msg.AvatarImg
		}
		row, err := BuildMessageRow(block, maxBubbleWidth, s)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}

	return Box(BoxOpts{Dir: "col", Gap: s.VerticalGap, Children: rows}), nil
}

func BuildMessageRow(m MessageBlock, maxBubbleWidth float64, s Sizes) (*Node, error) {
	bubble, err := BuildQuoteBubble(QuoteParams{
		AuthorID:   m.AuthorID,
		AuthorName: m.AuthorName,
		// AuthorColor:    m.AuthorColor,
		Reply:          m.Reply,
		Segments:       m.Segments,
		BubbleColor:    m.BubbleColor,
		MaxBubbleWidth: maxBubbleWidth,
		Media:          m.Media,
	}, s)
	if err != nil {
		return nil, err
	}

	var children []*Node

	switch {
	case m.AvatarImg != nil:
		avatarRounded := roundImage(resizeImage(m.AvatarImg, int(s.AvatarSize), int(s.AvatarSize)), s.AvatarSize/2)
		children = append(children, Leaf(avatarRounded.Image(), LeafOpts{W: s.AvatarSize, H: s.AvatarSize}))
	case m.AuthorName != "":
		// c1, c2 := avatarGradientColors(ColorForUserID(m.AuthorID))
		c1, c2 := AvatarColorFromID(m.AuthorID)
		fallback, err := avatarImageLetters(initials(m.AuthorName), c1, c2, int(s.AvatarSize))
		if err != nil {
			return nil, err
		}
		rounded := roundImage(fallback.Image(), s.AvatarSize/2)
		children = append(children, Leaf(rounded.Image(), LeafOpts{W: s.AvatarSize, H: s.AvatarSize}))
	default:
		spacerImg := image.NewRGBA(image.Rect(0, 0, 1, 1))
		children = append(children, Leaf(spacerImg, LeafOpts{W: s.AvatarSize, H: 1}))
	}

	children = append(children, bubble)
	return Box(BoxOpts{Dir: "row", Gap: s.AvatarGap, Children: children}), nil
}

func LoadAvatarPNG(path string) (image.Image, error) {
	img, err := gg.LoadImage(path)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func SavePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
