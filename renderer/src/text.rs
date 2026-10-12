use crate::{fonts::*, image::resize_image, types::TextSegment};
use skia_safe::{
    Color, FontStyle, Image,
    font_style::{Slant, Weight, Width},
    surfaces,
    textlayout::{
        FontCollection, ParagraphBuilder, ParagraphStyle, PlaceholderAlignment, PlaceholderStyle,
        TextAlign, TextBaseline, TextDecoration, TextStyle,
    },
};

pub const MONO_FAMILIES: &[&str] = &[NOTO_MONO, NOTO_CJK, NOTO_CUNEIFORM, NOTO_EMOJI];
const EMOJI_SCALE: f32 = 1.15;

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
        // let mut bg = Paint::default();
        // bg.set_color(color.with_a(220));
        // ts.set_background_paint(&bg);
        // ts.set_color(Color::TRANSPARENT);
        ts.set_color(Color::from_argb(38, 255, 255, 255));
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
    max_lines: Option<usize>,
) -> (Image, f32, f32) {
    let empty = || {
        let mut s = surfaces::raster_n32_premul((1, 1)).expect("surface");
        (s.image_snapshot(), 0.0, 0.0)
    };

    if segments.iter().all(|s| s.text.is_empty()) {
        return empty();
    }

    let emoji_size = font_size * EMOJI_SCALE;
    let mut emoji_images: Vec<Image> = Vec::new();

    let mut para_style = ParagraphStyle::new();
    para_style.set_text_align(TextAlign::Left);
    if let Some(n) = max_lines {
        para_style.set_max_lines(n);
        para_style.set_ellipsis("…");
    }

    let mut builder = ParagraphBuilder::new(&para_style, fonts.clone());
    for seg in segments.iter().filter(|s| !s.text.is_empty()) {
        match (&seg.emoji_image, seg.spoiler) {
            (Some(img), false) => {
                // custom tg emoji
                builder.add_placeholder(&PlaceholderStyle::new(
                    emoji_size,
                    emoji_size,
                    PlaceholderAlignment::Baseline,
                    TextBaseline::Alphabetic,
                    emoji_size * 0.85,
                ));
                emoji_images.push(img.clone());
            }
            _ => {
                // regular text and emoji
                builder.push_style(&make_text_style(seg, font_size));
                builder.add_text(&seg.text);
                builder.pop();
            }
        }
    }
    let mut paragraph = builder.build();

    if max_lines.is_some() {
        paragraph.layout(max_width);
    } else {
        shrink_wrap(&mut paragraph, max_width);
    }

    let (w, h) = (paragraph.longest_line(), paragraph.height());
    if w <= 0.0 || h <= 0.0 {
        return empty();
    }

    let mut surface =
        surfaces::raster_n32_premul((w.ceil() as i32, h.ceil() as i32)).expect("surface");
    paragraph.paint(surface.canvas(), (0.0, 0.0));

    // Emoji pictures on top of reserved space
    for (tb, img) in paragraph
        .get_rects_for_placeholders()
        .iter()
        .zip(&emoji_images)
    {
        let r = tb.rect;
        let rw = (r.width().round() as i32).max(1);
        let rh = (r.height().round() as i32).max(1);
        let scaled = resize_image(img, rw, rh);
        surface.canvas().draw_image(&scaled, (r.left, r.top), None);
    }

    (surface.image_snapshot(), w, h)
}
