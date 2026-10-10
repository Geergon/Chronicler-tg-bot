mod avatar;
mod colors;
mod fonts;
mod image;
mod layout;
mod media;
mod render;
mod text;
mod types;
mod spoiler;
mod config;

use std::{
    collections::HashMap,
    panic::AssertUnwindSafe,
    sync::{Arc, mpsc},
    time::Duration,
};

use anyhow::{Context, Result, anyhow, bail};
use axum::{
    Router,
    extract::{DefaultBodyLimit, Multipart, State},
    http::{StatusCode, header},
    response::{IntoResponse, Response},
    routing::{get, post},
};
use config::UiConfig;
use serde::Deserialize;
use skia_safe::{Color, Data, EncodedImageFormat, Image};
use tokio::sync::oneshot;

use render::{Renderer};
use types::{ChatMessage, MediaItem, ReplyInfo, Tag, TagKind, TextSegment};

// ───────────────────────── Limits ─────────────────────────

const MAX_BODY_BYTES: usize = 25 * 1024 * 1024;
const MAX_MESSAGES: usize = 30;
const MAX_FILES: usize = 40;
const MAX_IMAGE_PIXELS: i64 = 50_000_000;
const QUEUE_SIZE: usize = 64;
const RENDER_TIMEOUT: Duration = Duration::from_secs(30);

// ───────────────────────── DTO ─────────────────────────

#[derive(Debug, Deserialize)]
struct RenderRequest {
    messages: Vec<MessageDto>,
}

#[derive(Debug, Deserialize, Clone, Copy)]
#[serde(rename_all = "lowercase")]
enum TagKindDto {
    Owner,
    Admin,
    #[serde(other)]
    Member,
}

#[derive(Debug, Deserialize)]
struct TagDto {
    text: String,
    kind: TagKindDto,
}

#[derive(Debug, Deserialize)]
struct MessageDto {
    author_id: i64,
    #[serde(default)]
    author_name: String,
    tag: Option<TagDto>,
    avatar: Option<String>, // multipart part file name
    #[serde(default)]
    standalone: bool,
    reply: Option<ReplyDto>,
    #[serde(default)]
    segments: Vec<SegmentDto>,
    bubble_color: String, // "#RRGGBB" or "#AARRGGBB"
    #[serde(default)]
    media: Vec<MediaDto>, // multipart-part name
}

#[derive(Debug, Deserialize)]
struct MediaDto{
    part: String,
#[serde(default)]
    spoiler: bool,
}

#[derive(Debug, Deserialize)]
struct ReplyDto {
    author_id: i64,
    author_name: String,
    text: String,
}

#[derive(Debug, Deserialize)]
struct SegmentDto {
    text: String,
    #[serde(default)]
    bold: bool,
    #[serde(default)]
    italic: bool,
    #[serde(default)]
    mono: bool,
    #[serde(default)]
    underline: bool,
    #[serde(default)]
    strikethrough: bool,
    #[serde(default)]
    spoiler: bool,
    color: Option<String>,
}

// ───────────────────────── DTO -> internal types ─────────────────────────

const WEBP_QUALITIES: [u32; 3] = [90, 75, 60];
const MAX_STICKER_BYTES: usize = 500 * 1024;

fn encode_webp(img: &Image) -> Result<Vec<u8>> {
    let mut last = Vec::new();
    for q in WEBP_QUALITIES {
        let data = img
            .encode(None, EncodedImageFormat::WEBP, q)
            .context("failed to encode WebP")?;
        last = data.as_bytes().to_vec();
        if last.len() <= MAX_STICKER_BYTES {
            break;
        }
    }
    Ok(last)
}

fn parse_color(s: &str) -> Result<Color> {
    let hex = s.trim_start_matches('#');
    let v = u32::from_str_radix(hex, 16).with_context(|| format!("bad color {s:?}"))?;
    match hex.len() {
        6 => Ok(Color::new(0xFF00_0000 | v)),
        8 => Ok(Color::new(v)),
        _ => bail!("bad color {s:?}: expected #RRGGBB or #AARRGGBB"),
    }
}

struct ImageStore<'a> {
    files: &'a HashMap<String, Vec<u8>>,
    cache: HashMap<String, Image>,
}

impl<'a> ImageStore<'a> {
    fn new(files: &'a HashMap<String, Vec<u8>>) -> Self {
        Self {
            files,
            cache: HashMap::new(),
        }
    }

    fn get(&mut self, name: &str) -> Result<Image> {
        if let Some(img) = self.cache.get(name) {
            return Ok(img.clone());
        }
        let bytes = self
            .files
            .get(name)
            .with_context(|| format!("image part {name:?} is referenced but not uploaded"))?;

        let img = Image::from_encoded(Data::new_copy(bytes))
            .with_context(|| format!("image part {name:?} could not be decoded"))?;

        if img.width() as i64 * img.height() as i64 > MAX_IMAGE_PIXELS {
            bail!(
                "image part {name:?} is too large ({}x{})",
                img.width(),
                img.height()
            );
        }

        self.cache.insert(name.to_string(), img.clone());
        Ok(img)
    }
}

