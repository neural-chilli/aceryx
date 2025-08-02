// src/web/mod.rs

use anyhow::Result;
use axum::{serve, Router, response::{IntoResponse, Html}, http::StatusCode};
use std::sync::Arc;
use std::time::Duration;
use tokio::{net::TcpListener, signal};
use tower::ServiceBuilder;
use tower_http::{
    cors::CorsLayer,
    timeout::TimeoutLayer,
    trace::TraceLayer,
    compression::CompressionLayer,
};
use tracing::{info, warn, error};

mod handlers;
mod static_assets;
mod templates;

use crate::api;
use crate::storage::FlowStorage;
use crate::tools::ToolRegistry;
use crate::error::{request_logging, error_handling};

/// Start the Axum web server with storage and tools integration
pub async fn start_server_with_storage(
    host: &str,
    port: u16,
    dev_mode: bool,
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Result<()> {
    let app = create_app_with_storage(dev_mode, storage.clone(), tool_registry.clone())?;

    let listener = TcpListener::bind(format!("{}:{}", host, port)).await?;
    info!(
        "🚀 Server started successfully - listening on http://{}:{}",
        host, port
    );

    if dev_mode {
        info!("🔧 Development mode: Enhanced logging and CORS enabled");
        info!("📋 Available endpoints:");
        info!("   GET  /                    - Dashboard");
        info!("   GET  /flows               - Flow listing");
        info!("   GET  /flows/new           - Create flow");
        info!("   GET  /flows/:id           - Flow details");
        info!("   GET  /flows/:id/design    - Flow designer");
        info!("   GET  /tools               - Tool registry");
        info!("   GET  /tools/:id           - Tool details");
        info!("   GET  /system              - System overview");
        info!("   GET  /health              - Health check");
        info!("   GET  /static/*            - Static assets");
        info!("   API  /api/v1/*            - REST API endpoints");
        info!("   HTMX /partials/*          - Dynamic content");
        info!("");
        info!("📖 Web UI: http://{}:{}/", host, port);
        info!("📊 API Health: http://{}:{}/health", host, port);
    }

    // Log startup information
    log_startup_info(&storage, &tool_registry).await;

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
    use crate::storage::memory::MemoryStorage;
    use crate::tools::{native::NativeProtocol, ToolRegistry};

    warn!("Using legacy start_server function - consider upgrading to start_server_with_storage");

    let storage = Arc::new(MemoryStorage::new());
    let mut tool_registry = ToolRegistry::new(storage.clone());
    tool_registry.add_protocol(Box::new(NativeProtocol::new()));
    tool_registry.refresh_tools().await?;

    start_server_with_storage(host, port, dev_mode, storage, Arc::new(tool_registry)).await
}

/// Create the Axum application with all routes, middleware, and integrations
fn create_app_with_storage(
    dev_mode: bool,
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Result<Router> {
    let mut app = Router::new()
        // Web UI routes (enhanced Tabler-based interface)
        .merge(handlers::create_routes(storage.clone(), tool_registry.clone())?)
        // API routes (flows, tools, system)
        .merge(api::create_api_router(storage.clone(), tool_registry.clone()));

    // Apply middleware stack
    let middleware_stack = ServiceBuilder::new()
        // Request logging and error handling
        .layer(axum::middleware::from_fn(request_logging))
        .layer(axum::middleware::from_fn(error_handling))
        // HTTP tracing
        .layer(TraceLayer::new_for_http())
        // Request timeout
        .layer(TimeoutLayer::new(Duration::from_secs(60)));

    // Conditional middleware based on mode
    if dev_mode {
        app = app.layer(middleware_stack);
    } else {
        let middleware_with_compression = middleware_stack
            .layer(CompressionLayer::new());
        app = app.layer(middleware_with_compression);
    }

    // Configure CORS based on mode
    if dev_mode {
        info!("🌐 CORS enabled for all origins (development mode)");
        app = app.layer(
            CorsLayer::new()
                .allow_origin(tower_http::cors::Any)
                .allow_methods(tower_http::cors::Any)
                .allow_headers(tower_http::cors::Any)
        );
    } else {
        // Production CORS - more restrictive
        app = app.layer(
            CorsLayer::new()
                .allow_origin([
                    "http://localhost:3000".parse().unwrap(),
                    "http://localhost:8080".parse().unwrap(),
                ])
                .allow_methods([
                    axum::http::Method::GET,
                    axum::http::Method::POST,
                    axum::http::Method::PUT,
                    axum::http::Method::DELETE,
                    axum::http::Method::OPTIONS,
                ])
                .allow_headers([
                    axum::http::header::CONTENT_TYPE,
                    axum::http::header::AUTHORIZATION,
                ])
        );
    }

    Ok(app)
}

/// Create the basic app for backward compatibility (without storage integration)
fn create_app(dev_mode: bool) -> Result<Router> {
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

/// Initialize the web server for testing purposes
pub async fn create_test_server(
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Result<Router> {
    create_app_with_storage(true, storage, tool_registry)
}

/// Health check handler specifically for the web module
pub async fn web_health_check(
    storage: &Arc<dyn FlowStorage>,
    tool_registry: &Arc<ToolRegistry>,
) -> Result<serde_json::Value> {
    use serde_json::json;

    // Check storage health
    let storage_health = match storage.health_check().await {
        Ok(health) => json!({
            "status": "healthy",
            "backend": health.backend_type,
            "flows": health.total_flows,
            "tools": health.total_tools
        }),
        Err(e) => {
            error!("Storage health check failed: {}", e);
            json!({
                "status": "unhealthy",
                "error": e.to_string()
            })
        }
    };

    // Check tool registry health
    let registry_health = match tool_registry.health_check().await {
        Ok(health) => json!({
            "status": if health.healthy { "healthy" } else { "unhealthy" },
            "protocols": health.protocols.len(),
            "cached_tools": health.cached_tools
        }),
        Err(e) => {
            error!("Tool registry health check failed: {}", e);
            json!({
                "status": "unhealthy",
                "error": e.to_string()
            })
        }
    };

    Ok(json!({
        "service": "aceryx-web",
        "version": env!("CARGO_PKG_VERSION"),
        "storage": storage_health,
        "tools": registry_health,
        "timestamp": chrono::Utc::now().to_rfc3339()
    }))
}

/// Error handling middleware for web routes
pub async fn handle_web_error(error: anyhow::Error) -> axum::response::Response {
    error!("Web error: {}", error);

    let error_html = format!(
        r#"
        <!DOCTYPE html>
        <html>
        <head>
            <title>Aceryx - Error</title>
            <style>
                body {{ font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 40px; background: #f8fafc; }}
                .error {{ color: #e53e3e; }}
                .container {{ max-width: 600px; margin: 0 auto; background: white; padding: 2rem; border-radius: 8px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); }}
                .header {{ display: flex; align-items: center; margin-bottom: 1rem; }}
                .back-link {{ color: #4299e1; text-decoration: none; font-weight: 500; }}
                .back-link:hover {{ text-decoration: underline; }}
            </style>
        </head>
        <body>
            <div class="container">
                <div class="header">
                    <h1>🍁 Aceryx</h1>
                </div>
                <h2 class="error">Something went wrong</h2>
                <p>The server encountered an error while processing your request.</p>
                <p><strong>Error:</strong> {}</p>
                <hr style="margin: 2rem 0; border: none; border-top: 1px solid #e2e8f0;">
                <p>
                    <a href="/dashboard" class="back-link">← Back to Dashboard</a> |
                    <a href="/health" class="back-link">Check System Health</a>
                </p>
            </div>
        </body>
        </html>
        "#,
        error
    );

    (StatusCode::INTERNAL_SERVER_ERROR, Html(error_html)).into_response()
}

/// Configuration for the web server
#[derive(Debug, Clone)]
pub struct WebConfig {
    pub dev_mode: bool,
    pub cors_origins: Vec<String>,
    pub request_timeout: Duration,
    pub compression_enabled: bool,
    pub static_cache_max_age: Duration,
}

impl Default for WebConfig {
    fn default() -> Self {
        Self {
            dev_mode: false,
            cors_origins: vec!["http://localhost:3000".to_string()],
            request_timeout: Duration::from_secs(60),
            compression_enabled: true,
            static_cache_max_age: Duration::from_secs(86400), // 24 hours
        }
    }
}

impl WebConfig {
    /// Create development configuration
    pub fn development() -> Self {
        Self {
            dev_mode: true,
            cors_origins: vec!["*".to_string()],
            request_timeout: Duration::from_secs(30),
            compression_enabled: false, // Disable in dev for faster iteration
            static_cache_max_age: Duration::from_secs(0), // No cache in dev
        }
    }

    /// Create production configuration
    pub fn production() -> Self {
        Self {
            dev_mode: false,
            cors_origins: vec![
                "https://yourdomain.com".to_string(),
                "https://app.yourdomain.com".to_string(),
            ],
            request_timeout: Duration::from_secs(60),
            compression_enabled: true,
            static_cache_max_age: Duration::from_secs(31536000), // 1 year
        }
    }
}

/// Advanced server startup with custom configuration
pub async fn start_server_with_config(
    host: &str,
    port: u16,
    config: WebConfig,
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Result<()> {
    let app = create_app_with_config(config.clone(), storage.clone(), tool_registry.clone())?;

    let listener = TcpListener::bind(format!("{}:{}", host, port)).await?;
    info!(
        "🚀 Aceryx server starting on http://{}:{} ({})",
        host,
        port,
        if config.dev_mode { "development" } else { "production" }
    );

    if config.dev_mode {
        info!("🔧 Development features enabled:");
        info!("   - Permissive CORS");
        info!("   - Extended logging");
        info!("   - No static caching");
        info!("   - Debug endpoints");
        info!("   - HTMX development tools");
    }

    // Log startup information
    log_startup_info(&storage, &tool_registry).await;

    serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    info!("Server shutdown complete");
    Ok(())
}

/// Create app with custom configuration
fn create_app_with_config(
    config: WebConfig,
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Result<Router> {
    let mut app = Router::new()
        .merge(handlers::create_routes(storage.clone(), tool_registry.clone())?)
        .merge(api::create_api_router(storage.clone(), tool_registry.clone()));

    // Apply middleware based on configuration
    let base_middleware = ServiceBuilder::new()
        .layer(axum::middleware::from_fn(request_logging))
        .layer(axum::middleware::from_fn(error_handling))
        .layer(TraceLayer::new_for_http())
        .layer(TimeoutLayer::new(config.request_timeout));

    if config.compression_enabled {
        app = app.layer(
            base_middleware.layer(CompressionLayer::new())
        );
    } else {
        app = app.layer(base_middleware);
    }

    // Configure CORS
    if config.dev_mode {
        app = app.layer(CorsLayer::permissive());
    } else {
        let cors_origins: Result<Vec<_>, _> = config
            .cors_origins
            .iter()
            .map(|origin| origin.parse())
            .collect();

        match cors_origins {
            Ok(origins) => {
                app = app.layer(
                    CorsLayer::new()
                        .allow_origin(origins)
                        .allow_methods([
                            axum::http::Method::GET,
                            axum::http::Method::POST,
                            axum::http::Method::PUT,
                            axum::http::Method::DELETE,
                            axum::http::Method::OPTIONS,
                        ])
                        .allow_headers([
                            axum::http::header::CONTENT_TYPE,
                            axum::http::header::AUTHORIZATION,
                        ])
                );
            }
            Err(e) => {
                warn!("Invalid CORS origin configuration: {}", e);
                app = app.layer(CorsLayer::permissive());
            }
        }
    }

    Ok(app)
}

/// Log startup information
async fn log_startup_info(
    storage: &Arc<dyn FlowStorage>,
    tool_registry: &Arc<ToolRegistry>,
) {
    // Log storage information
    match storage.health_check().await {
        Ok(health) => {
            info!("💾 Storage: {} ({} flows, {} tools)",
                health.backend_type, health.total_flows, health.total_tools);
        }
        Err(e) => {
            error!("💾 Storage health check failed: {}", e);
        }
    }

    // Log tool registry information
    match tool_registry.health_check().await {
        Ok(health) => {
            info!("🔧 Tools: {} protocols, {} cached tools",
                health.protocols.len(), health.cached_tools);

            for protocol in &health.protocols {
                let status = if protocol.healthy { "✅" } else { "❌" };
                info!("   {} {}: {} tools",
                    status, protocol.protocol_name, protocol.tool_count);
            }
        }
        Err(e) => {
            error!("🔧 Tool registry health check failed: {}", e);
        }
    }

    info!("🍁 Aceryx UI ready: Dashboard, Flow Designer, and Tool Registry available!");
}

/// Response helpers for consistent HTMX handling
pub mod response_helpers {
    use axum::{response::{Html, IntoResponse}, http::StatusCode};
    use serde_json::Value;
    use super::templates::Templates;

    /// Render response that handles both full page and HTMX requests
    pub fn render_response(
        templates: &Templates,
        template: &str,
        context: &Value,
        is_htmx: bool,
    ) -> Result<impl IntoResponse, crate::error::AceryxError> {
        let template_name = if is_htmx {
            // For HTMX requests, use partial templates
            &format!("partials/{}", template.replace("pages/", ""))
        } else {
            template
        };

        match templates.render(template_name, context) {
            Ok(html) => Ok(Html(html)),
            Err(e) => Err(crate::error::AceryxError::internal(format!("Template error: {}", e))),
        }
    }

    /// Create HTMX-aware JSON response
    pub fn htmx_json_response(data: Value, is_htmx: bool) -> impl IntoResponse {
        if is_htmx {
            // For HTMX, we might want to trigger specific client-side actions
            (
                StatusCode::OK,
                [("HX-Trigger", "dataUpdated")],
                axum::Json(data),
            )
        } else {
            (
                StatusCode::OK,
                [("Content-Type", "application/json")],
                axum::Json(data),
            )
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::memory::MemoryStorage;
    use crate::tools::{native::NativeProtocol, ToolRegistry};
    use axum::{body::Body, http::{Method, Request, StatusCode}};
    use tower::ServiceExt;

    async fn create_test_setup() -> (Arc<MemoryStorage>, Arc<ToolRegistry>) {
        let storage = Arc::new(MemoryStorage::new());
        let mut tool_registry = ToolRegistry::new(storage.clone());
        tool_registry.add_protocol(Box::new(NativeProtocol::new()));
        tool_registry.refresh_tools().await.unwrap();
        (storage, Arc::new(tool_registry))
    }

    #[tokio::test]
    async fn test_enhanced_app_creation() {
        let (storage, tool_registry) = create_test_setup().await;

        let app_result = create_app_with_storage(false, storage, tool_registry);
        assert!(app_result.is_ok());
    }

    #[tokio::test]
    async fn test_dev_mode_enhanced_app() {
        let (storage, tool_registry) = create_test_setup().await;

        let app_result = create_app_with_storage(true, storage, tool_registry);
        assert!(app_result.is_ok());
    }

    #[tokio::test]
    async fn test_dashboard_endpoint() {
        let (storage, tool_registry) = create_test_setup().await;
        let app = create_app_with_storage(true, storage, tool_registry).unwrap();

        let request = Request::builder()
            .method(Method::GET)
            .uri("/dashboard")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_flows_endpoint() {
        let (storage, tool_registry) = create_test_setup().await;
        let app = create_app_with_storage(true, storage, tool_registry).unwrap();

        let request = Request::builder()
            .method(Method::GET)
            .uri("/flows")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_tools_endpoint() {
        let (storage, tool_registry) = create_test_setup().await;
        let app = create_app_with_storage(true, storage, tool_registry).unwrap();

        let request = Request::builder()
            .method(Method::GET)
            .uri("/tools")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_htmx_partial_endpoints() {
        let (storage, tool_registry) = create_test_setup().await;
        let app = create_app_with_storage(true, storage, tool_registry).unwrap();

        // Test HTMX flow partial
        let request = Request::builder()
            .method(Method::GET)
            .uri("/partials/flows")
            .header("hx-request", "true")
            .body(Body::empty())
            .unwrap();

        let response = app.clone().oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        // Test HTMX tool partial
        let request = Request::builder()
            .method(Method::GET)
            .uri("/partials/tools")
            .header("hx-request", "true")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_api_integration() {
        let (storage, tool_registry) = create_test_setup().await;
        let app = create_app_with_storage(true, storage, tool_registry).unwrap();

        // Test that API endpoints still work
        let request = Request::builder()
            .method(Method::GET)
            .uri("/api/v1/flows")
            .body(Body::empty())
            .unwrap();

        let response = app.clone().oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let request = Request::builder()
            .method(Method::GET)
            .uri("/api/v1/tools")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_web_config() {
        let dev_config = WebConfig::development();
        assert!(dev_config.dev_mode);
        assert_eq!(dev_config.cors_origins, vec!["*"]);
        assert!(!dev_config.compression_enabled);

        let prod_config = WebConfig::production();
        assert!(!prod_config.dev_mode);
        assert!(prod_config.compression_enabled);
        assert!(prod_config.cors_origins.len() > 0);
        assert!(!prod_config.cors_origins.contains(&"*".to_string()));
    }

    #[tokio::test]
    async fn test_enhanced_web_health_check() {
        let (storage, tool_registry) = create_test_setup().await;

        let storage_trait: Arc<dyn FlowStorage> = storage;
        let health_result = web_health_check(&storage_trait, &tool_registry).await;
        assert!(health_result.is_ok());

        let health = health_result.unwrap();
        assert_eq!(health["service"], "aceryx-web");
        assert!(health["storage"].is_object());
        assert!(health["tools"].is_object());
    }
}