use anyhow::{Result, bail};
use skia_safe::{
    Canvas, Color, Color4f, FontStyle, Image, Paint, Point, Rect, TileMode,
    gradient::{Colors, Gradient, Interpolation},
    shaders::linear_gradient,
    surfaces,
    textlayout::{FontCollection, ParagraphBuilder, ParagraphStyle, TextAlign, TextStyle},
};
use unicode_segmentation::UnicodeSegmentation;

use crate::{fonts::TEXT_FAMILIES, image::resize_image};

pub fn initials(name: &str) -> String {
    let name = name.trim();
    if name.is_empty() {
        return "?".to_string();
    }

    let words: Vec<&str> = name.split_whitespace().collect();

    if words.len() >= 2 {
        let grapheme1 = match first_grapheme_upper(words[0]) {
            Some(g) => g,
            None => "".to_string(),
        };
        let grapheme2 = match first_grapheme_upper(words[1]) {
            Some(g) => g,
            None => "".to_string(),
        };
        return format!("{}{}", grapheme1, grapheme2);
    }

    let chars: Vec<char> = words[0].chars().collect();
    if chars.len() == 0 {
        return "?".to_string();
    }
    chars[0].to_uppercase().to_string()
}

fn first_grapheme_upper(s: &str) -> Option<String> {
    s.graphemes(true).next().map(|g| g.to_uppercase())
}

pub fn avatar_image_letters(
    fonts: &FontCollection,
    letter: &str,
    color1: Color,
    color2: Color,
    size: i32,
) -> Result<Image> {
    let mut surface = surfaces::raster_n32_premul((size, size)).expect("failed to create surface");

    let canvas = surface.canvas();

    draw_gradient_square(canvas, size as f32, color1, color2);

    let letter_count = letter.chars().count();

    // Target text size relative to avatar
    let target_ratio: f32;
    if letter_count > 1 {
        target_ratio = 0.5; // the text takes 50% of the avatar height
    } else {
        target_ratio = 0.6; // 60% for one letter
    }
    let target_h = size as f32 * target_ratio;

    // Render with a large fontSize to avoid losing quality when scaling
    let font_size = size as f32 * 0.8;
    let (image, text_w, text_h) = render_avatar_text(fonts, letter, font_size);

    if text_w <= 0.0 || text_h <= 0.0 {
        bail!("width or height lower than 0")
    }

    // Scale to targetH while preserving proportions
    let scale = target_h / text_h;
    let new_w = (text_w * scale).round();
    let new_h = (text_h * scale).round();

    let scaled_img = resize_image(&image, new_w as i32, new_h as i32);

    let x = (size - new_w as i32) / 2;
    let y = (size - new_h as i32) / 2;

    canvas.draw_image(scaled_img, (x, y), None);

    Ok(surface.image_snapshot())
}

fn render_avatar_text(fonts: &FontCollection, letters: &str, font_size: f32) -> (Image, f32, f32) {
    let mut para_style = ParagraphStyle::new();
    para_style.set_text_align(TextAlign::Left);

    let mut ts = TextStyle::new();
    ts.set_font_size(font_size);
    ts.set_color(Color::WHITE);
    ts.set_font_style(FontStyle::bold());
    ts.set_font_families(&TEXT_FAMILIES);

    let mut builder = ParagraphBuilder::new(&para_style, fonts.clone());
    builder.push_style(&ts);
    builder.add_text(letters);
    let mut paragraph = builder.build();

    paragraph.layout(10_000.0);

    // Actual text width (not max_size) and height
    let (text_w, text_h) = (paragraph.longest_line(), paragraph.height());

    if text_w <= 0.0 || text_h <= 0.0 {
        let mut s = surfaces::raster_n32_premul((1, 1)).expect("surface");
        return (s.image_snapshot(), 0.0, 0.0);
    }

    let (iw, ih) = (text_w.ceil() as i32, text_h.ceil() as i32);
    let mut surface = surfaces::raster_n32_premul((iw, ih)).expect("surface");

    paragraph.paint(surface.canvas(), (0.0, 0.0));
    (surface.image_snapshot(), text_w, text_h)
}

fn draw_gradient_square(canvas: &Canvas, size: f32, color1: Color, color2: Color) {
    let stops = [Color4f::from(color1), Color4f::from(color2)];

    let gradient = Gradient::new(
        Colors::new_evenly_spaced(&stops, TileMode::Clamp, None),
        Interpolation::default(),
    );

    let shader = linear_gradient(
        (Point::new(0.0, 0.0), Point::new(size, size)),
        &gradient,
        None,
    );

    let mut paint = Paint::default();
    paint.set_anti_alias(true);
    paint.set_shader(shader);

    canvas.draw_rect(Rect::from_wh(size, size), &paint);
}
