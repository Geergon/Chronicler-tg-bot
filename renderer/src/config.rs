use std::{
    fs,
    path::Path,
    sync::{Arc, mpsc::channel},
    time::Duration,
};

use anyhow::Result;
use arc_swap::ArcSwap;
use notify::{
    Config as NotifyConfig, RecommendedWatcher, RecursiveMode, Watcher, event::ModifyKind,
};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct Config {
    pub ui: UiConfig,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct UiConfig {
    pub padding_x: i32,
    pub padding_y: i32,
    pub name_font_size: i32,
    pub reply_font_size: i32,
    pub text_font_size: i32,
    pub gap_after_name: i32,
    pub gap_after_reply: i32,
    pub reply_line_width: i32,
    pub reply_line_gap: i32,
    pub corner_radius: i32,
    pub avatar_size: i32,
    pub avatar_gap: i32,
    pub vertical_gap: i32,
    pub media_radius: i32,
    pub media_group_gap: i32,
    pub gap_after_media: i32,
    pub bottom_margin: i32,
}

impl Default for UiConfig {
    fn default() -> Self {
        Self {
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
}

pub struct ConfigManager {
    config: ArcSwap<Config>,
    path: String,
}

impl ConfigManager {
    pub fn new() -> Result<Self> {
        let config_path = "./config/config.toml";
        let config_dir = "./config";

        if !fs::exists(config_path)? {
            if !fs::exists(config_dir)? {
                fs::create_dir_all("./config")?;
            }

            let default_config = Config {
                ui: UiConfig::default(),
            };

            let toml_string =
                toml::to_string_pretty(&default_config).expect("failed to serialize config");
            fs::write(config_path, toml_string)?;

            return Ok(Self {
                config: ArcSwap::from_pointee(Config {
                    ui: UiConfig::default(),
                }),
                path: config_path.to_string(),
            });
        }

        let config = Self::load_from_file(config_path)?;
        Ok(Self {
            config: ArcSwap::from_pointee(config),
            path: config_path.to_string(),
        })
    }

    fn load_from_file(path: &str) -> Result<Config> {
        let content = fs::read_to_string(&path)?;
        let config: Config = toml::from_str(&content)?;

        Ok(config)
    }

    pub fn get(&self) -> arc_swap::Guard<Arc<Config>> {
        self.config.load()
    }

    pub fn reload(&self) -> Result<()> {
        let new_config = Self::load_from_file(&self.path)?;
        self.config.store(Arc::new(new_config));
        Ok(())
    }

    pub fn watch_for_changes(manager: Arc<ConfigManager>) -> Result<()> {
        let (tx, rx) = channel();

        let mut watcher = RecommendedWatcher::new(
            move |res| {
                if let Ok(event) = res {
                    let _ = tx.send(event);
                }
            },
            NotifyConfig::default().with_poll_interval(Duration::from_secs(2)),
        )?;

        let config_path = Path::new("./config/config.toml");
        let parent_dir = Path::new("./config");
        watcher.watch(parent_dir, RecursiveMode::NonRecursive)?;
        let target_file_name = config_path.file_name();

        // Spawn watcher thread
        std::thread::spawn(move || {
            // Keep watcher alive
            let _watcher = watcher;

            for event in rx {
                let is_target_file = event
                    .paths
                    .iter()
                    .any(|p| p.file_name() == target_file_name);

                if is_target_file {
                    let is_data_modified =
                        matches!(event.kind, notify::EventKind::Modify(ModifyKind::Data(_)));
                    let is_created = event.kind.is_create();

                    if is_data_modified || is_created {
                        println!("Config file changed, reloading...");

                        // Small delay to ensure file write is complete
                        std::thread::sleep(Duration::from_millis(100));

                        match manager.reload() {
                            Ok(()) => println!("Configuration reloaded successfully"),
                            Err(e) => eprintln!("Failed to reload config: {}", e),
                        }
                    }
                }
            }
        });

        Ok(())
    }
}
