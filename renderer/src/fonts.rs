use anyhow::{Context, Result};
use skia_safe::{
    textlayout::{FontCollection, TypefaceFontProvider},
    Data, FontMgr,
};
use std::{fs, io, path::Path};

pub const NOTO_SANS: &str = "noto-sans";
pub const NOTO_MONO: &str = "noto-mono";
pub const NOTO_CUNEIFORM: &str = "noto-cuneiform";
pub const NOTO_CJK: &str = "noto-cjk";
pub const NOTO_EMOJI: &str = "noto-emoji";

/// Order = fallback priority for each glyph
pub const TEXT_FAMILIES: &[&str] = &[NOTO_SANS, NOTO_EMOJI, NOTO_CJK, NOTO_CUNEIFORM];

pub fn init_fonts(dir: impl AsRef<Path>) -> Result<FontCollection> {
    let dir = dir.as_ref();
    let mgr = FontMgr::default();
    let mut provider = TypefaceFontProvider::new();

    let files: &[(&str, &str)] = &[
        ("NotoSans-Regular.ttf", NOTO_SANS),
        ("NotoSans-Bold.ttf", NOTO_SANS),
        ("NotoSans-Italic.ttf", NOTO_SANS),
        ("NotoSans-BoldItalic.ttf", NOTO_SANS),
        ("NotoSansMono-Regular.ttf", NOTO_MONO),
        ("NotoSansCuneiform-Regular.ttf", NOTO_CUNEIFORM),
        ("NotoSansCJKjp-Regular.otf", NOTO_CJK),
        ("NotoColorEmoji.ttf", NOTO_EMOJI),
    ];

    for (file, family) in files {
        let path = dir.join(file);
        let bytes = fs::read(&path).map_err(|e| {
            io::Error::new(e.kind(), format!("failed to read {}: {e}", path.display()))
        })?;

        let typeface = mgr
            .new_from_data(Data::new_copy(&bytes), None)
            .with_context(|| format!("failed to parse font {}", path.display()))?;

        provider.register_typeface(typeface, Some(*family));
    }

    let mut fonts = FontCollection::new();
    fonts.set_asset_font_manager(Some(provider.into()));
    fonts.set_default_font_manager(FontMgr::default(), None); // fallback on system fonts

    Ok(fonts)
}
