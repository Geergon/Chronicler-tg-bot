use anyhow::{Context, Result};
use skia_safe::{Canvas, Color, Image, Paint, Rect, surfaces, textlayout::FontCollection};
use std::{path::Path, rc::Rc};

use crate::{
    avatar::{avatar_image_letters, initials},
    colors::{
        ADMIN_TAG_BG, ADMIN_TAG_FG, MEMBER_TAG_BG, MEMBER_TAG_FG, OWNER_TAG_BG, OWNER_TAG_FG,
        avatar_color_from_id, name_color_from_id,
    },
    config::UiConfig,
    fonts::init_fonts,
    image::{finalize_size, resize_image, round_image},
    layout::{measure, place, render, spacer},
    media::{
        build_media_node, build_standalone_media, build_standalone_media_node, is_standalone_media,
    },
    text::render_rich_text,
    types::{
        Align, BoxOpts, ChatMessage, Dir, Justify, LeafOpts, MessageBlock, Node, NodeFn, Pad,
        QuoteParams, Sizes, Tag, TagKind, TextSegment,
    },
};

const MAX_SCALE: f32 = 3.0;
pub const STICKER_MAX_SIDE: f32 = 512.0;
const REPLY_TEXT_COLOR: skia_safe::Color = skia_safe::Color::from_argb(255, 209, 209, 209);

struct TagStyle {
    bg: Color,
    fg: Color,
}

fn tag_style(kind: TagKind) -> TagStyle {
    match kind {
        TagKind::Owner => TagStyle {
            bg: OWNER_TAG_BG,
            fg: OWNER_TAG_FG,
        },
        TagKind::Admin => TagStyle {
            bg: ADMIN_TAG_BG,
            fg: ADMIN_TAG_FG,
        },
        TagKind::Member => TagStyle {
            bg: MEMBER_TAG_BG,
            fg: MEMBER_TAG_FG,
        },
    }
}

pub struct Renderer {
    pub fonts: FontCollection,
    pub ui: UiConfig,
}

impl Renderer {
    pub fn new(fonts_dir: impl AsRef<Path>, ui: UiConfig) -> Result<Self> {
        Ok(Self {
            fonts: init_fonts(fonts_dir)?,
            ui,
        })
    }

