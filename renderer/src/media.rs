use skia_safe::Image;

use skia_safe::surfaces;

use crate::image::finalize_size;
use crate::image::resize_image;
use crate::image::round_image;
use crate::types::BoxOpts;
use crate::types::ChatMessage;
use crate::types::LeafOpts;
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
    crop_to_aspect(&img, 1.0)
}

pub fn single_media_leaf(original: &Image, max_w: f32, max_height_ratio: f32, radius: f32) -> Node {
    let (src_w, src_h) = (original.width() as f32, original.height() as f32);
    let aspect = src_w / src_h;

    let w = max_w;
    let mut h = w / aspect;
    let max_h = max_w * max_height_ratio;

    let source = if h > max_h {
        h = max_h;
        crop_to_aspect(original, (w / h) as f32)
    } else {
        original.clone()
    };

    let resized = resize_image(&source, w.round() as i32, h.round() as i32);
    let mut rounded = round_image(&resized, radius);

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

fn media_group_row(images: &[Image], max_w: f32, gap: f32, radius: f32) -> Node {
    let n = images.len() as f32;
    let thumb_size = (max_w - gap * (n - 1.0)) / n;

    let children: Vec<Option<Node>> = images
        .iter()
        .map(|img| {
            let square = crop_to_square(img);
            let resized = resize_image(
                &square,
                thumb_size.round() as i32,
                thumb_size.round() as i32,
            );
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

pub fn build_media_node(media: &[Image], max_content_width: f32, s: &Sizes) -> Node {
    if media.len() == 1 {
        return single_media_leaf(
            &media[0],
            max_content_width,
            s.media_max_height_ratio,
            s.media_radius,
        );
    }
    media_group_row(media, max_content_width, s.media_group_gap, s.media_radius)
}

pub fn is_standalone_media(messages: &[ChatMessage]) -> bool {
    if messages.len() != 1 {
        return false;
    }
    let m = &messages[0];
    let has_text = m.segments.iter().any(|seg| !seg.text.is_empty());

    m.media.len() == 1 && !has_text && m.reply.is_none()
}

pub fn build_standalone_media(img: &Image) -> Image {
    finalize_size(img.clone())
}

pub fn build_standalone_media_node(media: &[Image], max_width: f32, s: &Sizes) -> Node {
    if media.len() == 1 {
        return single_media_leaf(
            &media[0],
            max_width,
            s.media_max_height_ratio,
            s.media_radius,
        );
    }
    media_group_row(&media, max_width, s.media_group_gap, s.media_radius)
}
