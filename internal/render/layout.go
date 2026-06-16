package render

import (
	"image"
	"math"

	"git.sr.ht/~sbinet/gg"
	"golang.org/x/image/draw"
)

type LeafOpts struct {
	W, H, MaxW float64
	Bleed      bool
	Paint      func(ctx *gg.Context, n *Node)
}

type BoxOpts struct {
	Dir, Align, Justify string
	Gap                 float64
	Pad                 Pad
	Stretch             bool
	MinW, MaxW          float64
	Bg, Fg              func(ctx *gg.Context, n *Node)
	Children            []*Node
}

func Leaf(img image.Image, opts LeafOpts) *Node {
	if img == nil {
		return nil
	}
	bounds := img.Bounds()
	w := float64(bounds.Dx())
	h := float64(bounds.Dy())

	if opts.W > 0 {
		w = opts.W
	}
	if opts.H > 0 {
		h = opts.H
	}
	if opts.MaxW > 0 && w > opts.MaxW {
		w = opts.MaxW
	}

	return &Node{
		Kind:  "leaf",
		Img:   img,
		SrcH:  h,
		W:     w,
		H:     h,
		Bleed: opts.Bleed,
		Paint: opts.Paint,
	}
}

func Box(opts BoxOpts) *Node {
	dir := opts.Dir
	if dir == "" {
		dir = "col"
	}
	align := opts.Align
	if align == "" {
		align = "start"
	}
	justify := opts.Justify
	if justify == "" {
		justify = "start"
	}

	var children []*Node
	for _, c := range opts.Children {
		if c != nil {
			children = append(children, c)
		}
	}

	return &Node{
		Kind:     "box",
		Dir:      dir,
		Gap:      opts.Gap,
		Pad:      opts.Pad,
		Align:    align,
		Justify:  justify,
		Stretch:  opts.Stretch,
		MinW:     opts.MinW,
		MaxW:     opts.MaxW,
		Bg:       opts.Bg,
		Fg:       opts.Fg,
		Children: children,
	}
}

func Measure(n *Node) *Node {
	if n.Kind == "leaf" {
		return n
	}

	for _, c := range n.Children {
		Measure(c)
	}

	var h float64
	if n.Dir == "col" {
		var outerW float64
		for _, c := range n.Children {
			cw := c.W
			if !c.Bleed {
				cw += n.Pad.L + n.Pad.R
			}
			if cw > outerW {
				outerW = cw
			}
			h += c.H
		}
		if len(n.Children) > 1 {
			h += n.Gap * float64(len(n.Children)-1)
		}
		n.W = outerW
	} else {
		var w float64
		for _, c := range n.Children {
			if c.H > h {
				h = c.H
			}
			w += c.W
		}
		if len(n.Children) > 1 {
			w += n.Gap * float64(len(n.Children)-1)
		}
		n.W = w + n.Pad.L + n.Pad.R
	}

	n.W = math.Ceil(n.W)
	n.H = math.Ceil(h + n.Pad.T + n.Pad.B)

	if n.W < n.MinW {
		n.W = n.MinW
	}
	if n.MaxW > 0 && n.W > n.MaxW {
		n.W = n.MaxW
	}

	return n
}

func Place(n *Node, x, y, stretchW float64) {
	if stretchW > 0 && n.Stretch {
		n.W = stretchW
	}
	n.X = math.Round(x)
	n.Y = math.Round(y)
	x, y = n.X, n.Y

	if n.Kind == "leaf" {
		return
	}

	innerW := n.W - n.Pad.L - n.Pad.R

	if n.Dir == "col" {
		cy := y + n.Pad.T
		for _, c := range n.Children {
			cx := x + n.Pad.L
			if c.Bleed {
				cx = x + math.Max(0, (n.W-c.W)/2)
			} else if n.Align == "center" && c.W < innerW {
				cx += (innerW - c.W) / 2
			}
			Place(c, cx, cy, innerW)
			cy += c.H + n.Gap
		}
	} else {
		crossY := func(c *Node) float64 {
			if n.Align == "center" {
				return y + n.Pad.T + (n.H-n.Pad.T-n.Pad.B-c.H)/2
			}
			return y + n.Pad.T
		}

		if n.Justify == "between" && len(n.Children) == 2 {
			a, b := n.Children[0], n.Children[1]
			availA := innerW - b.W - n.Gap
			if a.W > availA {
				a.W = math.Max(0, availA)
			}
			Place(a, x+n.Pad.L, crossY(a), 0)
			Place(b, x+n.W-n.Pad.R-b.W, crossY(b), 0)
			return
		}

		cx := x + n.Pad.L
		for _, c := range n.Children {
			Place(c, cx, crossY(c), 0)
			cx += c.W + n.Gap
		}
	}
}

func cropImage(img image.Image, w, h int) image.Image {
	bounds := img.Bounds()
	if w > bounds.Dx() {
		w = bounds.Dx()
	}
	if h > bounds.Dy() {
		h = bounds.Dy()
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), img, bounds.Min, draw.Src)
	return dst
}

func Render(ctx *gg.Context, n *Node) {
	if n.Bg != nil {
		n.Bg(ctx, n)
	}

	if n.Kind == "leaf" {
		if n.Paint != nil {
			n.Paint(ctx, n)
		} else {
			bounds := n.Img.Bounds()
			srcW := float64(bounds.Dx())
			if srcW > n.W+1 {
				// overflow — для MVP без fade, просто crop
				ctx.DrawImage(cropImage(n.Img, int(n.W), int(n.SrcH)), int(n.X), int(n.Y))
			} else {
				ctx.DrawImage(n.Img, int(n.X), int(n.Y))
			}
		}
	} else {
		for _, c := range n.Children {
			Render(ctx, c)
		}
	}

	if n.Fg != nil {
		n.Fg(ctx, n)
	}
}

func spacer(h float64) *Node {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	return Leaf(img, LeafOpts{W: 0, H: h})
}