    pub fn build_quote_bubble(&self, p: &QuoteParams, s: &Sizes) -> Node {
        let max_content_width = p.max_bubble_width - s.padding_x * 2.0;
        let mut children: Vec<Option<Node>> = Vec::new();

        if !p.author_name.is_empty() {
            let mut tag = p
                .tag
                .as_ref()
                .and_then(|t| self.build_tag_node(t, max_content_width * 0.5, s));
            if let Some(t) = tag.as_mut() {
                measure(t);
            }

            let gap = s.name_font_size * 0.5;

            let name_segs = [TextSegment {
                text: p.author_name.clone(),
                bold: true,
                color: Some(name_color_from_id(p.author_id)),
                ..Default::default()
            }];

            let (name_img, _, _) = render_rich_text(
                &self.fonts,
                &name_segs,
                max_content_width,
                s.name_font_size,
                None,
            );
            let name_leaf = Node::leaf(Some(name_img), LeafOpts::default());

            let left = match p.name_emoji.as_ref() {
                Some(emoji) => {
                    let size = (s.name_font_size * 1.15).round().max(1.0);
                    let scaled = resize_image(emoji, size as i32, size as i32);
                    let emoji_leaf = Node::leaf(
                        Some(scaled),
                        LeafOpts {
                            w: size,
                            h: size,
                            ..Default::default()
                        },
                    );
                    Some(Node::new_box(BoxOpts {
                        dir: Dir::Row,
                        align: Align::Center,
                        gap: s.name_font_size * 0.2,
                        children: vec![name_leaf, emoji_leaf],
                        ..Default::default()
                    }))
                }
                None => name_leaf,
            };

            match tag {
                Some(tag) => children.push(Some(Node::new_box(BoxOpts {
                    dir: Dir::Row,
                    align: Align::Center, // tag vertically in the middle of the line
                    justify: Justify::Between, // name on the left, tag on the right
                    stretch: true,        // line the full width of the bubble
                    gap,
                    children: vec![left, Some(tag)],
                    ..Default::default()
                }))),
                None => children.push(left),
            }
            children.push(spacer(s.gap_after_name));
        }

        if let Some(reply) = &p.reply {
            let reply_text_max_width = max_content_width - s.reply_line_width - s.reply_line_gap;
            let accent = name_color_from_id(reply.author_id);

            let mut reply_segs = vec![TextSegment {
                text: format!("{}\n", reply.author_name),
                bold: true,
                color: Some(accent),
                ..Default::default()
            }];

            if reply.segments.is_empty() {
                reply_segs.push(TextSegment {
                    text: reply.text.clone(),
                    color: Some(REPLY_TEXT_COLOR),
                    ..Default::default()
                });
            } else {
                reply_segs.extend(reply.segments.iter().cloned());
            }

            let (reply_img, _, reply_h) = render_rich_text(
                &self.fonts,
                &reply_segs,
                reply_text_max_width,
                s.reply_font_size,
                Some(2),
            );

            let line_img = draw_reply_line(s.reply_line_width, reply_h, accent);

            children.push(Some(Node::new_box(BoxOpts {
                dir: Dir::Row,
                gap: s.reply_line_gap,
                children: vec![
                    Node::leaf(Some(line_img), LeafOpts::default()),
                    Node::leaf(Some(reply_img), LeafOpts::default()),
                ],
                ..Default::default()
            })));
            children.push(spacer(s.gap_after_reply));
        }

        let (text_img, text_w, _) = render_rich_text(
            &self.fonts,
            &p.segments,
            max_content_width,
            s.text_font_size,
            None,
        );
        let has_text = text_w > 0.0;

        if !p.media.is_empty() {
            children.push(Some(build_media_node(&p.media, max_content_width, s)));
            if has_text {
                children.push(spacer(s.gap_after_media));
            }
        }

        if has_text {
            children.push(Node::leaf(Some(text_img), LeafOpts::default()));
        }

        let bubble_color = p.bubble_color;
        let corner = s.corner_radius;
        let bg: NodeFn = Rc::new(move |canvas: &Canvas, n: &Node| {
            let r = corner.min(n.w / 2.0).min(n.h / 2.0);
            let mut paint = Paint::default();
            paint.set_anti_alias(true);
            paint.set_color(bubble_color);
            canvas.draw_round_rect(Rect::from_xywh(n.x, n.y, n.w, n.h), r, r, &paint);
        });

        Node::new_box(BoxOpts {
            dir: Dir::Col,
            pad: Pad {
                t: s.padding_y,
                r: s.padding_x,
                b: s.padding_y,
                l: s.padding_x,
            },
            max_w: p.max_bubble_width,
            bg: Some(bg),
            children,
            ..Default::default()
        })
    }

    pub fn build_message_row(
        &self,
        m: &MessageBlock,
        max_bubble_width: f32,
        s: &Sizes,
    ) -> Result<Node> {
        // if message contain only media without text, it will render message as pictures without avatar, bubble and nickname
        // division by 1.5 reduce the size of pictures. The value is approximate and not exact
        if m.standalone && !m.media.is_empty() {
            return Ok(build_standalone_media_node(
                &m.media,
                max_bubble_width / 1.3,
                s,
            ));
        }

        let bubble = self.build_quote_bubble(
            &QuoteParams {
                author_id: m.author_id,
                author_name: m.author_name.clone(),
                tag: m.tag.clone(),
                reply: m.reply.clone(),
                segments: m.segments.clone(),
                bubble_color: m.bubble_color,
                max_bubble_width,
                media: m.media.clone(),
                name_emoji: m.name_emoji.clone(),
            },
            s,
        );

        let size = s.avatar_size;
        let avatar_opts = || LeafOpts {
            w: size,
            h: size,
            ..Default::default()
        };

        let avatar = if let Some(av) = &m.avatar_img {
            let resized = resize_image(av, size as i32, size as i32);
            let mut rounded = round_image(&resized, size / 2.0);
            Node::leaf(Some(rounded.image_snapshot()), avatar_opts())
        } else if !m.author_name.is_empty() {
            let (c1, c2) = avatar_color_from_id(m.author_id);
            let fallback =
                avatar_image_letters(&self.fonts, &initials(&m.author_name), c1, c2, size as i32)?;
            let mut rounded = round_image(&fallback, size / 2.0);
            Node::leaf(Some(rounded.image_snapshot()), avatar_opts())
        } else {
            let mut s1 = surfaces::raster_n32_premul((1, 1)).expect("surface");
            Node::leaf(
                Some(s1.image_snapshot()),
                LeafOpts {
                    w: size,
                    h: 1.0,
                    ..Default::default()
                },
            )
        };

        Ok(Node::new_box(BoxOpts {
            dir: Dir::Row,
            gap: s.avatar_gap,
            children: vec![avatar, Some(bubble)],
            ..Default::default()
        }))
    }

