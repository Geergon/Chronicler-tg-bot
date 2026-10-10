use skia_safe::{Canvas, Rect, surfaces};

use crate::types::{Align, Dir, Justify, LeafOpts, Node, NodeKind};

pub fn measure(n: &mut Node) {
    let Node {
        kind,
        w: node_w,
        h: node_h,
        ..
    } = n;
    let NodeKind::Box {
        dir,
        gap,
        pad,
        min_w,
        max_w,
        children,
        ..
    }: &mut NodeKind = kind
    else {
        return;
    };

    for c in children.iter_mut() {
        measure(c);
    }

    let gaps = if children.len() > 1 {
        *gap * (children.len() - 1) as f32
    } else {
        0.0
    };

    let (w, h) = match dir {
        Dir::Col => {
            let mut outer_w = 0.0_f32;
            let mut h = 0.0_f32;
            for c in children.iter() {
                let mut cw = c.w;
                if !c.bleed() {
                    cw += pad.l + pad.r;
                }
                outer_w = outer_w.max(cw);
                h += c.h;
            }
            (outer_w, h + gaps)
        }
        Dir::Row => {
            let mut h = 0.0_f32;
            let mut w = 0.0_f32;
            for c in children.iter() {
                h = h.max(c.h);
                w += c.w;
            }
            (w + gaps + pad.l + pad.r, h)
        }
    };

    let mut w = w.ceil();
    let h = (h + pad.t + pad.b).ceil();

    if w < *min_w {
        w = *min_w;
    }
    if *max_w > 0.0 && w > *max_w {
        w = *max_w;
    }

    *node_w = w;
    *node_h = h;
}

pub fn place(n: &mut Node, x: f32, y: f32, stretch_w: f32) {
    if stretch_w > 0.0 && n.stretch() {
        n.w = stretch_w;
    }
    n.x = x.round();
    n.y = y.round();
    let (x, y) = (n.x, n.y);

    let Node {
        kind, w: nw, h: nh, ..
    } = n;
    let NodeKind::Box {
        dir,
        gap,
        pad,
        align,
        justify,
        children,
        ..
    } = kind
    else {
        return;
    };
    let (nw, nh, dir, gap, pad, align, justify) = (*nw, *nh, *dir, *gap, *pad, *align, *justify);

    let inner_w = nw - pad.l - pad.r;

    match dir {
        Dir::Col => {
            let mut cy = y + pad.t;
            for c in children.iter_mut() {
                let mut cx = x + pad.l;
                if c.bleed() {
                    cx = x + ((nw - c.w) / 2.0).max(0.0);
                } else if align == Align::Center && c.w < inner_w {
                    cx += (inner_w - c.w) / 2.0;
                }
                place(c, cx, cy, inner_w);
                cy += c.h + gap;
            }
        }
        Dir::Row => {
            let cross_y = |c: &Node| -> f32 {
                if align == Align::Center {
                    y + pad.t + (nh - pad.t - pad.b - c.h) / 2.0
                } else {
                    y + pad.t
                }
            };

            if justify == Justify::Between && children.len() == 2 {
                let (left, right) = children.split_at_mut(1);
                let (a, b) = (&mut left[0], &mut right[0]);

                let avail_a = inner_w - b.w - gap;
                if a.w > avail_a {
                    a.w = avail_a.max(0.0);
                }

                let ay = cross_y(a);
                place(a, x + pad.l, ay, 0.0);

                let by = cross_y(b);
                let bx = x + nw - pad.r - b.w;
                place(b, bx, by, 0.0);
                return;
            }

            let mut cx = x + pad.l;
            for c in children.iter_mut() {
                let cy = cross_y(c);
                place(c, cx, cy, 0.0);
                cx += c.w + gap;
            }
        }
    }
}

pub fn render(canvas: &Canvas, n: &Node) {
    if let Some(bg) = n.bg() {
        bg(canvas, n);
    }

    match &n.kind {
        NodeKind::Leaf {
            img, src_h, paint, ..
        } => {
            if let Some(paint_fn) = paint {
                paint_fn(canvas, n);
            } else {
                let src_w = img.width() as f32;
                if src_w > n.w + 1.0 {
                    // overflow: просто обрізаємо (як crop у Go)
                    let cw = (n.w as i32).clamp(1, img.width());
                    let ch = (*src_h as i32).clamp(1, img.height());

                    canvas.save();
                    canvas.clip_rect(Rect::from_xywh(n.x, n.y, cw as f32, ch as f32), None, false);
                    canvas.draw_image(img, (n.x, n.y), None);
                    canvas.restore();
                } else {
                    canvas.draw_image(img, (n.x, n.y), None);
                }
            }
        }
        NodeKind::Box { children, .. } => {
            for c in children {
                render(canvas, c);
            }
        }
    }

    if let Some(fg) = n.fg() {
        fg(canvas, n);
    }
}

pub fn spacer(h: f32) -> Option<Node> {
    let mut surface = surfaces::raster_n32_premul((1, 1))?;
    Node::leaf(
        Some(surface.image_snapshot()),
        LeafOpts {
            w: 0.0,
            h,
            ..Default::default()
        },
    )
}
