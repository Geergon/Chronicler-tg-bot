use skia_safe::{
    AlphaType, Color, ColorType, Image, ImageInfo, image::CachingHint, surfaces,
    textlayout::FontCollection,
};

use crate::{
    config::UiConfig,
    fonts::init_fonts,
    render::Renderer,
    types::{LeafOpts, Node, TextSegment},
};

thread_local! {
    static FONTS: FontCollection = init_fonts("./fonts").expect("fixture fonts");
    static RENDERER: Renderer =
        Renderer::new("./fonts", UiConfig::default()).expect("renderer");
}

pub fn with_fonts<R>(f: impl FnOnce(&FontCollection) -> R) -> R {
    FONTS.with(|x| f(x))
}
pub fn _with_renderer<R>(f: impl FnOnce(&Renderer) -> R) -> R {
    RENDERER.with(|x| f(x))
}

pub fn solid(w: i32, h: i32, c: Color) -> Image {
    let mut s = surfaces::raster_n32_premul((w, h)).unwrap();
    s.canvas().clear(c);
    s.image_snapshot()
}

pub fn leaf(w: f32, h: f32) -> Option<Node> {
    let img = solid(1, 1, Color::WHITE);
    Node::leaf(
        Some(img),
        LeafOpts {
            w,
            h,
            ..Default::default()
        },
    )
}

pub fn text_seg(text: &str) -> TextSegment {
    TextSegment {
        text: text.into(),
        color: Some(Color::WHITE),
        ..Default::default()
    }
}

pub fn pixels(img: &Image) -> Vec<u8> {
    let (w, h) = (img.width(), img.height());
    let info = ImageInfo::new((w, h), ColorType::RGBA8888, AlphaType::Unpremul, None);
    let mut buf = vec![0u8; (w * h * 4) as usize];
    assert!(
        img.read_pixels(
            &info,
            &mut buf,
            (w * 4) as usize,
            (0, 0),
            CachingHint::Allow
        ),
        "read_pixels failed"
    );
    buf
}

fn rgba(p: &[u8]) -> &[[u8; 4]] {
    p.as_chunks::<4>().0
}

pub fn pixel(img: &Image, x: i32, y: i32) -> Color {
    let p = pixels(img);
    let i = ((y * img.width() + x) * 4) as usize;
    Color::from_argb(p[i + 3], p[i], p[i + 1], p[i + 2])
}

pub fn _is_blank(img: &Image) -> bool {
    let p = pixels(img);
    let px = rgba(&p);
    px.iter().all(|c| c[3] == 0) || px.iter().all(|c| *c == px[0])
}

pub fn has_color(img: &Image) -> bool {
    let p = pixels(img);
    rgba(&p).iter().any(|&[r, g, b, a]| {
        let (r, g, b) = (r as i32, g as i32, b as i32);
        a > 128 && (r - g).abs().max((g - b).abs()).max((r - b).abs()) > 60
    })
}

pub fn has_white(img: &Image) -> bool {
    let p = pixels(img);
    rgba(&p)
        .iter()
        .any(|&[r, g, b, a]| a > 200 && r > 230 && g > 230 && b > 230)
}
