// src/storage/mod.rs

use async_trait::async_trait;
use anyhow::Result;

pub mod memory;
pub mod types;

pub use types::*;

/// Core storage trait for flow management and tool registry
///
/// This trait serves as the "rosetta stone" for all storage implementations,
/// enabling seamless transitions between development (memory) and production 
/// (Redis/PostgreSQL) deployments.
#[async_trait]
pub trait FlowStorage: Send + Sync {
    // ========================================================================
    // Flow CRUD Operations
    // ========================================================================

    /// Create a new flow and return its generated ID
    async fn create_flow(&self, flow: Flow) -> Result<FlowId>;

    /// Retrieve a flow by ID, returning None if not found
    async fn get_flow(&self, id: &FlowId) -> Result<Option<Flow>>;

    /// List flows with optional filtering and pagination
    async fn list_flows(&self, filters: FlowFilters) -> Result<Vec<Flow>>;

    /// Update an existing flow (must exist)
    async fn update_flow(&self, flow: Flow) -> Result<()>;

    /// Delete a flow by ID
    async fn delete_flow(&self, id: &FlowId) -> Result<()>;

    // ========================================================================
    // Flow Versioning
    // ========================================================================

    /// Create a new version of an existing flow
    async fn create_flow_version(&self, flow_id: &FlowId, flow: Flow) -> Result<String>;

    /// Get a specific version of a flow
    async fn get_flow_version(&self, flow_id: &FlowId, version: &str) -> Result<Option<Flow>>;

    /// List all versions for a flow
    async fn list_flow_versions(&self, flow_id: &FlowId) -> Result<Vec<String>>;

    // ========================================================================
    // Tool Registry Operations
    // ========================================================================

    /// Register a new tool in the universal registry
    async fn register_tool(&self, tool: ToolDefinition) -> Result<()>;

    /// Retrieve a tool definition by ID
    async fn get_tool(&self, id: &str) -> Result<Option<ToolDefinition>>;

    /// List tools, optionally filtered by category
    async fn list_tools(&self, category: Option<ToolCategory>) -> Result<Vec<ToolDefinition>>;

    /// Update an existing tool definition
    async fn update_tool(&self, tool: ToolDefinition) -> Result<()>;

    /// Remove a tool from the registry
    async fn delete_tool(&self, id: &str) -> Result<()>;

    // ========================================================================
    // Search and Discovery
    // ========================================================================

    /// Search flows by name, description, or tags
    async fn search_flows(&self, query: &str) -> Result<Vec<Flow>>;

    /// Search tools by name, description, or category
    async fn search_tools(&self, query: &str) -> Result<Vec<ToolDefinition>>;

    // ========================================================================
    // Health and Diagnostics
    // ========================================================================

    /// Check storage health and return basic metrics
    async fn health_check(&self) -> Result<StorageHealth>;
}

/// Storage health information for monitoring and diagnostics
#[derive(Debug, Clone)]
pub struct StorageHealth {
    pub healthy: bool,
    pub backend_type: String,
    pub total_flows: u64,
    pub total_tools: u64,
    pub version: String,
    pub last_check: chrono::DateTime<chrono::Utc>,
}

impl StorageHealth {
    pub fn new(backend_type: String, total_flows: u64, total_tools: u64) -> Self {
        Self {
            healthy: true,
            backend_type,
            total_flows,
            total_tools,
            version: env!("CARGO_PKG_VERSION").to_string(),
            last_check: chrono::Utc::now(),
        }
    }

    pub fn unhealthy(backend_type: String, error: String) -> Self {
        Self {
            healthy: false,
            backend_type,
            total_flows: 0,
            total_tools: 0,
            version: format!("{} (ERROR: {})", env!("CARGO_PKG_VERSION"), error),
            last_check: chrono::Utc::now(),
        }
    }
}

/// Helper trait for storage implementations that need initialization
#[async_trait]
pub trait StorageInit {
    /// Initialize storage backend (create tables, indices, etc.)
    async fn initialize(&self) -> Result<()>;

    /// Migrate storage schema to latest version
    async fn migrate(&self) -> Result<()>;

    /// Clean up storage resources
    async fn cleanup(&self) -> Result<()>;
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::memory::MemoryStorage;

    #[tokio::test]
    async fn test_storage_trait_flow_operations() {
        let storage = MemoryStorage::new();

        // Test flow creation
        let flow = Flow::new(
            "Test Flow".to_string(),
            "Test Description".to_string(),
            "test_user".to_string(),
        );
        let flow_id = storage.create_flow(flow.clone()).await.unwrap();

        // Test flow retrieval
        let retrieved = storage.get_flow(&flow_id).await.unwrap();
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().name, "Test Flow");

        // Test flow listing
        let flows = storage.list_flows(FlowFilters::default()).await.unwrap();
        assert_eq!(flows.len(), 1);

        // Test flow update
        let mut updated_flow = flow.clone();
        updated_flow.id = flow_id;
        updated_flow.name = "Updated Flow".to_string();
        storage.update_flow(updated_flow).await.unwrap();

        let retrieved = storage.get_flow(&flow_id).await.unwrap().unwrap();
        assert_eq!(retrieved.name, "Updated Flow");

        // Test flow deletion
        storage.delete_flow(&flow_id).await.unwrap();
        let retrieved = storage.get_flow(&flow_id).await.unwrap();
        assert!(retrieved.is_none());
    }

    #[tokio::test]
    async fn test_storage_trait_tool_operations() {
        let storage = MemoryStorage::new();

        // Test tool registration
        let tool = ToolDefinition::new(
            "test_tool".to_string(),
            "Test Tool".to_string(),
            "A test tool".to_string(),
            ToolCategory::Http,
            serde_json::json!({"type": "object"}),
            serde_json::json!({"type": "object"}),
            ExecutionMode::Wasm { permissions: WasmPermissions::default() },
        );

        storage.register_tool(tool.clone()).await.unwrap();

        // Test tool retrieval
        let retrieved = storage.get_tool("test_tool").await.unwrap();
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().name, "Test Tool");

        // Test tool listing
        let tools = storage.list_tools(None).await.unwrap();
        assert_eq!(tools.len(), 1);

        // Test category filtering
        let http_tools = storage.list_tools(Some(ToolCategory::Http)).await.unwrap();
        assert_eq!(http_tools.len(), 1);

        let ai_tools = storage.list_tools(Some(ToolCategory::AI)).await.unwrap();
        assert_eq!(ai_tools.len(), 0);
    }

    #[tokio::test]
    async fn test_storage_health_check() {
        let storage = MemoryStorage::new();
        let health = storage.health_check().await.unwrap();

        assert!(health.healthy);
        assert_eq!(health.backend_type, "memory");
        assert_eq!(health.total_flows, 0);
        assert_eq!(health.total_tools, 0);
    }
}