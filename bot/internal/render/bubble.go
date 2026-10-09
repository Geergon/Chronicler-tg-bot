package render

import (
	"image/color"

	"git.sr.ht/~sbinet/gg"
)

func drawGradientRoundRect(colorOne, colorTwo color.Color, w, h int, r float64) *gg.Context {
	dc := gg.NewContext(w, h)

	fw, fh := float64(w), float64(h)
	grad := gg.NewLinearGradient(0, 0, fw, fh)
	grad.AddColorStop(0, colorOne)
	grad.AddColorStop(1, colorTwo)

	dc.SetFillStyle(grad)

	if fw < 2*r {
		r = fw / 2
	}
	if fh < 2*r {
		r = fh / 2
	}

	dc.DrawRoundedRectangle(0, 0, fw, fh, r)

	dc.Fill()
	return dc
}

func drawRoundRect(fillColor color.Color, w, h int, r float64) *gg.Context {
	dc := gg.NewContext(w, h)

	dc.SetColor(fillColor)

	fw, fh := float64(w), float64(h)
	if fw < 2*r {
		r = fw / 2
	}
	if fh < 2*r {
		r = fh / 2
	}

	dc.DrawRoundedRectangle(0, 0, fw, fh, r)

	dc.Fill()
	return dc
}
