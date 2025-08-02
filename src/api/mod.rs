// src/api/mod.rs

use axum::Router;
use std::sync::Arc;
use tower_http::cors::CorsLayer;

pub mod flows;
pub mod tools;

use crate::storage::FlowStorage;
use crate::tools::ToolRegistry;

/// Create the complete API router with all endpoints
pub fn create_api_router(
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Router {
    Router::new()
        .nest("/api/v1/flows", flows::create_routes(storage.clone()))
        .nest("/api/v1/tools", tools::create_routes(tool_registry.clone()))
        .nest("/api/v1/system", create_system_routes(storage))
        .layer(CorsLayer::permissive())
    // Note: Middleware will be added at the web layer
}

/// System-level routes (health, info, etc.)
fn create_system_routes(storage: Arc<dyn FlowStorage>) -> Router {
    use axum::{response::Json, routing::get};
    use serde_json::json;

    async fn system_info(
        axum::extract::State(storage): axum::extract::State<Arc<dyn FlowStorage>>,
    ) -> Json<serde_json::Value> {
        let health = storage.health_check().await.unwrap_or_else(|e| {
            crate::storage::StorageHealth::unhealthy("unknown".to_string(), e.to_string())
        });

        Json(json!({
            "service": "aceryx",
            "version": env!("CARGO_PKG_VERSION"),
            "description": env!("CARGO_PKG_DESCRIPTION"),
            "storage": {
                "backend": health.backend_type,
                "healthy": health.healthy,
                "flows": health.total_flows,
                "tools": health.total_tools
            },
            "timestamp": chrono::Utc::now().to_rfc3339()
        }))
    }

    Router::new()
        .route("/info", get(system_info))
        .with_state(storage)
}