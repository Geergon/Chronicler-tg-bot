use skia_safe::Color;

const TOP_COLORS: [u32; 7] = [
    0xFFFF845E, 0xFFFEBB5B, 0xFFB694F9, 0xFF9AD164, 0xFF5BCBE3, 0xFF5CAFFA, 0xFFFF8AAC,
];
const BOTTOM_COLORS: [u32; 7] = [
    0xFFD45246, 0xFFF68136, 0xFF6C61DF, 0xFF46BA43, 0xFF359AD4, 0xFF408ACF, 0xFFD95574,
];

pub const ADMIN_TAG_FG: Color = Color::from_argb(255, 111, 203, 109);
pub const ADMIN_TAG_BG: Color = Color::from_argb(255, 66, 83, 89);
pub const OWNER_TAG_FG: Color = Color::from_argb(255, 176, 140, 240);
pub const OWNER_TAG_BG: Color = Color::from_argb(255, 77, 75, 106);
pub const MEMBER_TAG_FG: Color = Color::from_argb(150, 255, 255, 255);
pub const MEMBER_TAG_BG: Color = Color::from_argb(0, 0, 0, 0);

const NAME_COLORS: [u32; 7] = [
    0xFFCA5650, // #ca5650
    0xFFFFA500, // #ffa500
    0xFF9B66DC, // #9b66dc
    0xFF50B232, // #50b232
    0xFF379EB8, // #379eb8
    0xFF4E92CC, // #4e92cc
    0xFFCF5C95, // #cf5c95
];

fn color_index(user_id: i64) -> usize {
    (user_id.unsigned_abs() % 7) as usize
}

pub fn avatar_color_from_id(user_id: i64) -> (Color, Color) {
    let i = color_index(user_id);
    (Color::new(TOP_COLORS[i]), Color::new(BOTTOM_COLORS[i]))
}

pub fn name_color_from_id(user_id: i64) -> Color {
    Color::new(NAME_COLORS[color_index(user_id)])
}
