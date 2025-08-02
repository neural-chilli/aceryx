// src/main.rs

use anyhow::Result;
use clap::{Parser, Subcommand};
use std::sync::Arc;
use tracing::{info, warn};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

mod api;
mod config;
mod error;
mod storage;
mod tools;
mod web;

use config::{load_config, generate_sample_config};
use storage::{memory::MemoryStorage, FlowStorage};
use tools::{native::NativeProtocol, ToolRegistry};

#[derive(Parser)]
#[command(name = "aceryx")]
#[command(about = "An open-source agentic flow builder for Rust")]
#[command(long_about = r#"
Aceryx is the visual AI workflow platform that bridges modern AI interfaces 
with enterprise systems through secure, high-performance workflow orchestration.

The Apache Camel of AI - Universal bridge for enterprise AI integration.
"#)]
struct Cli {
    #[command(subcommand)]
    command: Commands,
}

#[derive(Subcommand)]
enum Commands {
    /// Start the Aceryx server
    Serve {
        /// Port to bind to
        #[arg(short, long, env = "ACERYX_PORT")]
        port: Option<u16>,

        /// Host to bind to
        #[arg(long, env = "ACERYX_HOST")]
        host: Option<String>,

        /// Enable development mode (more verbose logging, CORS, etc.)
        #[arg(long)]
        dev: bool,

        /// Configuration file path
        #[arg(short, long, env = "ACERYX_CONFIG")]
        config: Option<String>,
    },
    /// Validate a flow configuration file
    Validate {
        /// Path to the flow configuration file
        #[arg(value_name = "FILE")]
        file: String,
    },
    /// Generate sample configuration files
    Config {
        /// Generate production configuration
        #[arg(long)]
        production: bool,
    },
    /// Tool management commands
    Tools {
        #[command(subcommand)]
        action: ToolCommands,
    },
    /// Show version information
    Version,
}

#[derive(Subcommand)]
enum ToolCommands {
    /// List available tools
    List {
        /// Filter by category
        #[arg(short, long)]
        category: Option<String>,
    },
    /// Refresh tools from all protocols
    Refresh,
    /// Execute a tool with JSON input
    Execute {
        /// Tool ID to execute
        tool_id: String,
        /// JSON input for the tool
        #[arg(short, long)]
        input: String,
    },
}

#[tokio::main]
async fn main() -> Result<()> {
    let cli = Cli::parse();

    match cli.command {
        Commands::Serve { port, host, dev, config } => {
            // Set config file path if provided
            if let Some(config_path) = config {
                std::env::set_var("ACERYX_CONFIG", config_path);
            }

            // Load configuration
            let mut app_config = load_config()?;

            // Override with CLI arguments
            if let Some(port) = port {
                app_config.server.port = port;
            }
            if let Some(host) = host {
                app_config.server.host = host;
            }

            // Initialize logging based on config
            init_logging(&app_config, dev)?;

            info!("🍁 Starting Aceryx - The Apache Camel of AI");
            info!("Version: {}", env!("CARGO_PKG_VERSION"));
            info!("Binding to {}:{}", app_config.server.host, app_config.server.port);
            info!("Storage backend: {:?}", app_config.storage.backend);

            if dev {
                warn!("Development mode enabled - not for production use");
                info!("CORS enabled for all origins");
                info!("Enhanced logging and debugging features active");
            }

            // Initialize storage backend
            let storage = create_storage_backend(&app_config).await?;
            info!("Storage backend initialized successfully");

            // Initialize tool registry
            let tool_registry = create_tool_registry(&app_config, storage.clone()).await?;
            info!("Tool registry initialized with {} protocols", 
                tool_registry.protocols().len());

            // Discover and register tools
            let discovered_tools = tool_registry.refresh_tools().await?;
            info!("Discovered {} tools across all protocols", discovered_tools);

            // Start the web server
            web::start_server_with_storage(
                &app_config.server.host,
                app_config.server.port,
                dev,
                storage,
                tool_registry,
            ).await?;
        }

        Commands::Validate { file } => {
            init_minimal_logging()?;
            info!("Validating flow configuration: {}", file);

            // TODO: Implement flow validation
            println!("✅ Flow validation will be implemented in next iteration");
            println!("   File: {}", file);
        }

        Commands::Config { production } => {
            init_minimal_logging()?;
            generate_sample_config(production)?;
        }

        Commands::Tools { action } => {
            init_minimal_logging()?;

            // For tool commands, we need to initialize storage and tools
            let app_config = load_config()?;
            let storage = create_storage_backend(&app_config).await?;
            let tool_registry = create_tool_registry(&app_config, storage).await?;

            match action {
                ToolCommands::List { category } => {
                    list_tools(&tool_registry, category).await?;
                }
                ToolCommands::Refresh => {
                    refresh_tools(&tool_registry).await?;
                }
                ToolCommands::Execute { tool_id, input } => {
                    execute_tool(&tool_registry, tool_id, input).await?;
                }
            }
        }

        Commands::Version => {
            println!("aceryx {}", env!("CARGO_PKG_VERSION"));
            println!("The Apache Camel of AI");
            println!();
            println!("Build information:");
            println!("  Rust version: {}", env!("RUSTC_VERSION").unwrap_or("unknown"));
            println!("  Build date: {}", env!("BUILD_DATE").unwrap_or("unknown"));
            println!("  Git commit: {}", env!("GIT_HASH").unwrap_or("unknown"));
            println!();
            println!("Features:");
            #[cfg(feature = "redis-storage")]
            println!("  ✓ Redis storage support");
            #[cfg(not(feature = "redis-storage"))]
            println!("  ✗ Redis storage support");

            #[cfg(feature = "postgres-storage")]
            println!("  ✓ PostgreSQL storage support");
            #[cfg(not(feature = "postgres-storage"))]
            println!("  ✗ PostgreSQL storage support");

            #[cfg(feature = "ai-agents")]
            println!("  ✓ AI agents support");
            #[cfg(not(feature = "ai-agents"))]
            println!("  ✗ AI agents support");
        }
    }

    Ok(())
}

