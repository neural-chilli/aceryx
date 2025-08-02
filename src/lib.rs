// src/lib.rs

//! # Aceryx - The multi-tool of AI
//!
//! Aceryx is a visual AI workflow platform that bridges modern AI interfaces 
//! with enterprise systems through secure, high-performance workflow orchestration.
//!
//! ## Features
//!
//! - **Visual Flow Designer** - Intuitive drag-and-drop interface powered by ReactFlow
//! - **Universal Tool Registry** - Supporting any AI protocol or enterprise system
//! - **High-Performance Execution** - Rust core with WASM extensibility
//! - **Enterprise-Grade Security** - Sandboxed execution environments
//!
//! ## Quick Start
//!
//! ```rust,no_run
//! use aceryx::storage::memory::MemoryStorage;
//! use aceryx::tools::{ToolRegistry, native::NativeProtocol};
//! use std::sync::Arc;
//!
//! #[tokio::main]
//! async fn main() -> anyhow::Result<()> {
//!     // Initialize storage and tool registry
//!     let storage = Arc::new(MemoryStorage::new());
//!     let mut registry = ToolRegistry::new(storage.clone());
//!     registry.add_protocol(Box::new(NativeProtocol::new()));
//!     
//!     // Discover tools
//!     let discovered = registry.refresh_tools().await?;
//!     println!("Discovered {} tools", discovered);
//!     
//!     Ok(())
//! }
//! ```

pub mod api;
pub mod config;
pub mod error;
pub mod storage;
pub mod tools;
pub mod web;

// Re-export commonly used types for convenience
pub use error::AceryxError;
pub use storage::{Flow, FlowStorage, ToolDefinition};
pub use tools::{ToolRegistry, ExecutionContext};

/// Result type alias for Aceryx operations
pub type Result<T> = std::result::Result<T, AceryxError>;

/// Version information
pub const VERSION: &str = env!("CARGO_PKG_VERSION");
pub const DESCRIPTION: &str = env!("CARGO_PKG_DESCRIPTION");

/// Initialize logging for library usage
pub fn init_logging() -> anyhow::Result<()> {
    use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt, EnvFilter};

    tracing_subscriber::registry()
        .with(
            EnvFilter::try_from_default_env()
                .or_else(|_| EnvFilter::try_new("aceryx=info"))
                .unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .with(tracing_subscriber::fmt::layer())
        .try_init()
        .map_err(|e| anyhow::anyhow!("Failed to initialize logging: {}", e))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_version_info() {
        assert!(!VERSION.is_empty());
        assert!(!DESCRIPTION.is_empty());
    }

    #[tokio::test]
    async fn test_basic_setup() {
        use crate::storage::memory::MemoryStorage;
        use crate::tools::native::NativeProtocol;
        use std::sync::Arc;

        let storage = Arc::new(MemoryStorage::new());
        let mut registry = ToolRegistry::new(storage.clone());
        registry.add_protocol(Box::new(NativeProtocol::new()));

        let discovered = registry.refresh_tools().await.unwrap();
        assert!(discovered > 0);
    }
}