fn into_chat_messages(
    req: RenderRequest,
    files: &HashMap<String, Vec<u8>>,
) -> Result<Vec<ChatMessage>> {
    let mut store = ImageStore::new(files);
    let mut out = Vec::with_capacity(req.messages.len());

    for (i, m) in req.messages.into_iter().enumerate() {
        let ctx = || format!("message #{i}");

        let avatar_img = match &m.avatar {
            Some(name) => match store.get(name) {
                Ok(img) => Some(img),
                Err(e) => {
                    eprintln!("message #{i}: avatar skipped: {e:#}");
                    None
                }
            },
            None => None,
        };

        let mut media: Vec<MediaItem> = vec![];
        for img in m.media {
            match store.get(&img.part) {
                Ok(i) => {
                    media.push(MediaItem { image: i, spoiler: img.spoiler });
                },
                Err(e) => {
                    eprintln!("message #{i}: media {} skipped: {e:#}", &img.part);
                },
            }
        }

        let segments = m
            .segments
            .into_iter()
            .map(|s| -> Result<TextSegment> {
                Ok(TextSegment {
                    text: s.text,
                    bold: s.bold,
                    italic: s.italic,
                    mono: s.mono,
                    color: s.color.as_deref().map(parse_color).transpose()?,
                    underline: s.underline,
                    strikethrough: s.strikethrough,
                    spoiler: s.spoiler,
                })
            })
            .collect::<Result<Vec<_>>>()
            .with_context(ctx)?;

        out.push(ChatMessage {
            author_id: m.author_id,
            author_name: m.author_name,
            tag: m.tag.filter(|t| !t.text.is_empty()).map(|t| Tag {
                text: t.text,
                kind: match t.kind {
                    TagKindDto::Owner => TagKind::Owner,
                    TagKindDto::Admin => TagKind::Admin,
                    TagKindDto::Member => TagKind::Member,
                },
            }),
            avatar_img,
            standalone: m.standalone,
            reply: m.reply.map(|r| ReplyInfo {
                author_name: r.author_name,
                author_id: r.author_id,
                text: r.text,
            }),
            segments,
            bubble_color: parse_color(&m.bubble_color).with_context(ctx)?,
            media,
        });
    }

    Ok(out)
}

// fn encode_png(img: &Image) -> Result<Vec<u8>> {
//     let data = img
//         .encode(None, EncodedImageFormat::PNG, None)
//         .context("failed to encode PNG")?;
//     Ok(data.as_bytes().to_vec())
// }

// ───────────────────────── Workers ─────────────────────────

enum JobError {
    BadInput(String),
    Internal(String),
}

struct Job {
    request: RenderRequest,
    files: HashMap<String, Vec<u8>>,
    reply: oneshot::Sender<Result<Vec<u8>, JobError>>,
}

fn process(
    renderer: &Renderer,
    request: RenderRequest,
    files: HashMap<String, Vec<u8>>,
) -> Result<Vec<u8>, JobError> {
    let messages =
        into_chat_messages(request, &files).map_err(|e| JobError::BadInput(format!("{e:#}")))?;

    let sticker = renderer
        .build_sticker_chat_stack(&messages)
        .map_err(|e| JobError::Internal(e.to_string()))?;

    encode_webp(&sticker).map_err(|e| JobError::Internal(format!("{e:#}")))
}

fn spawn_workers(n: usize, fonts_dir: &str, ui: UiConfig) -> Result<async_channel::Sender<Job>> {
    let (tx, rx) = async_channel::bounded::<Job>(QUEUE_SIZE);
    let (ready_tx, ready_rx) = mpsc::channel::<std::result::Result<(), String>>();

    for id in 0..n {
        let rx = rx.clone();
        let ready_tx = ready_tx.clone();
        let ui = ui.clone();
        let fonts_dir = fonts_dir.to_string();

        std::thread::Builder::new()
            .name(format!("render-{id}"))
            .spawn(move || {
                let renderer = match Renderer::new(&fonts_dir, ui) {
                    Ok(r) => {
                        let _ = ready_tx.send(Ok(()));
                        r
                    }
                    Err(e) => {
                        let _ = ready_tx.send(Err(e.to_string()));
                        return;
                    }
                };

                while let Ok(Job {
                    request,
                    files,
                    reply,
                }) = rx.recv_blocking()
                {
                    if reply.is_closed() {
                        continue;
                    }
                    // catch panic
                    let result = std::panic::catch_unwind(AssertUnwindSafe(|| {
                        process(&renderer, request, files)
                    }))
                    .unwrap_or_else(|_| Err(JobError::Internal("renderer panicked".into())));

                    let _ = reply.send(result);
                }
            })
            .context("failed to spawn worker thread")?;
    }

    // wait until all workers load the fonts. If even one fails, crash immediately.
    for _ in 0..n {
        match ready_rx.recv() {
            Ok(Ok(())) => {}
            Ok(Err(e)) => bail!("worker failed to initialise: {e}"),
            Err(_) => bail!("worker exited during initialisation"),
        }
    }

    Ok(tx)
}