/// Initialize logging based on configuration and development mode
fn init_logging(config: &config::AceryxConfig, dev_mode: bool) -> Result<()> {
    let filter = if dev_mode {
        "aceryx=debug,tower_http=debug,axum=debug,info"
    } else {
        &config.log_filter()
    };

    let subscriber = tracing_subscriber::registry()
        .with(
            tracing_subscriber::EnvFilter::try_from_default_env()
                .unwrap_or_else(|_| filter.into()),
        );

    match config.logging.format {
        config::LogFormat::Json => {
            subscriber
                .with(tracing_subscriber::fmt::layer().json())
                .init();
        }
        config::LogFormat::Compact => {
            subscriber
                .with(tracing_subscriber::fmt::layer().compact())
                .init();
        }
        config::LogFormat::Pretty => {
            subscriber
                .with(tracing_subscriber::fmt::layer().pretty())
                .init();
        }
    }

    Ok(())
}

/// Initialize minimal logging for CLI commands
fn init_minimal_logging() -> Result<()> {
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::from_default_env().unwrap_or_else(|_| "info".into()))
        .with(tracing_subscriber::fmt::layer().compact())
        .init();

    Ok(())
}

/// Create the appropriate storage backend based on configuration
async fn create_storage_backend(
    config: &config::AceryxConfig,
) -> Result<Arc<dyn FlowStorage>> {
    match config.storage.backend {
        config::StorageBackend::Memory => {
            info!("Using in-memory storage (development mode)");
            Ok(Arc::new(MemoryStorage::new()))
        }
        config::StorageBackend::Redis => {
            #[cfg(feature = "redis-storage")]
            {
                info!("Initializing Redis storage backend");
                // TODO: Implement Redis storage
                Err(anyhow::anyhow!("Redis storage not yet implemented"))
            }
            #[cfg(not(feature = "redis-storage"))]
            {
                Err(anyhow::anyhow!(
                    "Redis storage support not compiled in. Enable 'redis-storage' feature."
                ))
            }
        }
        config::StorageBackend::Postgres => {
            #[cfg(feature = "postgres-storage")]
            {
                info!("Initializing PostgreSQL storage backend");
                // TODO: Implement PostgreSQL storage
                Err(anyhow::anyhow!("PostgreSQL storage not yet implemented"))
            }
            #[cfg(not(feature = "postgres-storage"))]
            {
                Err(anyhow::anyhow!(
                    "PostgreSQL storage support not compiled in. Enable 'postgres-storage' feature."
                ))
            }
        }
    }
}

/// Create and configure the tool registry
async fn create_tool_registry(
    config: &config::AceryxConfig,
    storage: Arc<dyn FlowStorage>,
) -> Result<Arc<ToolRegistry>> {
    let mut registry = ToolRegistry::new(storage);

    // Add enabled protocols
    for protocol_name in &config.tools.enabled_protocols {
        match protocol_name.as_str() {
            "native" => {
                info!("Enabling native tool protocol");
                registry.add_protocol(Box::new(NativeProtocol::new()));
            }
            "mcp" => {
                info!("Enabling MCP (Model Context Protocol)");
                // TODO: Implement MCP protocol
                warn!("MCP protocol not yet implemented");
            }
            "openai" => {
                info!("Enabling OpenAI function calling protocol");
                // TODO: Implement OpenAI protocol
                warn!("OpenAI protocol not yet implemented");
            }
            unknown => {
                warn!("Unknown tool protocol '{}' - skipping", unknown);
            }
        }
    }

    Ok(Arc::new(registry))
}

