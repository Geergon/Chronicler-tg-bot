use skia_safe::Color;
use skia_safe::Image;

use skia_safe::Paint;
use skia_safe::PathBuilder;
use skia_safe::surfaces;

use crate::image::finalize_size;
use crate::image::resize_image;
use crate::image::round_image;
use crate::spoiler::apply_spoiler;
use crate::types::BoxOpts;
use crate::types::ChatMessage;
use crate::types::LeafOpts;
use crate::types::MediaItem;
use crate::types::Node;
use crate::types::Sizes;

fn crop_to_aspect(img: &Image, target_ratio: f32) -> Image {
    let (w, h) = (img.width(), img.height());
    let current = w as f32 / h as f32;

    let (crop_w, crop_h) = if current > target_ratio {
        let crop_h = h;
        let crop_w = (crop_h as f32 * target_ratio).round() as i32;
        (crop_w, crop_h)
    } else {
        let crop_w = w;
        let crop_h = (crop_w as f32 / target_ratio).round() as i32;
        (crop_w, crop_h)
    };

    let crop_w = crop_w.clamp(1, w);
    let crop_h = crop_h.clamp(1, h);

    let off_x = (w - crop_w) / 2;
    let off_y = (h - crop_h) / 2;

    let mut surface = surfaces::raster_n32_premul((crop_w, crop_h)).expect("surface");

    surface
        .canvas()
        .draw_image(img, (-off_x as f32, -off_y as f32), None);

    surface.image_snapshot()
}

fn crop_to_square(img: &Image) -> Image {
    crop_to_aspect(img, 1.0)
}

pub fn single_media_leaf(media: &MediaItem, max_w: f32, s: &Sizes) -> Node {
    if media.round {
        return round_media_leaf(media, max_w, s);
    }
    let max_height_ratio = s.media_max_height_ratio;
    let original = &media.image;
    let (src_w, src_h) = (original.width() as f32, original.height() as f32);
    let aspect = src_w / src_h;

    let w = max_w;
    let mut h = w / aspect;
    let max_h = max_w * max_height_ratio;

    let source = if h > max_h {
        h = max_h;
        crop_to_aspect(original, w / h)
    } else {
        original.clone()
    };

    let resized = resize_image(&source, w.round() as i32, h.round() as i32);
    let resized = if media.spoiler {
        apply_spoiler(&resized, s.scale)
    } else if media.video {
        with_play_badge(&resized)
    } else {
        resized
    };
    let mut rounded = round_image(&resized, s.media_radius);

    Node::leaf(
        Some(rounded.image_snapshot()),
        LeafOpts {
            w,
            h,
            ..Default::default()
        },
    )
    .unwrap()
}

fn media_group_row(images: &[MediaItem], max_w: f32, s: &Sizes) -> Node {
    let gap = s.media_group_gap;
    let n = images.len() as f32;
    let thumb_size = (max_w - gap * (n - 1.0)) / n;
    let side = (thumb_size.round() as i32).max(1);

    let children: Vec<Option<Node>> = images
        .iter()
        .map(|img| {
            let square = crop_to_square(&img.image);
            let resized = resize_image(&square, side, side);
            let resized = if img.spoiler {
                apply_spoiler(&resized, s.scale)
            } else if img.video {
                with_play_badge(&resized)
            } else {
                resized
            };
            let radius = if img.round {
                side as f32 / 2.0
            } else {
                s.media_radius
            };
            let mut rounded = round_image(&resized, radius);
            Node::leaf(
                Some(rounded.image_snapshot()),
                LeafOpts {
                    w: thumb_size,
                    h: thumb_size,
                    ..Default::default()
                },
            )
        })
        .collect();

    Node::new_box(BoxOpts {
        dir: crate::types::Dir::Row,
        gap,
        children,
        ..Default::default()
    })
}

pub fn build_media_node(media: &[MediaItem], max_content_width: f32, s: &Sizes) -> Node {
    if media.len() == 1 {
        return single_media_leaf(&media[0], max_content_width, s);
    }
    media_group_row(media, max_content_width, s)
}

pub fn is_standalone_media(messages: &[ChatMessage]) -> bool {
    if messages.len() != 1 {
        return false;
    }
    let m = &messages[0];
    let has_text = m.segments.iter().any(|seg| !seg.text.is_empty());

    m.media.len() == 1 && !has_text && m.reply.is_none()
}

pub fn build_standalone_media(m: &MediaItem) -> Image {
    let img = finalize_size(m.image.clone());
    let img = if m.spoiler {
        apply_spoiler(&img, 2.0)
    } else if m.video {
        with_play_badge(&img)
    } else {
        img
    };
    if m.round {
        let mut s = round_image(&img, img.width() as f32 / 2.0);
        s.image_snapshot()
    } else {
        img
    }
}

pub fn build_standalone_media_node(media: &[MediaItem], max_width: f32, s: &Sizes) -> Node {
    if media.len() == 1 {
        return single_media_leaf(&media[0], max_width, s);
    }
    media_group_row(media, max_width, s)
}

// video play icon
pub fn with_play_badge(img: &Image) -> Image {
    let (w, h) = (img.width(), img.height());
    let mut surface = surfaces::raster_n32_premul((w, h)).expect("surface");
    let canvas = surface.canvas();
    canvas.draw_image(img, (0, 0), None);

    let (cx, cy) = (w as f32 / 2.0, h as f32 / 2.0);
    let r = (w.min(h) as f32 * 0.14).max(10.0);

    // circle
    let mut p = Paint::default();
    p.set_anti_alias(true);
    p.set_color(Color::from_argb(165, 0, 0, 0));
    canvas.draw_circle((cx, cy), r, &p);

    // triangle
    let mut tri = PathBuilder::new();
    tri.move_to((cx - r * 0.28, cy - r * 0.42));
    tri.line_to((cx - r * 0.28, cy + r * 0.42));
    tri.line_to((cx + r * 0.46, cy));
    tri.close();
    let path = tri.snapshot();

    p.set_color(Color::from_argb(165, 255, 255, 255));
    canvas.draw_path(&path, &p);

    surface.image_snapshot()
}

fn round_media_leaf(media: &MediaItem, max_w: f32, s: &Sizes) -> Node {
    let side = ((max_w * 0.75).round() as i32).max(1);

    let square = crop_to_square(&media.image);
    let resized = resize_image(&square, side, side);
    let resized = if media.spoiler {
        apply_spoiler(&resized, s.scale)
    } else if media.video {
        with_play_badge(&resized)
    } else {
        resized
    };

    let mut rounded = round_image(&resized, side as f32 / 2.0);

    Node::leaf(
        Some(rounded.image_snapshot()),
        LeafOpts {
            w: side as f32,
            h: side as f32,
            ..Default::default()
        },
    )
    .unwrap()
}