// ───────────────────────── HTTP ─────────────────────────

struct AppState {
    jobs: async_channel::Sender<Job>,
}

struct AppError(StatusCode, String);

impl AppError {
    fn bad_request(msg: impl ToString) -> Self {
        Self(StatusCode::BAD_REQUEST, msg.to_string())
    }
    fn internal(msg: impl ToString) -> Self {
        Self(StatusCode::INTERNAL_SERVER_ERROR, msg.to_string())
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> Response {
        (self.0, self.1).into_response()
    }
}

async fn health() -> &'static str {
    "ok"
}

async fn render_handler(
    State(app): State<Arc<AppState>>,
    mut multipart: Multipart,
) -> Result<Response, AppError> {
    let mut request: Option<RenderRequest> = None;
    let mut files: HashMap<String, Vec<u8>> = HashMap::new();

    while let Some(field) = multipart
        .next_field()
        .await
        .map_err(AppError::bad_request)?
    {
        let name = field.name().unwrap_or_default().to_string();
        let data = field.bytes().await.map_err(AppError::bad_request)?;

        if name == "request" {
            request = Some(
                serde_json::from_slice(&data)
                    .map_err(|e| AppError::bad_request(format!("invalid request JSON: {e}")))?,
            );
        } else {
            if files.len() >= MAX_FILES {
                return Err(AppError::bad_request(format!(
                    "too many files (max {MAX_FILES})"
                )));
            }
            files.insert(name, data.to_vec());
        }
    }

    let request = request.ok_or_else(|| AppError::bad_request("missing \"request\" part"))?;

    if request.messages.is_empty() {
        return Err(AppError::bad_request("messages must not be empty"));
    }
    if request.messages.len() > MAX_MESSAGES {
        return Err(AppError::bad_request(format!(
            "too many messages (max {MAX_MESSAGES})"
        )));
    }

    let (reply_tx, reply_rx) = oneshot::channel();
    app.jobs
        .try_send(Job {
            request,
            files,
            reply: reply_tx,
        })
        .map_err(|e| match e {
            async_channel::TrySendError::Full(_) => {
                AppError(StatusCode::SERVICE_UNAVAILABLE, "renderer is busy".into())
            }
            async_channel::TrySendError::Closed(_) => AppError::internal("renderer is down"),
        })?;

    let result = tokio::time::timeout(RENDER_TIMEOUT, reply_rx)
        .await
        .map_err(|_| AppError(StatusCode::GATEWAY_TIMEOUT, "render timed out".into()))?
        .map_err(|_| AppError::internal("worker dropped the job"))?;

    match result {
        Ok(png) => Ok(([(header::CONTENT_TYPE, "image/webp")], png).into_response()),
        Err(JobError::BadInput(msg)) => Err(AppError::bad_request(msg)),
        Err(JobError::Internal(msg)) => {
            eprintln!("render failed: {msg}");
            Err(AppError::internal(msg))
        }
    }
}

// ───────────────────────── Launching ─────────────────────────

fn default_ui() -> UiConfig {
    UiConfig {
        padding_x: 10,
        padding_y: 12,
        name_font_size: 46,
        reply_font_size: 36,
        text_font_size: 50,
        gap_after_name: 2,
        gap_after_reply: 6,
        reply_line_width: 5,
        reply_line_gap: 1,
        corner_radius: 18,
        avatar_size: 45,
        avatar_gap: 8,
        vertical_gap: 4,
        media_radius: 12,
        media_group_gap: 6,
        gap_after_media: 10,
        bottom_margin: 14,
    }
}

fn env_or<T: std::str::FromStr>(key: &str, default: T) -> T {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

async fn shutdown_signal() {
    let ctrl_c = async {
        let _ = tokio::signal::ctrl_c().await;
    };

    #[cfg(unix)]
    let terminate = async {
        // docker stop send SIGTERM
        if let Ok(mut s) = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
        {
            s.recv().await;
        }
    };
    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}

#[tokio::main]
async fn main() -> Result<()> {
    let port: u16 = env_or("PORT", 8080);
    let workers: usize = env_or("WORKERS", 2).max(1);
    let fonts_dir = std::env::var("FONTS_DIR").unwrap_or_else(|_| "./fonts".into());

    println!("loading fonts from {fonts_dir} in {workers} worker(s)...");
    let jobs = spawn_workers(workers, &fonts_dir, default_ui())?;
    println!("workers ready");

    let app = Router::new()
        .route("/health", get(health))
        .route("/v1/render", post(render_handler))
        .layer(DefaultBodyLimit::max(MAX_BODY_BYTES))
        .with_state(Arc::new(AppState { jobs }));

    let listener = tokio::net::TcpListener::bind(("0.0.0.0", port))
        .await
        .with_context(|| format!("cannot bind port {port}"))?;
    println!("listening on :{port}");

    axum::serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await
        .map_err(|e| anyhow!("server error: {e}"))
}
