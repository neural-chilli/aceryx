// src/storage/memory.rs

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use uuid::Uuid;

use super::{Flow, FlowFilters, FlowId, FlowStorage, StorageHealth, ToolCategory, ToolDefinition};

/// In-memory storage implementation using DashMap for high-performance concurrent access
///
/// This implementation is perfect for:
/// - Development and testing
/// - Single-node deployments
/// - Quick prototyping
/// - Situations where persistence isn't required
#[derive(Debug)]
pub struct MemoryStorage {
    flows: Arc<RwLock<HashMap<FlowId, Flow>>>,
    flow_versions: Arc<RwLock<HashMap<FlowId, HashMap<String, Flow>>>>,
    tools: Arc<RwLock<HashMap<String, ToolDefinition>>>,
}

impl MemoryStorage {
    /// Create a new memory storage instance
    pub fn new() -> Self {
        Self {
            flows: Arc::new(RwLock::new(HashMap::new())),
            flow_versions: Arc::new(RwLock::new(HashMap::new())),
            tools: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Helper function to perform case-insensitive search in text fields
    fn matches_query(text: &str, query: &str) -> bool {
        text.to_lowercase().contains(&query.to_lowercase())
    }

    /// Apply flow filters to a flow
    fn flow_matches_filters(flow: &Flow, filters: &FlowFilters) -> bool {
        // Filter by creator
        if let Some(ref created_by) = filters.created_by {
            if flow.created_by != *created_by {
                return false;
            }
        }

        // Filter by tags (flow must have ALL specified tags)
        if !filters.tags.is_empty() {
            for required_tag in &filters.tags {
                if !flow.tags.contains(required_tag) {
                    return false;
                }
            }
        }

        // Note: category filter is not applied to flows directly
        // as flows don't have categories (their nodes/tools do)

        true
    }

    /// Apply pagination to a vector of results
    fn apply_pagination<T>(mut items: Vec<T>, filters: &FlowFilters) -> Vec<T> {
        // Apply offset
        if let Some(offset) = filters.offset {
            if offset < items.len() {
                items = items.into_iter().skip(offset).collect();
            } else {
                return Vec::new();
            }
        }

        // Apply limit
        if let Some(limit) = filters.limit {
            items.truncate(limit);
        }

        items
    }
}

impl Default for MemoryStorage {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl FlowStorage for MemoryStorage {
    async fn create_flow(&self, mut flow: Flow) -> Result<FlowId> {
        // Validate the flow before storing
        flow.validate().map_err(|e| anyhow!("Flow validation failed: {}", e))?;

        let flow_id = flow.id;

        let mut flows = self.flows.write().await;

        // Check if flow with this ID already exists
        if flows.contains_key(&flow_id) {
            return Err(anyhow!("Flow with ID {} already exists", flow_id));
        }

        flows.insert(flow_id, flow);
        Ok(flow_id)
    }

    async fn get_flow(&self, id: &FlowId) -> Result<Option<Flow>> {
        let flows = self.flows.read().await;
        Ok(flows.get(id).cloned())
    }

    async fn list_flows(&self, filters: FlowFilters) -> Result<Vec<Flow>> {
        let flows = self.flows.read().await;

        let mut result: Vec<Flow> = flows
            .values()
            .filter(|flow| Self::flow_matches_filters(flow, &filters))
            .cloned()
            .collect();

        // Sort by creation time (newest first)
        result.sort_by(|a, b| b.created_at.cmp(&a.created_at));

        // Apply pagination
        result = Self::apply_pagination(result, &filters);

        Ok(result)
    }

    async fn update_flow(&self, mut flow: Flow) -> Result<()> {
        // Validate the flow before updating
        flow.validate().map_err(|e| anyhow!("Flow validation failed: {}", e))?;

        let mut flows = self.flows.write().await;

        if !flows.contains_key(&flow.id) {
            return Err(anyhow!("Flow with ID {} not found", flow.id));
        }

        // Update timestamp
        flow.touch();
        flows.insert(flow.id, flow);
        Ok(())
    }

    async fn delete_flow(&self, id: &FlowId) -> Result<()> {
        let mut flows = self.flows.write().await;
        let mut versions = self.flow_versions.write().await;

        flows.remove(id);
        versions.remove(id); // Also remove all versions

        Ok(())
    }

    async fn create_flow_version(&self, flow_id: &FlowId, flow: Flow) -> Result<String> {
        // Validate the flow
        flow.validate().map_err(|e| anyhow!("Flow validation failed: {}", e))?;

        let flows = self.flows.read().await;

        // Ensure the base flow exists
        if !flows.contains_key(flow_id) {
            return Err(anyhow!("Base flow with ID {} not found", flow_id));
        }

        drop(flows); // Release the read lock

        let mut versions = self.flow_versions.write().await;

        let flow_versions = versions.entry(*flow_id).or_insert_with(HashMap::new);
        let version = flow.version.clone();

        if flow_versions.contains_key(&version) {
            return Err(anyhow!("Version {} already exists for flow {}", version, flow_id));
        }

        flow_versions.insert(version.clone(), flow);
        Ok(version)
    }

    async fn get_flow_version(&self, flow_id: &FlowId, version: &str) -> Result<Option<Flow>> {
        let versions = self.flow_versions.read().await;

        Ok(versions
            .get(flow_id)
            .and_then(|flow_versions| flow_versions.get(version))
            .cloned())
    }

    async fn list_flow_versions(&self, flow_id: &FlowId) -> Result<Vec<String>> {
        let versions = self.flow_versions.read().await;

        Ok(versions
            .get(flow_id)
            .map(|flow_versions| {
                let mut version_list: Vec<String> = flow_versions.keys().cloned().collect();
                version_list.sort();
                version_list
            })
            .unwrap_or_default())
    }

    async fn register_tool(&self, tool: ToolDefinition) -> Result<()> {
        let mut tools = self.tools.write().await;

        if tools.contains_key(&tool.id) {
            return Err(anyhow!("Tool with ID {} already exists", tool.id));
        }

        tools.insert(tool.id.clone(), tool);
        Ok(())
    }

    async fn get_tool(&self, id: &str) -> Result<Option<ToolDefinition>> {
        let tools = self.tools.read().await;
        Ok(tools.get(id).cloned())
    }

    async fn list_tools(&self, category: Option<ToolCategory>) -> Result<Vec<ToolDefinition>> {
        let tools = self.tools.read().await;

        let mut result: Vec<ToolDefinition> = tools
            .values()
            .filter(|tool| {
                category.as_ref().map_or(true, |cat| tool.category == *cat)
            })
            .cloned()
            .collect();

        // Sort by name for consistent ordering
        result.sort_by(|a, b| a.name.cmp(&b.name));

        Ok(result)
    }

    async fn update_tool(&self, mut tool: ToolDefinition) -> Result<()> {
        let mut tools = self.tools.write().await;

        if !tools.contains_key(&tool.id) {
            return Err(anyhow!("Tool with ID {} not found", tool.id));
        }

        // Update timestamp
        tool.touch();
        tools.insert(tool.id.clone(), tool);
        Ok(())
    }

    async fn delete_tool(&self, id: &str) -> Result<()> {
        let mut tools = self.tools.write().await;
        tools.remove(id);
        Ok(())
    }

    async fn search_flows(&self, query: &str) -> Result<Vec<Flow>> {
        if query.trim().is_empty() {
            return self.list_flows(FlowFilters::default()).await;
        }

        let flows = self.flows.read().await;

        let mut result: Vec<Flow> = flows
            .values()
            .filter(|flow| {
                Self::matches_query(&flow.name, query)
                    || Self::matches_query(&flow.description, query)
                    || flow.tags.iter().any(|tag| Self::matches_query(tag, query))
                    || Self::matches_query(&flow.created_by, query)
            })
            .cloned()
            .collect();

        // Sort by relevance (name matches first, then description, then tags)
        result.sort_by(|a, b| {
            let a_name_match = Self::matches_query(&a.name, query);
            let b_name_match = Self::matches_query(&b.name, query);

            match (a_name_match, b_name_match) {
                (true, false) => std::cmp::Ordering::Less,
                (false, true) => std::cmp::Ordering::Greater,
                _ => a.name.cmp(&b.name),
            }
        });

        Ok(result)
    }

    async fn search_tools(&self, query: &str) -> Result<Vec<ToolDefinition>> {
        if query.trim().is_empty() {
            return self.list_tools(None).await;
        }

        let tools = self.tools.read().await;

        let mut result: Vec<ToolDefinition> = tools
            .values()
            .filter(|tool| {
                Self::matches_query(&tool.name, query)
                    || Self::matches_query(&tool.description, query)
                    || Self::matches_query(&tool.category.to_string(), query)
            })
            .cloned()
            .collect();

        // Sort by relevance
        result.sort_by(|a, b| {
            let a_name_match = Self::matches_query(&a.name, query);
            let b_name_match = Self::matches_query(&b.name, query);

            match (a_name_match, b_name_match) {
                (true, false) => std::cmp::Ordering::Less,
                (false, true) => std::cmp::Ordering::Greater,
                _ => a.name.cmp(&b.name),
            }
        });

        Ok(result)
    }

    async fn health_check(&self) -> Result<StorageHealth> {
        let flows = self.flows.read().await;
        let tools = self.tools.read().await;

        Ok(StorageHealth::new(
            "memory".to_string(),
            flows.len() as u64,
            tools.len() as u64,
        ))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[tokio::test]
    async fn test_memory_storage_flow_crud() {
        let storage = MemoryStorage::new();

        // Create a test flow
        let flow = Flow::new(
            "Test Flow".to_string(),
            "Test Description".to_string(),
            "test_user".to_string(),
        );
        let flow_id = flow.id;

        // Test creation
        let created_id = storage.create_flow(flow).await.unwrap();
        assert_eq!(created_id, flow_id);

        // Test retrieval
        let retrieved = storage.get_flow(&flow_id).await.unwrap();
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().name, "Test Flow");

        // Test update
        let mut updated_flow = storage.get_flow(&flow_id).await.unwrap().unwrap();
        updated_flow.name = "Updated Flow".to_string();
        storage.update_flow(updated_flow).await.unwrap();

        let retrieved = storage.get_flow(&flow_id).await.unwrap().unwrap();
        assert_eq!(retrieved.name, "Updated Flow");

        // Test deletion
        storage.delete_flow(&flow_id).await.unwrap();
        let retrieved = storage.get_flow(&flow_id).await.unwrap();
        assert!(retrieved.is_none());
    }

    #[tokio::test]
    async fn test_memory_storage_tool_crud() {
        let storage = MemoryStorage::new();

        // Create a test tool
        let tool = ToolDefinition::new(
            "http_request".to_string(),
            "HTTP Request".to_string(),
            "Make HTTP requests".to_string(),
            ToolCategory::Http,
            json!({"type": "object"}),
            json!({"type": "object"}),
            ExecutionMode::Wasm {
                permissions: WasmPermissions::default(),
            },
        );

        // Test registration
        storage.register_tool(tool.clone()).await.unwrap();

        // Test retrieval
        let retrieved = storage.get_tool("http_request").await.unwrap();
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().name, "HTTP Request");

        // Test listing by category
        let http_tools = storage.list_tools(Some(ToolCategory::Http)).await.unwrap();
        assert_eq!(http_tools.len(), 1);

        let ai_tools = storage.list_tools(Some(ToolCategory::AI)).await.unwrap();
        assert_eq!(ai_tools.len(), 0);

        // Test update
        let mut updated_tool = storage.get_tool("http_request").await.unwrap().unwrap();
        updated_tool.description = "Updated description".to_string();
        storage.update_tool(updated_tool).await.unwrap();

        let retrieved = storage.get_tool("http_request").await.unwrap().unwrap();
        assert_eq!(retrieved.description, "Updated description");

        // Test deletion
        storage.delete_tool("http_request").await.unwrap();
        let retrieved = storage.get_tool("http_request").await.unwrap();
        assert!(retrieved.is_none());
    }

    #[tokio::test]
    async fn test_memory_storage_search() {
        let storage = MemoryStorage::new();

        // Create test flows
        let flow1 = Flow::new(
            "HTTP API Flow".to_string(),
            "Process HTTP requests".to_string(),
            "user1".to_string(),
        );
        let flow2 = Flow::new(
            "Database Flow".to_string(),
            "Handle database operations".to_string(),
            "user2".to_string(),
        );

        storage.create_flow(flow1).await.unwrap();
        storage.create_flow(flow2).await.unwrap();

        // Test flow search
        let results = storage.search_flows("HTTP").await.unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].name, "HTTP API Flow");

        let results = storage.search_flows("database").await.unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].name, "Database Flow");

        // Create test tools
        let tool1 = ToolDefinition::new(
            "http_get".to_string(),
            "HTTP GET".to_string(),
            "Perform HTTP GET requests".to_string(),
            ToolCategory::Http,
            json!({}),
            json!({}),
            ExecutionMode::Wasm {
                permissions: WasmPermissions::default(),
            },
        );

        storage.register_tool(tool1).await.unwrap();

        // Test tool search
        let results = storage.search_tools("HTTP").await.unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].name, "HTTP GET");
    }

    #[tokio::test]
    async fn test_memory_storage_filtering() {
        let storage = MemoryStorage::new();

        // Create test flows with different creators and tags
        let mut flow1 = Flow::new(
            "Flow 1".to_string(),
            "First flow".to_string(),
            "user1".to_string(),
        );
        flow1.tags = vec!["test".to_string(), "demo".to_string()];

        let mut flow2 = Flow::new(
            "Flow 2".to_string(),
            "Second flow".to_string(),
            "user2".to_string(),
        );
        flow2.tags = vec!["production".to_string()];

        storage.create_flow(flow1).await.unwrap();
        storage.create_flow(flow2).await.unwrap();

        // Test filtering by creator
        let filters = FlowFilters::default().created_by("user1".to_string());
        let results = storage.list_flows(filters).await.unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].name, "Flow 1");

        // Test filtering by tags
        let filters = FlowFilters::default().with_tags(vec!["test".to_string()]);
        let results = storage.list_flows(filters).await.unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].name, "Flow 1");

        // Test pagination
        let filters = FlowFilters::default().limit(1);
        let results = storage.list_flows(filters).await.unwrap();
        assert_eq!(results.len(), 1);
    }

    #[tokio::test]
    async fn test_memory_storage_versioning() {
        let storage = MemoryStorage::new();

        // Create base flow
        let flow = Flow::new(
            "Versioned Flow".to_string(),
            "A flow with versions".to_string(),
            "user1".to_string(),
        );
        let flow_id = flow.id;

        storage.create_flow(flow).await.unwrap();

        // Create version
        let mut v2_flow = storage.get_flow(&flow_id).await.unwrap().unwrap();
        v2_flow.version = "2.0.0".to_string();
        v2_flow.description = "Updated description".to_string();

        let version = storage.create_flow_version(&flow_id, v2_flow).await.unwrap();
        assert_eq!(version, "2.0.0");

        // Test version retrieval
        let retrieved = storage
            .get_flow_version(&flow_id, "2.0.0")
            .await
            .unwrap();
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().description, "Updated description");

        // Test version listing
        let versions = storage.list_flow_versions(&flow_id).await.unwrap();
        assert_eq!(versions.len(), 1);
        assert_eq!(versions[0], "2.0.0");
    }

    #[tokio::test]
    async fn test_memory_storage_health_check() {
        let storage = MemoryStorage::new();

        let health = storage.health_check().await.unwrap();
        assert!(health.healthy);
        assert_eq!(health.backend_type, "memory");
        assert_eq!(health.total_flows, 0);
        assert_eq!(health.total_tools, 0);

        // Add some data and check again
        let flow = Flow::new("Test".to_string(), "Test".to_string(), "user".to_string());
        storage.create_flow(flow).await.unwrap();

        let tool = ToolDefinition::new(
            "test".to_string(),
            "Test".to_string(),
            "Test".to_string(),
            ToolCategory::Custom,
            json!({}),
            json!({}),
            ExecutionMode::Wasm {
                permissions: WasmPermissions::default(),
            },
        );
        storage.register_tool(tool).await.unwrap();

        let health = storage.health_check().await.unwrap();
        assert_eq!(health.total_flows, 1);
        assert_eq!(health.total_tools, 1);
    }
}