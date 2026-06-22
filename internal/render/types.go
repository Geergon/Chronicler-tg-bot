package render

import (
	"image"
	"image/color"

	"git.sr.ht/~sbinet/gg"
)

type ReplyInfo struct {
	AuthorName string
	AuthorID   int64
	Text       string
}

type QuoteParams struct {
	AuthorID       int64
	AuthorName     string
	Reply          *ReplyInfo    // nil, якщо це не reply
	Segments       []TextSegment // стилізований текст самого повідомлення
	BubbleColor    color.Color
	MaxBubbleWidth float64 // максимальна загальна ширина бульбашки
	Media          []image.Image
}

type MessageBlock struct {
	AuthorID    int64
	AuthorName  string
	AvatarImg   image.Image // nil якщо аватар не показуємо (групове повідомлення підряд)
	Reply       *ReplyInfo
	Standalone  bool
	Segments    []TextSegment
	BubbleColor color.Color
	Media       []image.Image
}

type ChatMessage struct {
	AuthorID    int64
	AuthorName  string
	AvatarImg   image.Image
	Standalone  bool
	Reply       *ReplyInfo
	Segments    []TextSegment
	BubbleColor color.Color
	Media       []image.Image
}

// layout

type Pad struct {
	T, R, B, L float64
}

type Node struct {
	Kind string // "leaf" | "box"

	// leaf fields
	Img   image.Image
	SrcY  float64
	SrcH  float64
	Bleed bool
	Paint func(ctx *gg.Context, n *Node)

	// box fields
	Dir      string // "col" | "row"
	Gap      float64
	Pad      Pad
	Align    string // "start" | "center"
	Justify  string // "start" | "between"
	Stretch  bool
	MinW     float64
	MaxW     float64
	Bg, Fg   func(ctx *gg.Context, n *Node)
	Children []*Node

	// computed
	W, H, X, Y float64
}

type Sizes struct {
	Scale float64

	PaddingX            float64
	PaddingY            float64
	NameFontSize        float64
	ReplyFontSize       float64
	TextFontSize        float64
	GapAfterName        float64
	GapAfterReply       float64
	ReplyLineWidth      float64
	ReplyLineGap        float64
	CornerRadius        float64
	AvatarSize          float64
	AvatarGap           float64
	VerticalGap         float64
	MediaRadius         float64 // скруглення кутів фото (невеликий радіус, як у quotly: 5*scale)
	MediaGroupGap       float64 // gap між фото в медіа-групі
	MediaMaxHeightRatio float64 // обмеження висоти одного фото = maxContentWidth * ratio
	GapAfterMedia       float64
}

type TextSegment struct {
	Text          string
	Bold          bool
	Italic        bool
	Mono          bool
	Color         color.Color
	Underline     bool
	Strikethrough bool
	Spoiler       bool
}

type faceDecoration struct {
	Underline     bool
	Strikethrough bool
	Spoiler       bool
	Color         color.Color
}