    fn build_root(&self, messages: &[ChatMessage], s: &Sizes) -> Result<Node> {
        // 80/16 = base AvatarSize/AvatarGap at scale=1
        let max_bubble_width = (STICKER_MAX_SIDE - 80.0 - 16.0) * s.scale;

        let mut rows: Vec<Option<Node>> = Vec::new();
        for (i, msg) in messages.iter().enumerate() {
            let is_first = i == 0 || messages[i - 1].author_id != msg.author_id;

            let mut block = MessageBlock {
                author_id: msg.author_id,
                author_name: String::new(),
                avatar_img: None,
                tag: None,
                reply: msg.reply.clone(),
                standalone: msg.standalone,
                segments: msg.segments.clone(),
                bubble_color: msg.bubble_color,
                media: msg.media.clone(),
                name_emoji: None,
            };
            if !msg.standalone && is_first {
                block.author_name = msg.author_name.clone();
                block.avatar_img = msg.avatar_img.clone();
                block.tag = msg.tag.clone();
                block.name_emoji = msg.name_emoji.clone();
            }

            rows.push(Some(self.build_message_row(&block, max_bubble_width, s)?));
        }

        Ok(Node::new_box(BoxOpts {
            dir: Dir::Col,
            gap: s.vertical_gap,
            pad: Pad {
                t: 0.0,
                r: 0.0,
                b: s.bottom_margin,
                l: 0.0,
            },
            children: rows,
            ..Default::default()
        }))
    }

    pub fn build_sticker_chat_stack(&self, messages: &[ChatMessage]) -> Result<Image> {
        if is_standalone_media(messages) {
            return Ok(build_standalone_media(&messages[0].media[0]));
        }

        // Pass 1: natural dimensions at scale = 1
        let mut root1 = self.build_root(messages, &Sizes::new(&self.ui, 1.0))?;
        measure(&mut root1);

        let max_side = root1.w.max(root1.h).max(1.0);
        // let scale = (STICKER_MAX_SIDE / max_side).min(MAX_SCALE);
        let scale = (STICKER_MAX_SIDE / max_side).min(MAX_SCALE) * 0.995;

        // Pass 2: Rebuild to the desired scale
        let mut root2 = self.build_root(messages, &Sizes::new(&self.ui, scale))?;
        measure(&mut root2);
        place(&mut root2, 0.0, 0.0, 0.0);

        let mut surface =
            surfaces::raster_n32_premul(((root2.w as i32).max(1), (root2.h as i32).max(1)))
                .context("failed to create surface")?;
        render(surface.canvas(), &root2);

        Ok(finalize_size(surface.image_snapshot()))
    }

    fn build_tag_node(&self, tag: &Tag, max_w: f32, s: &Sizes) -> Option<Node> {
        let style = tag_style(tag.kind);

        let seg = [TextSegment {
            text: tag.text.clone(),
            color: Some(style.fg),
            ..Default::default()
        }];
        let (img, _, _) = render_rich_text(&self.fonts, &seg, max_w, s.name_font_size * 0.8, None);
        let label = Node::leaf(Some(img), LeafOpts::default())?;

        let (px, py) = (6.0 * s.scale, 2.0 * s.scale);
        let bg: NodeFn = Rc::new(move |canvas: &Canvas, n: &Node| {
            let mut paint = Paint::default();
            paint.set_anti_alias(true);
            paint.set_color(style.bg);
            let r = n.h / 2.0;
            canvas.draw_round_rect(Rect::from_xywh(n.x, n.y, n.w, n.h), r, r, &paint);
        });

        Some(Node::new_box(BoxOpts {
            dir: Dir::Col,
            pad: Pad {
                t: py,
                r: px,
                b: py,
                l: px,
            },
            bg: Some(bg),
            children: vec![Some(label)],
            ..Default::default()
        }))
    }
}

fn draw_reply_line(line_width: f32, h: f32, col: Color) -> Image {
    let mut surface = surfaces::raster_n32_premul((20, (h as i32).max(1))).expect("surface");

    let mut paint = Paint::default();
    paint.set_anti_alias(true);
    paint.set_color(col);
    paint.set_style(skia_safe::PaintStyle::Stroke);
    paint.set_stroke_width(line_width);

    surface.canvas().draw_line((1.0, 0.0), (1.0, h), &paint);
    surface.image_snapshot()
}
