use anyhow::Result;
use clap::{Parser, Subcommand};
use tracing::{info, warn};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[derive(Parser)]
#[command(name = "aceryx")]
#[command(about = "An open-source agentic flow builder for Rust")]
#[command(long_about = None)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Start the Aceryx server
    Serve {
        /// Port to bind to
        #[arg(short, long, env = "ACERYX_PORT", default_value = "8080")]
        port: u16,

        /// Host to bind to
        #[arg(long, env = "ACERYX_HOST", default_value = "0.0.0.0")]
        host: String,

        /// Enable development mode (more verbose logging, hot reload)
        #[arg(long)]
        dev: bool,
    },
    /// Validate a flow configuration file
    Validate {
        /// Path to the flow configuration file
        #[arg(value_name = "FILE")]
        file: String,
    },
    /// Show version information
    Version,
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Commands::Serve { port, host, dev } => {
            init_logging(dev)?;

            info!("🍁 Starting Aceryx server");
            info!("Version: {}", env!("CARGO_PKG_VERSION"));
            info!("Binding to {}:{}", host, port);

            if dev {
                warn!("Development mode enabled - not for production use");
            }

            // For now, just bind and hold
            let listener = tokio::net::TcpListener::bind(format!("{}:{}", host, port)).await?;
            info!(
                "Server started successfully - listening on http://{}:{}",
                host, port
            );
            info!("Press Ctrl+C to stop");

            // Simple holding pattern until we implement the actual server
            loop {
                tokio::time::sleep(tokio::time::Duration::from_secs(1)).await;
            }
        }

        Commands::Validate { file } => {
            init_logging(false)?;
            info!("Validating flow configuration: {}", file);

            // TODO: Implement flow validation
            println!("✅ Flow validation will be implemented in next iteration");
            Ok(())
        }

        Commands::Version => {
            println!("aceryx {}", env!("CARGO_PKG_VERSION"));
            println!(
                "Built with Rust {}",
                std::env::var("RUSTC_VERSION").unwrap_or_else(|_| "unknown".to_string())
            );
            Ok(())
        }
    }
}


/// Initialize logging/tracing with appropriate levels for development vs production
fn init_logging(dev_mode: bool) -> Result<()> {
    let filter = if dev_mode {
        "aceryx=debug,tower_http=debug,axum=debug,info"
    } else {
        "aceryx=info,warn"
    };

    tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env().unwrap_or_else(|_| filter.into()),
        )
        .with(tracing_subscriber::fmt::layer())
        .init();

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_cli_parsing() {
        // Test basic serve command
        let cli = Cli::try_parse_from(&["aceryx", "serve"]).unwrap();
        match cli.command {
            Commands::Serve { port, host, dev } => {
                assert_eq!(port, 8080);
                assert_eq!(host, "0.0.0.0");
                assert!(!dev);
            }
            _ => panic!("Expected serve command"),
        }
    }

    #[test]
    fn test_cli_with_options() {
        // Test serve with custom port and dev mode
        let cli = Cli::try_parse_from(&["aceryx", "serve", "--port", "3000", "--dev"]).unwrap();
        match cli.command {
            Commands::Serve { port, host, dev } => {
                assert_eq!(port, 3000);
                assert_eq!(host, "0.0.0.0");
                assert!(dev);
            }
            _ => panic!("Expected serve command"),
        }
    }

    #[test]
    fn test_validate_command() {
        let cli = Cli::try_parse_from(&["aceryx", "validate", "test-flow.yaml"]).unwrap();
        match cli.command {
            Commands::Validate { file } => {
                assert_eq!(file, "test-flow.yaml");
            }
            _ => panic!("Expected validate command"),
        }
    }
}
