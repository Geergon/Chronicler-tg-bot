use skia_safe::{Canvas, Color, Image};
use std::rc::Rc;

use crate::render::UiConfig;

// ---------- Data from message ----------

#[derive(Debug, Clone)]
pub struct ReplyInfo {
    pub author_name: String,
    pub author_id: i64,
    pub text: String,
}

#[derive(Debug, Clone, Default)]
pub struct TextSegment {
    pub text: String,
    pub bold: bool,
    pub italic: bool,
    pub mono: bool,
    pub color: Option<Color>, // None = default color
    pub underline: bool,
    pub strikethrough: bool,
    pub spoiler: bool,
}

#[derive(Debug, Clone)]
pub struct QuoteParams {
    pub author_id: i64,
    pub author_name: String,
    pub admin_tag: Option<String>,
    pub reply: Option<ReplyInfo>,
    pub segments: Vec<TextSegment>,
    pub bubble_color: Color,
    pub max_bubble_width: f32,
    pub media: Vec<Image>,
}

#[derive(Debug, Clone)]
pub struct MessageBlock {
    pub author_id: i64,
    pub author_name: String,
    pub avatar_img: Option<Image>, // None if don't show avatar (group messages from one user)
    pub admin_tag: Option<String>,
    pub reply: Option<ReplyInfo>,
    pub standalone: bool,
    pub segments: Vec<TextSegment>,
    pub bubble_color: Color,
    pub media: Vec<Image>,
}

#[derive(Debug, Clone)]
pub struct ChatMessage {
    pub author_id: i64,
    pub author_name: String,
    pub avatar_img: Option<Image>,
    pub admin_tag: Option<String>,
    pub standalone: bool,
    pub reply: Option<ReplyInfo>,
    pub segments: Vec<TextSegment>,
    pub bubble_color: Color,
    pub media: Vec<Image>,
}

// ---------- Layout ----------

#[derive(Debug, Clone, Copy, Default, PartialEq)]
pub struct Pad {
    pub t: f32,
    pub r: f32,
    pub b: f32,
    pub l: f32,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Dir {
    #[default]
    Col,
    Row,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Align {
    #[default]
    Start,
    Center,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Justify {
    #[default]
    Start,
    Between,
}

pub type NodeFn = Rc<dyn Fn(&Canvas, &Node)>;

#[derive(Clone)]
pub enum NodeKind {
    Leaf {
        img: Image,
        src_h: f32,
        bleed: bool,
        paint: Option<NodeFn>,
    },
    Box {
        dir: Dir,
        gap: f32,
        pad: Pad,
        align: Align,
        justify: Justify,
        stretch: bool,
        min_w: f32,
        max_w: f32,
        bg: Option<NodeFn>,
        fg: Option<NodeFn>,
        children: Vec<Node>,
    },
}

#[derive(Clone)]
pub struct Node {
    pub kind: NodeKind,

    // computed
    pub w: f32,
    pub h: f32,
    pub x: f32,
    pub y: f32,
}

impl Node {
    /// Returns None if the image is missing..
    pub fn leaf(img: Option<Image>, opts: LeafOpts) -> Option<Node> {
        let img = img?;

        let mut w = img.width() as f32;
        let mut h = img.height() as f32;

        if opts.w > 0.0 {
            w = opts.w;
        }
        if opts.h > 0.0 {
            h = opts.h;
        }
        if opts.max_w > 0.0 && w > opts.max_w {
            w = opts.max_w;
        }

        Some(Node {
            kind: NodeKind::Leaf {
                img,
                src_h: h,
                bleed: opts.bleed,
                paint: opts.paint,
            },
            w,
            h,
            x: 0.0,
            y: 0.0,
        })
    }

    pub fn new_box(opts: BoxOpts) -> Node {
        Node {
            kind: NodeKind::Box {
                dir: opts.dir,
                gap: opts.gap,
                pad: opts.pad,
                align: opts.align,
                justify: opts.justify,
                stretch: opts.stretch,
                min_w: opts.min_w,
                max_w: opts.max_w,
                bg: opts.bg,
                fg: opts.fg,
                children: opts.children.into_iter().flatten().collect(), // reject None
            },
            w: 0.0,
            h: 0.0,
            x: 0.0,
            y: 0.0,
        }
    }

    pub fn bleed(&self) -> bool {
        matches!(&self.kind, NodeKind::Leaf { bleed: true, .. })
    }

    pub fn stretch(&self) -> bool {
        matches!(&self.kind, NodeKind::Box { stretch: true, .. })
    }

    pub fn bg(&self) -> Option<&NodeFn> {
        match &self.kind {
            NodeKind::Box { bg, .. } => bg.as_ref(),
            _ => None,
        }
    }

    pub fn fg(&self) -> Option<&NodeFn> {
        match &self.kind {
            NodeKind::Box { fg, .. } => fg.as_ref(),
            _ => None,
        }
    }
}

#[derive(Clone, Default)]
pub struct LeafOpts {
    pub w: f32,
    pub h: f32,
    pub max_w: f32,
    pub bleed: bool,
    pub paint: Option<NodeFn>,
}

#[derive(Clone, Default)]
pub struct BoxOpts {
    pub dir: Dir,
    pub align: Align,
    pub justify: Justify,
    pub gap: f32,
    pub pad: Pad,
    pub stretch: bool,
    pub min_w: f32,
    pub max_w: f32,
    pub bg: Option<NodeFn>,
    pub fg: Option<NodeFn>,
    pub children: Vec<Option<Node>>,
}

// ---------- Sizes ----------

const PT_TO_PX: f32 = 25.4 / 72.0;

#[derive(Debug, Clone, Default)]
pub struct Sizes {
    pub scale: f32,

    pub padding_x: f32,
    pub padding_y: f32,
    pub name_font_size: f32,
    pub reply_font_size: f32,
    pub text_font_size: f32,
    pub gap_after_name: f32,
    pub gap_after_reply: f32,
    pub reply_line_width: f32,
    pub reply_line_gap: f32,
    pub corner_radius: f32,
    pub avatar_size: f32,
    pub avatar_gap: f32,
    pub vertical_gap: f32,
    pub media_radius: f32,
    pub media_group_gap: f32,
    pub media_max_height_ratio: f32,
    pub gap_after_media: f32,
    pub bottom_margin: f32,
}

impl Sizes {
    pub fn new(ui: &UiConfig, scale: f32) -> Self {
        let f = |v: i32| v as f32 * scale;
        let pt = |v: i32| v as f32 * scale * PT_TO_PX;
        Sizes {
            scale,
            padding_x: f(ui.padding_x),
            padding_y: f(ui.padding_y),
            name_font_size: pt(ui.name_font_size),
            reply_font_size: pt(ui.reply_font_size),
            text_font_size: pt(ui.text_font_size),
            gap_after_name: f(ui.gap_after_name),
            gap_after_reply: f(ui.gap_after_reply),
            reply_line_width: f(ui.reply_line_width),
            reply_line_gap: f(ui.reply_line_gap),
            corner_radius: f(ui.corner_radius),
            avatar_size: f(ui.avatar_size),
            avatar_gap: f(ui.avatar_gap),
            vertical_gap: f(ui.vertical_gap),
            media_radius: f(ui.media_radius),
            media_group_gap: f(ui.media_group_gap),
            media_max_height_ratio: 1.3,
            gap_after_media: f(ui.gap_after_media),
            bottom_margin: f(ui.bottom_margin),
        }
    }
}
