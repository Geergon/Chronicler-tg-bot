package render

import (
	"image"
	"math"

	"git.sr.ht/~sbinet/gg"
	"golang.org/x/image/draw"
)

func cropToAspect(img image.Image, targetRatio float64) image.Image {
	b := img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	current := w / h

	var cropW, cropH int
	if current > targetRatio {
		cropH = b.Dy()
		cropW = int(math.Round(float64(cropH) * targetRatio))
	} else {
		cropW = b.Dx()
		cropH = int(math.Round(float64(cropW) / targetRatio))
	}

	offX := (b.Dx() - cropW) / 2
	offY := (b.Dy() - cropH) / 2
	srcMin := image.Pt(b.Min.X+offX, b.Min.Y+offY)

	dst := image.NewRGBA(image.Rect(0, 0, cropW, cropH))
	draw.Draw(dst, dst.Bounds(), img, srcMin, draw.Src)
	return dst
}

func cropToSquare(img image.Image) image.Image {
	return cropToAspect(img, 1.0)
}

func singleMediaLeaf(original image.Image, maxW, maxHeightRatio, radius float64) *Node {
	b := original.Bounds()
	srcW, srcH := float64(b.Dx()), float64(b.Dy())
	aspect := srcW / srcH

	w := maxW
	h := w / aspect

	maxH := maxW * maxHeightRatio
	if h > maxH {
		h = maxH
		// перекадровуємо оригінал під нову пропорцію, щоб не "стиснути" зображення
		original = cropToAspect(original, w/h)
	}

	resized := resizeImage(original, int(math.Round(w)), int(math.Round(h)))
	rounded := roundImage(resized, radius)
	return Leaf(rounded.Image(), LeafOpts{W: w, H: h})
}

func mediaGroupRow(items []image.Image, maxW, gap, radius float64) *Node {
	n := float64(len(items))
	thumbSize := (maxW - gap*(n-1)) / n

	var children []*Node
	for _, img := range items {
		square := cropToSquare(img)
		resized := resizeImage(square, int(math.Round(thumbSize)), int(math.Round(thumbSize)))
		rounded := roundImage(resized, radius)
		children = append(children, Leaf(rounded.Image(), LeafOpts{W: thumbSize, H: thumbSize}))
	}

	return Box(BoxOpts{Dir: "row", Gap: gap, Children: children})
}

func buildMediaNode(media []image.Image, maxContentWidth float64, s Sizes) *Node {
	if len(media) == 0 {
		return nil
	}
	if len(media) == 1 {
		return singleMediaLeaf(media[0], maxContentWidth, s.MediaMaxHeightRatio, s.MediaRadius)
	}
	return mediaGroupRow(media, maxContentWidth, s.MediaGroupGap, s.MediaRadius)
}

func isStandaloneMedia(messages []ChatMessage) bool {
	if len(messages) != 1 {
		return false
	}
	m := messages[0]
	hasText := false
	for _, seg := range m.Segments {
		if seg.Text != "" {
			hasText = true
			break
		}
	}
	return len(m.Media) == 1 && !hasText && m.Reply == nil
}

func buildStandaloneMedia(img image.Image) (*gg.Context, error) {
	b := img.Bounds()
	dc := gg.NewContext(b.Dx(), b.Dy())
	dc.DrawImage(img, 0, 0)
	return finalizeSize(dc), nil
}

func roundImage(img image.Image, r float64) *gg.Context {
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	dc := gg.NewContext(w, h)

	fw, fh := float64(w), float64(h)
	if fw < 2*r {
		r = fw / 2
	}
	if fh < 2*r {
		r = fh / 2
	}

	dc.DrawRoundedRectangle(0, 0, fw, fh, r)
	dc.Clip()
	dc.DrawImage(img, 0, 0)
	dc.ResetClip()

	return dc
}
