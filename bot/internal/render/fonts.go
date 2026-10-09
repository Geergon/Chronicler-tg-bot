package render

import (
	"fmt"

	"github.com/tdewolff/canvas"
)

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

	cuneiformFamily = canvas.NewFontFamily("noto-cuneiform")
	if err := cuneiformFamily.LoadFontFile("./fonts/NotoSansCuneiform-Regular.ttf", canvas.FontRegular); err != nil {
		return fmt.Errorf("failed to load cuneiform font: %w", err)
	}

	cjkFamily = canvas.NewFontFamily("noto-cjk")
	if err := cjkFamily.LoadFontFile("./fonts/NotoSansCJKjp-Regular.otf", canvas.FontRegular); err != nil {
		return fmt.Errorf("failed to load CJK font: %w", err)
	}

	return nil
}
