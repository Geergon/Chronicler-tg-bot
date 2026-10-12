use skia_safe::{
    image::CachingHint, surfaces, textlayout::FontCollection, AlphaType, Color, ColorType, Image,
    ImageInfo,
};

use crate::{
    config::UiConfig,
    fonts::init_fonts,
    render::Renderer,
    types::{LeafOpts, Node, TextSegment},
};

thread_local! {
    static FONTS: FontCollection = init_fonts("./fonts".to_string()).expect("fixture fonts");
    static RENDERER: Renderer =
        Renderer::new("./fonts".to_string(), UiConfig::default()).expect("renderer");
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

pub fn pixel(img: &Image, x: i32, y: i32) -> Color {
    let p = pixels(img);
    let i = ((y * img.width() + x) * 4) as usize;
    Color::from_argb(p[i + 3], p[i], p[i + 1], p[i + 2])
}

pub fn _is_blank(img: &Image) -> bool {
    let p = pixels(img);
    p.chunks_exact(4).all(|px| px[3] == 0) || p.chunks_exact(4).all(|px| px == &p[..4])
}

pub fn has_color(img: &Image) -> bool {
    pixels(img).chunks_exact(4).any(|px| {
        let (r, g, b) = (px[0] as i32, px[1] as i32, px[2] as i32);
        px[3] > 128 && (r - g).abs().max((g - b).abs()).max((r - b).abs()) > 60
    })
}

pub fn has_white(img: &Image) -> bool {
    pixels(img)
        .chunks_exact(4)
        .any(|px| px[3] > 200 && px[0] > 230 && px[1] > 230 && px[2] > 230)
}
