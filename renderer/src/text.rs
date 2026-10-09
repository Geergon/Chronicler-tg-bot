use crate::{fonts::*, types::TextSegment};
use skia_safe::{
    font_style::{Slant, Weight, Width},
    surfaces,
    textlayout::{
        FontCollection, ParagraphBuilder, ParagraphStyle, TextAlign, TextDecoration, TextStyle,
    },
    Color, FontStyle, Image, Paint,
};

pub const MONO_FAMILIES: &[&str] = &[NOTO_MONO, NOTO_CJK, NOTO_CUNEIFORM, NOTO_EMOJI];

fn make_text_style(seg: &TextSegment, font_size: f32) -> TextStyle {
    let color = seg.color.unwrap_or(Color::WHITE);

    let mut ts = TextStyle::new();
    ts.set_font_size(font_size);
    ts.set_font_families(if seg.mono {
        MONO_FAMILIES
    } else {
        TEXT_FAMILIES
    });
    ts.set_font_style(FontStyle::new(
        if seg.bold {
            Weight::BOLD
        } else {
            Weight::NORMAL
        },
        Width::NORMAL,
        if seg.italic {
            Slant::Italic
        } else {
            Slant::Upright
        },
    ));

    if seg.spoiler {
        let mut bg = Paint::default();
        bg.set_color(color.with_a(220));
        ts.set_background_paint(&bg);
        ts.set_color(Color::TRANSPARENT);
    } else {
        ts.set_color(color);
    }

    let mut deco = TextDecoration::empty();
    if seg.underline {
        deco |= TextDecoration::UNDERLINE;
    }
    if seg.strikethrough {
        deco |= TextDecoration::LINE_THROUGH;
    }
    if !deco.is_empty() {
        ts.set_decoration_type(deco);
        ts.set_decoration_color(color);
        ts.set_decoration_thickness_multiplier(1.0);
    }

    ts
}

fn shrink_wrap(paragraph: &mut skia_safe::textlayout::Paragraph, max_width: f32) {
    paragraph.layout(max_width);
    let target_lines = paragraph.line_number();
    if target_lines <= 1 {
        return;
    }

    let initial_width = paragraph.longest_line();
    let mut lo = initial_width / target_lines as f32 * 0.7;
    let mut hi = initial_width;

    while hi - lo > 2.0 {
        let mid = (lo + hi) / 2.0;
        paragraph.layout(mid);
        if paragraph.line_number() <= target_lines {
            hi = mid;
        } else {
            lo = mid;
        }
    }
    paragraph.layout(hi.ceil());
}

pub fn render_rich_text(
    fonts: &FontCollection,
    segments: &[TextSegment],
    max_width: f32,
    font_size: f32,
) -> (Image, f32, f32) {
    let empty = || {
        let mut s = surfaces::raster_n32_premul((1, 1)).expect("surface");
        (s.image_snapshot(), 0.0, 0.0)
    };

    if segments.iter().all(|s| s.text.is_empty()) {
        return empty();
    }

    let mut para_style = ParagraphStyle::new();
    para_style.set_text_align(TextAlign::Left);

    let mut builder = ParagraphBuilder::new(&para_style, fonts.clone());
    for seg in segments.iter().filter(|s| !s.text.is_empty()) {
        builder.push_style(&make_text_style(seg, font_size));
        builder.add_text(&seg.text);
        builder.pop();
    }
    let mut paragraph = builder.build();

    shrink_wrap(&mut paragraph, max_width);

    let (w, h) = (paragraph.longest_line(), paragraph.height());
    if w <= 0.0 || h <= 0.0 {
        return empty();
    }

    let mut surface =
        surfaces::raster_n32_premul((w.ceil() as i32, h.ceil() as i32)).expect("surface");
    paragraph.paint(surface.canvas(), (0.0, 0.0));

    (surface.image_snapshot(), w, h)
}
