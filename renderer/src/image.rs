use skia_safe::{ClipOp, CubicResampler, Image, RRect, Rect, SamplingOptions, Surface, surfaces};

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

#[cfg(test)]
mod tests {
    use skia_safe::Color;

    use crate::avatar::avatar_image_letters;
    use crate::image::{finalize_size, resize_image, round_image};
    use crate::media::single_media_leaf;
    use crate::render::STICKER_MAX_SIDE;
    use crate::spoiler::apply_spoiler;
    use crate::tests_helper::{has_color, has_white, pixel, pixels, solid, text_seg, with_fonts};
    use crate::text::render_rich_text;
    use crate::types::{MediaItem, Sizes};

    #[test]
    fn resize_keeps_color() {
        let out = resize_image(&solid(200, 200, Color::RED), 50, 50);
        assert_eq!((out.width(), out.height()), (50, 50));
        let c = pixel(&out, 25, 25);
        assert!(c.r() > 240 && c.g() < 15 && c.a() == 255, "receive {c:?}");
    }

    #[test]
    fn round_image_makes_corners_transparent() {
        let mut s = round_image(&solid(100, 100, Color::BLUE), 30.0);
        let img = s.image_snapshot();
        assert_eq!(pixel(&img, 0, 0).a(), 0, "the corner should be transparent");
        assert_eq!(pixel(&img, 50, 50), Color::BLUE);
    }

    #[test]
    fn round_image_radius_is_clamped_to_circle() {
        let mut s = round_image(&solid(40, 40, Color::BLUE), 9999.0);
        let img = s.image_snapshot();
        assert_eq!(pixel(&img, 0, 0).a(), 0);
        assert_eq!(pixel(&img, 20, 20).a(), 255);
    }

    #[test]
    fn tall_photo_is_limited_by_max_height_ratio() {
        let item = MediaItem {
            image: solid(100, 1000, Color::GREEN),
            spoiler: false,
            video: false,
            round: false,
        };
        let node = single_media_leaf(
            &item,
            200.0,
            &Sizes {
                scale: 1.0,
                ..Default::default()
            },
        );
        assert!(
            node.h <= 200.0 * 1.3 + 0.5,
            "height {} exceeded limit",
            node.h
        );
    }

    #[test]
    fn finalize_size_caps_longest_side() {
        let out = finalize_size(solid(1024, 300, Color::WHITE));
        assert_eq!(out.width(), STICKER_MAX_SIDE as i32);
        assert!(out.height() <= STICKER_MAX_SIDE as i32);
    }

    #[test]
    fn finalize_size_leaves_one_pixel_difference_sharp() {
        let src = solid(513, 150, Color::RED);
        let out = finalize_size(src);
        assert_eq!(out.width(), 512);
        assert_eq!(pixel(&out, 100, 70), Color::RED);
    }

    #[test]
    fn color_emoji_renders_in_color() {
        let (img, w, _) = with_fonts(|f| render_rich_text(f, &[text_seg("👋")], 400.0, 48.0, None));
        assert!(w > 0.0);
        assert!(has_color(&img), "emoji not colored (CBDT font?)");
    }

    #[test]
    fn two_letter_initials_are_not_truncated() {
        let img = with_fonts(|f| {
            avatar_image_letters(
                f,
                "JT",
                Color::from_rgb(200, 80, 60),
                Color::from_rgb(120, 40, 30),
                90,
            )
        })
        .unwrap();
        assert!(has_white(&img), "initials are not visible");
    }

    #[test]
    fn empty_text_has_zero_size() {
        let (_, w, h) = with_fonts(|f| render_rich_text(f, &[text_seg("")], 400.0, 24.0, None));
        assert_eq!((w, h), (0.0, 0.0));
    }

    #[test]
    fn narrow_width_wraps_into_more_lines() {
        let (_, _, h1) = with_fonts(|f| {
            render_rich_text(f, &[text_seg("word word word word")], 2000.0, 24.0, None)
        });
        let (_, _, h2) = with_fonts(|f| {
            render_rich_text(f, &[text_seg("word word word word")], 80.0, 24.0, None)
        });
        assert!(h2 > h1 * 1.5);
    }

    #[test]
    fn max_lines_limits_height() {
        let long = "word".repeat(200);
        let (_, _, h_all) =
            with_fonts(|f| render_rich_text(f, &[text_seg(&long)], 200.0, 24.0, None));
        let (_, _, h_two) =
            with_fonts(|f| render_rich_text(f, &[text_seg(&long)], 200.0, 24.0, Some(2)));
        assert!(h_two < h_all / 2.0);
    }

    #[test]
    fn spoiler_is_deterministic_and_changes_image() {
        let src = solid(200, 120, Color::from_rgb(30, 120, 220));
        let (a, b) = (apply_spoiler(&src, 2.0), apply_spoiler(&src, 2.0));
        assert_eq!(pixels(&a), pixels(&b), "'dust' must be deterministic");
        assert_ne!(pixels(&a), pixels(&src));
        assert_eq!((a.width(), a.height()), (200, 120));
    }
}
