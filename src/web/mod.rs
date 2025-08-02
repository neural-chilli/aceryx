use anyhow::Result;
use axum::{serve, Router};
use std::time::Duration;
use tokio::{net::TcpListener, signal};
use tower::ServiceBuilder;
use tower_http::{timeout::TimeoutLayer, trace::TraceLayer};
use tracing::info;

mod handlers;
mod static_assets;
mod templates;

use handlers::create_routes;

/// Start the Axum web server with all configured routes and middleware
pub async fn start_server(host: &str, port: u16, dev_mode: bool) -> Result<()> {
    let app = create_app(dev_mode)?;

    let listener = TcpListener::bind(format!("{}:{}", host, port)).await?;
    info!(
        "Server started successfully - listening on http://{}:{}",
        host, port
    );
    info!("Press Ctrl+C to stop");

    // Start server with graceful shutdown
    serve(listener, app)
        .with_graceful_shutdown(shutdown_signal())
        .await?;

    info!("Server shutdown complete");
    Ok(())
}

/// Create the Axum application with all routes and middleware
fn create_app(dev_mode: bool) -> Result<Router> {
    let app = Router::new().merge(create_routes()?).layer(
        ServiceBuilder::new()
            .layer(TraceLayer::new_for_http())
            .layer(TimeoutLayer::new(Duration::from_secs(30))),
    );

    if dev_mode {
        info!("Development mode: Enhanced logging enabled");
    }

    Ok(app)
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

    #[test]
    fn test_app_creation() {
        // Test that we can create the app without errors
        let app_result = create_app(false);
        assert!(app_result.is_ok());
    }
}
