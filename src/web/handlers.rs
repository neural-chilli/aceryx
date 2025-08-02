use anyhow::Result;
use axum::{
    http::{header, StatusCode},
    response::{Html, IntoResponse, Json, Response},
    routing::get,
    Router,
};
use serde_json::json;
use tracing::error;

use super::static_assets::StaticAssets;
use super::templates::Templates;

/// Create all application routes
pub fn create_routes() -> Result<Router> {
    let templates = Templates::new()?;

    Ok(Router::new()
        .route("/", get(root_handler))
        .route("/health", get(health_handler))
        .route("/static/*path", get(static_handler))
        .with_state(templates))
}

/// Handle requests to the root path - serve index.html from template
async fn root_handler(
    axum::extract::State(templates): axum::extract::State<Templates>,
) -> impl IntoResponse {
    let context = json!({
        "title": "Aceryx Flow Designer",
        "version": env!("CARGO_PKG_VERSION"),
        "features": [
            {
                "icon": "🎨",
                "title": "Visual Flow Designer",
                "description": "Drag-and-drop interface for building workflows"
            },
            {
                "icon": "🔗",
                "title": "MCP Integration",
                "description": "Native Model Context Protocol support"
            },
            {
                "icon": "⚡",
                "title": "Rust Performance",
                "description": "High-performance backend with Axum"
            }
        ]
    });

    match templates.render("index.html", &context) {
        Ok(html) => Html(html).into_response(),
        Err(e) => {
            error!("Template rendering error: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Html("<h1>Template Error</h1>".to_string()),
            )
                .into_response()
        }
    }
}

/// Health check endpoint with JSON response
async fn health_handler() -> Json<serde_json::Value> {
    Json(json!({
        "status": "healthy",
        "service": "aceryx",
        "version": env!("CARGO_PKG_VERSION"),
        "timestamp": chrono::Utc::now().to_rfc3339()
    }))
}

/// Serve static assets using rust-embed
async fn static_handler(
    axum::extract::Path(path): axum::extract::Path<String>,
) -> impl IntoResponse {
    match StaticAssets::get(&path) {
        Some(content) => {
            let mime_type = mime_guess::from_path(&path).first_or_octet_stream();

            Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, mime_type.as_ref())
                .header(header::CACHE_CONTROL, "public, max-age=31536000") // 1 year cache
                .body(axum::body::Body::from(content.data))
                .unwrap()
        }
        None => Response::builder()
            .status(StatusCode::NOT_FOUND)
            .body(axum::body::Body::from("File not found"))
            .unwrap(),
    }
}
