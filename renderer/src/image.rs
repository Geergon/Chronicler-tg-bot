use skia_safe::{
    surfaces, ClipOp, CubicResampler, FilterMode, Image, MipmapMode, Paint, RRect, Rect,
    SamplingOptions, Surface,
};

use crate::render::STICKER_MAX_SIDE;

pub fn resize_image(src: &Image, w: i32, h: i32) -> Image {
    let (w, h) = (w.max(1), h.max(1));
    let mut surface = surfaces::raster_n32_premul((w, h)).expect("surface");
    let canvas = surface.canvas();

    canvas.scale((
        w as f32 / src.width() as f32,
        h as f32 / src.height() as f32,
    ));
    canvas.draw_image_with_sampling_options(
        src,
        (0, 0),
        SamplingOptions::from(CubicResampler::catmull_rom()),
        None,
    );

    surface.image_snapshot()
}

pub fn round_image(img: &Image, r: f32) -> Surface {
    let img_w = img.width();
    let img_h = img.height();

    let mut surface = surfaces::raster_n32_premul((img_w, img_h)).expect("surface");
    let canvas = surface.canvas();

    let fw = img_w as f32;
    let fh = img_h as f32;

    let mut r = r;
    r = r.min(fw / 2.0).min(fh / 2.0);

    let rrect = RRect::new_rect_xy(Rect::from_xywh(0.0, 0.0, fw, fh), r, r);
    canvas.save();
    canvas.clip_rrect(rrect, ClipOp::Intersect, true);
    canvas.draw_image(img, (0, 0), None);
    canvas.restore();

    surface
}

pub fn finalize_size(img: Image) -> Image {
    let (w, h) = (img.width(), img.height());

    let (target_w, target_h) = if w >= h {
        (
            STICKER_MAX_SIDE as i32,
            (h as f32 * STICKER_MAX_SIDE / w as f32).round() as i32,
        )
    } else {
        (
            (w as f32 * STICKER_MAX_SIDE / h as f32).round() as i32,
            STICKER_MAX_SIDE as i32,
        )
    };

    if target_w == w && target_h == h {
        return img;
    }

    if (w - target_w).abs() <= 2 && (h - target_h).abs() <= 2 {
        let mut surface =
            surfaces::raster_n32_premul((target_w.max(1), target_h.max(1))).expect("surface");
        surface.canvas().draw_image(&img, (0, 0), None);
        return surface.image_snapshot();
    }

    resize_image(&img, target_w.max(1), target_h.max(1))
}
