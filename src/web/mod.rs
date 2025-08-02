// src/web/mod.rs

use anyhow::Result;
use axum::{serve, Router};
use std::sync::Arc;
use std::time::Duration;
use tokio::{net::TcpListener, signal};
use tower::ServiceBuilder;
use tower_http::{timeout::TimeoutLayer, trace::TraceLayer};
use tracing::info;

mod handlers;
mod static_assets;
mod templates;

use handlers::create_routes;
use crate::api;
use crate::storage::FlowStorage;
use crate::tools::ToolRegistry;

/// Start the Axum web server with storage and tools integration
pub async fn start_server_with_storage(
    host: &str,
    port: u16,
    dev_mode: bool,
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Result<()> {
    let app = create_app_with_storage(dev_mode, storage, tool_registry)?;

    let listener = TcpListener::bind(format!("{}:{}", host, port)).await?;
    info!(
        "🚀 Server started successfully - listening on http://{}:{}",
        host, port
    );

    if dev_mode {
        info!("🔧 Development mode: Enhanced logging and CORS enabled");
        // In development, we could add additional middleware like:
        // - More permissive CORS
        // - Request/response debugging
        // - Hot reload capabilities (future)
    }

    Ok(app)
}

/// Create the basic app for backward compatibility (without storage integration)
fn create_app(dev_mode: bool) -> Result<Router> {
    // For backward compatibility, create with minimal dependencies
    use crate::storage::memory::MemoryStorage;
    use crate::tools::ToolRegistry;

    let storage = Arc::new(MemoryStorage::new());
    let tool_registry = Arc::new(ToolRegistry::new(storage.clone()));

    create_app_with_storage(dev_mode, storage, tool_registry)
}

/// Handle graceful shutdown signals
async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install signal handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {
            info!("Received Ctrl+C, shutting down gracefully...");
        },
        _ = terminate => {
            info!("Received terminate signal, shutting down gracefully...");
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::memory::MemoryStorage;
    use crate::tools::ToolRegistry;

    #[test]
    fn test_app_creation_with_storage() {
        let storage = Arc::new(MemoryStorage::new());
        let tool_registry = Arc::new(ToolRegistry::new(storage.clone()));

        let app_result = create_app_with_storage(false, storage, tool_registry);
        assert!(app_result.is_ok());
    }

    #[test]
    fn test_dev_mode_app_creation() {
        let storage = Arc::new(MemoryStorage::new());
        let tool_registry = Arc::new(ToolRegistry::new(storage.clone()));

        let app_result = create_app_with_storage(true, storage, tool_registry);
        assert!(app_result.is_ok());
    }
}!("📋 Available endpoints:");
info!("   GET  /           - Landing page");
info!("   GET  /health     - Health check");
info!("   GET  /api/v1/system/info - System information");
info!("   GET  /api/v1/flows       - List flows");
info!("   POST /api/v1/flows       - Create flow");
info!("   GET  /api/v1/tools       - List tools");
info!("   POST /api/v1/tools/refresh - Refresh tools");
info!("   POST /api/v1/tools/execute/:id - Execute tool");
info!("");
info!("📖 API Documentation: http://{}:{}/api/docs", host, port);
}

info!("Press Ctrl+C to stop");

// Start server with graceful shutdown
serve(listener, app)
.with_graceful_shutdown(shutdown_signal())
.await?;

info!("Server shutdown complete");
Ok(())
}

/// Legacy function for backward compatibility
pub async fn start_server(host: &str, port: u16, dev_mode: bool) -> Result<()> {
    // For backward compatibility, create minimal storage and empty tool registry
    use crate::storage::memory::MemoryStorage;
    use crate::tools::ToolRegistry;

    let storage = Arc::new(MemoryStorage::new());
    let tool_registry = Arc::new(ToolRegistry::new(storage.clone()));

    start_server_with_storage(host, port, dev_mode, storage, tool_registry).await
}

/// Create the Axum application with all routes, middleware, and integrations
fn create_app_with_storage(
    dev_mode: bool,
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Result<Router> {
    let app = Router::new()
        // Web UI routes (landing page, static assets)
        .merge(create_routes()?)
        // API routes (flows, tools, system)
        .merge(api::create_api_router(storage.clone(), tool_registry.clone()))
        // Global middleware
        .layer(
            ServiceBuilder::new()
                .layer(TraceLayer::new_for_http())
                .layer(TimeoutLayer::new(Duration::from_secs(60)))
        );

    if dev_mode {
        info