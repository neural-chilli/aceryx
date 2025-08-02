use rust_embed::RustEmbed;

/// Embedded static assets (CSS, JS, images, etc.)
#[derive(RustEmbed)]
#[folder = "web/static/"]
pub struct StaticAssets;