/// List available tools
async fn list_tools(
    registry: &ToolRegistry,
    category_filter: Option<String>,
) -> Result<()> {
    use crate::storage::ToolCategory;

    let category = if let Some(cat_str) = category_filter {
        Some(match cat_str.to_lowercase().as_str() {
            "ai" => ToolCategory::AI,
            "http" => ToolCategory::Http,
            "database" => ToolCategory::Database,
            "files" => ToolCategory::Files,
            "messaging" => ToolCategory::Messaging,
            "enterprise" => ToolCategory::Enterprise,
            "custom" => ToolCategory::Custom,
            _ => {
                println!("❌ Unknown category: {}", cat_str);
                println!("Available categories: ai, http, database, files, messaging, enterprise, custom");
                return Ok(());
            }
        })
    } else {
        None
    };

    let tools = registry.storage.list_tools(category).await?;

    if tools.is_empty() {
        if let Some(cat) = category {
            println!("No tools found in category: {}", cat);
        } else {
            println!("No tools available. Run 'aceryx tools refresh' to discover tools.");
        }
        return Ok(());
    }

    println!("Available Tools:");
    println!("{:-<80}", "");

    for tool in tools {
        println!("🔧 {} ({})", tool.name, tool.id);
        println!("   Category: {}", tool.category);
        println!("   Description: {}", tool.description);
        println!("   Execution: {:?}", tool.execution_mode);
        println!();
    }

    println!("Total: {} tools", tools.len());
    Ok(())
}

/// Refresh tools from all protocols
async fn refresh_tools(registry: &ToolRegistry) -> Result<()> {
    println!("🔄 Refreshing tools from all protocols...");

    let discovered = registry.refresh_tools().await?;

    println!("✅ Refresh complete!");
    println!("   Discovered: {} tools", discovered);

    // Show health status
    let health = registry.health_check().await?;
    println!("\nProtocol Health:");
    for protocol in health.protocols {
        let status = if protocol.healthy { "✅" } else { "❌" };
        println!("   {} {}: {} tools", status, protocol.protocol_name, protocol.tool_count);
        if let Some(error) = protocol.error_message {
            println!("     Error: {}", error);
        }
    }

    Ok(())
}

/// Execute a tool with JSON input
async fn execute_tool(
    registry: &ToolRegistry,
    tool_id: String,
    input_json: String,
) -> Result<()> {
    use tools::ExecutionContext;

    println!("🚀 Executing tool: {}", tool_id);

    // Parse input JSON
    let input: serde_json::Value = serde_json::from_str(&input_json)
        .map_err(|e| anyhow::anyhow!("Invalid JSON input: {}", e))?;

    // Create execution context
    let context = ExecutionContext::new("cli-user".to_string());

    // Execute the tool
    let start_time = std::time::Instant::now();
    let result = registry.execute_tool(&tool_id, input, context).await;
    let duration = start_time.elapsed();

    match result {
        Ok(output) => {
            println!("✅ Execution successful ({:.2}ms)", duration.as_millis());
            println!("\nOutput:");
            println!("{}", serde_json::to_string_pretty(&output)?);
        }
        Err(e) => {
            println!("❌ Execution failed ({:.2}ms)", duration.as_millis());
            println!("Error: {}", e);
        }
    }

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
            Commands::Serve { port, host, dev, config } => {
                assert_eq!(port, None);
                assert_eq!(host, None);
                assert!(!dev);
                assert_eq!(config, None);
            }
            _ => panic!("Expected serve command"),
        }
    }

    #[test]
    fn test_cli_with_options() {
        // Test serve with custom options
        let cli = Cli::try_parse_from(&[
            "aceryx", "serve",
            "--port", "3000",
            "--host", "192.168.1.100",
            "--dev",
            "--config", "custom.toml"
        ]).unwrap();

        match cli.command {
            Commands::Serve { port, host, dev, config } => {
                assert_eq!(port, Some(3000));
                assert_eq!(host, Some("192.168.1.100".to_string()));
                assert!(dev);
                assert_eq!(config, Some("custom.toml".to_string()));
            }
            _ => panic!("Expected serve command"),
        }
    }

    #[test]
    fn test_tools_commands() {
        let cli = Cli::try_parse_from(&["aceryx", "tools", "list", "--category", "http"]).unwrap();
        match cli.command {
            Commands::Tools { action } => match action {
                ToolCommands::List { category } => {
                    assert_eq!(category, Some("http".to_string()));
                }
                _ => panic!("Expected list command"),
            },
            _ => panic!("Expected tools command"),
        }
    }

    #[test]
    fn test_config_command() {
        let cli = Cli::try_parse_from(&["aceryx", "config", "--production"]).unwrap();
        match cli.command {
            Commands::Config { production } => {
                assert!(production);
            }
            _ => panic!("Expected config command"),
        }
    }
}