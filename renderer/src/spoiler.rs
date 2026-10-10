use skia_safe::{image_filters, surfaces, Color, Image, Paint, Rect, TileMode};

use crate::image::resize_image;

const BLUR_SIDE: f32 = 24.0;

/// Linear congruent generator: deterministic, without dependencies.
struct Lcg(u32);

impl Lcg {
    fn next(&mut self) -> f32 {
        self.0 = self.0.wrapping_mul(1_664_525).wrapping_add(1_013_904_223);
        (self.0 >> 8) as f32 / (1u32 << 24) as f32 // [0, 1)
    }
}

pub fn apply_spoiler(img: &Image, scale: f32) -> Image {
    let (w, h) = (img.width().max(2), img.height().max(2));
    let (wf, hf) = (w as f32, h as f32);

    // 1. fill it with gray, draw a photo on top: transparency leaves no "cutouts"
    let mut flat_surface = surfaces::raster_n32_premul((w, h)).expect("surface");
    {
        let c = flat_surface.canvas();
        c.clear(Color::from_rgb(128, 128, 128));
        c.draw_image(img, (0, 0), None);
    }
    let flat = flat_surface.image_snapshot();

    // 2. Thumbnail and return to full size
    let (tw, th) = if w >= h {
        (BLUR_SIDE, BLUR_SIDE * hf / wf)
    } else {
        (BLUR_SIDE * wf / hf, BLUR_SIDE)
    };
    let tiny = resize_image(
        &flat,
        (tw.round() as i32).max(2),
        (th.round() as i32).max(2),
    );
    let upscaled = resize_image(&tiny, w, h);

    // 3. Blur. Sigma proportional to size (8 px at ~512 width)
    let sigma = wf.max(hf) * 0.0156;
    let mut out = surfaces::raster_n32_premul((w, h)).expect("surface");
    let canvas = out.canvas();

    let mut blur_paint = Paint::default();
    blur_paint.set_image_filter(image_filters::blur(
        (sigma, sigma),
        TileMode::Clamp,
        None,
        None,
    ));
    canvas.draw_image(&upscaled, (0, 0), Some(&blur_paint));

    // 4. Light dimming
    let mut dim = Paint::default();
    dim.set_color(Color::from_argb(71, 30, 34, 44)); // 0.28 * 255 ≈ 71
    canvas.draw_rect(Rect::from_wh(wf, hf), &dim);

    // 5. Dust
    let mut rnd = Lcg((w as u32).wrapping_mul(73_856_093) ^ (h as u32).wrapping_mul(19_349_663));
    let dot = (scale * wf / 640.0 * 1.1).max(1.0);
    let count = (wf * hf / (dot * dot * 22.0)).round() as i32;

    let mut p = Paint::default();
    p.set_anti_alias(true);
    for _ in 0..count {
        let x = rnd.next() * wf;
        let y = rnd.next() * hf;
        let r = dot * (0.45 + rnd.next() * 0.75);
        let alpha = 0.35 + rnd.next() * 0.5;
        p.set_color(Color::WHITE);
        p.set_alpha_f(alpha);
        canvas.draw_circle((x, y), r, &p);
    }

    out.image_snapshot()
}
