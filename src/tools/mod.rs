// src/tools/mod.rs

use anyhow::Result;
use async_trait::async_trait;
use serde_json::Value;
use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::RwLock;
use uuid::Uuid;

pub mod native;
mod native;

use crate::storage::{FlowId, FlowStorage, ToolDefinition};

/// Universal tool execution interface
///
/// All tools, regardless of their implementation (WASM, Container, Process, Native),
/// must implement this trait to participate in the Aceryx execution fabric.
#[async_trait]
pub trait Tool: Send + Sync {
    /// Execute the tool with given input and execution context
    async fn execute(&self, input: Value, context: ExecutionContext) -> Result<Value>;

    /// Get the tool's definition/metadata
    fn definition(&self) -> &ToolDefinition;

    /// Validate input against the tool's input schema
    fn validate_input(&self, input: &Value) -> Result<()>;

    /// Optional cleanup when tool is no longer needed
    async fn cleanup(&self) -> Result<()> {
        Ok(())
    }
}

/// Tool protocol interface for discovering and creating tools
///
/// Protocols represent different ways of discovering and instantiating tools:
/// - Native: Built-in Rust tools
/// - MCP: Model Context Protocol tools
/// - OpenAI: OpenAI function calling tools
/// - Custom: User-defined protocol implementations
#[async_trait]
pub trait ToolProtocol: Send + Sync {
    /// Get the protocol identifier (e.g., "native", "mcp", "openai")
    fn protocol_name(&self) -> &'static str;

    /// Discover available tools from this protocol
    async fn discover_tools(&self) -> Result<Vec<ToolDefinition>>;

    /// Create a tool instance from its definition
    async fn create_tool(&self, definition: &ToolDefinition) -> Result<Box<dyn Tool>>;

    /// Check if the protocol is healthy and responsive
    async fn health_check(&self) -> Result<ProtocolHealth>;

    /// Optional: Refresh/reload tools from the protocol source
    async fn refresh(&self) -> Result<()> {
        Ok(())
    }
}

/// Universal tool registry that manages all available tools
///
/// The registry serves as the central hub for tool discovery, caching,
/// and execution across all protocols.
pub struct ToolRegistry {
    protocols: Vec<Box<dyn ToolProtocol>>,
    storage: Arc<dyn FlowStorage>,
    tool_cache: Arc<RwLock<HashMap<String, Arc<dyn Tool>>>>,
}

impl ToolRegistry {
    /// Create a new tool registry with the given storage backend
    pub fn new(storage: Arc<dyn FlowStorage>) -> Self {
        Self {
            protocols: Vec::new(),
            storage,
            tool_cache: Arc::new(RwLock::new(HashMap::new())),
        }
    }

    /// Add a protocol to the registry
    pub fn add_protocol(&mut self, protocol: Box<dyn ToolProtocol>) {
        tracing::info!("Adding protocol: {}", protocol.protocol_name());
        self.protocols.push(protocol);
    }

    /// Refresh tools from all protocols and update storage
    pub async fn refresh_tools(&self) -> Result<usize> {
        let mut total_discovered = 0;

        for protocol in &self.protocols {
            tracing::info!("Refreshing tools from protocol: {}", protocol.protocol_name());

            match protocol.discover_tools().await {
                Ok(tools) => {
                    for tool in tools {
                        match self.storage.register_tool(tool.clone()).await {
                            Ok(()) => {
                                tracing::debug!("Registered tool: {}", tool.id);
                                total_discovered += 1;
                            }
                            Err(e) => {
                                // Tool might already exist, try updating instead
                                if let Err(update_err) = self.storage.update_tool(tool.clone()).await {
                                    tracing::warn!(
                                        "Failed to register/update tool {}: {} (update error: {})",
                                        tool.id, e, update_err
                                    );
                                } else {
                                    tracing::debug!("Updated existing tool: {}", tool.id);
                                }
                            }
                        }
                    }
                }
                Err(e) => {
                    tracing::error!("Failed to discover tools from {}: {}", protocol.protocol_name(), e);
                }
            }
        }

        // Clear tool cache to force reload
        self.tool_cache.write().await.clear();

        tracing::info!("Tool refresh complete. Discovered {} tools", total_discovered);
        Ok(total_discovered)
    }

    /// Get a tool by ID, creating and caching it if necessary
    pub async fn get_tool(&self, id: &str) -> Result<Option<Arc<dyn Tool>>> {
        // Check cache first
        {
            let cache = self.tool_cache.read().await;
            if let Some(tool) = cache.get(id) {
                return Ok(Some(tool.clone()));
            }
        }

        // Tool not in cache, try to load from storage
        let tool_def = match self.storage.get_tool(id).await? {
            Some(def) => def,
            None => return Ok(None),
        };

        // Find the right protocol to create the tool
        for protocol in &self.protocols {
            match protocol.create_tool(&tool_def).await {
                Ok(tool) => {
                    let tool_arc = Arc::from(tool);

                    // Cache the tool
                    let mut cache = self.tool_cache.write().await;
                    cache.insert(id.to_string(), tool_arc.clone());

                    return Ok(Some(tool_arc));
                }
                Err(_) => {
                    // This protocol can't create this tool, try the next one
                    continue;
                }
            }
        }

        tracing::warn!("No protocol could create tool: {}", id);
        Ok(None)
    }

    /// Execute a tool with the given input and context
    pub async fn execute_tool(
        &self,
        id: &str,
        input: Value,
        context: ExecutionContext,
    ) -> Result<Value> {
        let tool = self
            .get_tool(id)
            .await?
            .ok_or_else(|| anyhow::anyhow!("Tool not found: {}", id))?;

        // Validate input
        tool.validate_input(&input)
            .map_err(|e| anyhow::anyhow!("Input validation failed for tool {}: {}", id, e))?;

        // Execute with timeout
        let execution_future = tool.execute(input, context);

        match tokio::time::timeout(Duration::from_secs(30), execution_future).await {
            Ok(result) => result,
            Err(_) => Err(anyhow::anyhow!("Tool execution timed out: {}", id)),
        }
    }

    /// Get all available protocols
    pub fn protocols(&self) -> &[Box<dyn ToolProtocol>] {
        &self.protocols
    }

    /// Check health of all protocols
    pub async fn health_check(&self) -> Result<RegistryHealth> {
        let mut protocol_healths = Vec::new();

        for protocol in &self.protocols {
            match protocol.health_check().await {
                Ok(health) => protocol_healths.push(health),
                Err(e) => {
                    protocol_healths.push(ProtocolHealth {
                        protocol_name: protocol.protocol_name().to_string(),
                        healthy: false,
                        error_message: Some(e.to_string()),
                        tool_count: 0,
                        last_refresh: chrono::Utc::now(),
                    });
                }
            }
        }

        let cache = self.tool_cache.read().await;
        let cached_tools = cache.len();

        Ok(RegistryHealth {
            healthy: protocol_healths.iter().all(|h| h.healthy),
            protocols: protocol_healths,
            cached_tools,
            last_check: chrono::Utc::now(),
        })
    }
}

/// Execution context for tool runs
#[derive(Debug, Clone)]
pub struct ExecutionContext {
    pub flow_id: Option<FlowId>,
    pub node_id: Option<String>,
    pub user_id: String,
    pub request_id: Uuid,
    pub timeout: Duration,
    pub variables: HashMap<String, Value>,
}

impl ExecutionContext {
    /// Create a new execution context with defaults
    pub fn new(user_id: String) -> Self {
        Self {
            flow_id: None,
            node_id: None,
            user_id,
            request_id: Uuid::new_v4(),
            timeout: Duration::from_secs(30),
            variables: HashMap::new(),
        }
    }

    /// Set flow context
    pub fn with_flow(mut self, flow_id: FlowId, node_id: Option<String>) -> Self {
        self.flow_id = Some(flow_id);
        self.node_id = node_id;
        self
    }

    /// Set timeout
    pub fn with_timeout(mut self, timeout: Duration) -> Self {
        self.timeout = timeout;
        self
    }

    /// Add variables
    pub fn with_variables(mut self, variables: HashMap<String, Value>) -> Self {
        self.variables = variables;
        self
    }

    /// Get a variable by key
    pub fn get_variable(&self, key: &str) -> Option<&Value> {
        self.variables.get(key)
    }

    /// Set a variable
    pub fn set_variable(&mut self, key: String, value: Value) {
        self.variables.insert(key, value);
    }
}

/// Health information for a tool protocol
#[derive(Debug, Clone)]
pub struct ProtocolHealth {
    pub protocol_name: String,
    pub healthy: bool,
    pub error_message: Option<String>,
    pub tool_count: usize,
    pub last_refresh: chrono::DateTime<chrono::Utc>,
}

/// Health information for the entire tool registry
#[derive(Debug, Clone)]
pub struct RegistryHealth {
    pub healthy: bool,
    pub protocols: Vec<ProtocolHealth>,
    pub cached_tools: usize,
    pub last_check: chrono::DateTime<chrono::Utc>,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::{memory::MemoryStorage, ToolCategory, ExecutionMode, WasmPermissions};
    use serde_json::json;

    // Mock tool for testing
    struct MockTool {
        definition: ToolDefinition,
    }

    #[async_trait]
    impl Tool for MockTool {
        async fn execute(&self, input: Value, _context: ExecutionContext) -> Result<Value> {
            Ok(json!({
                "status": "success",
                "input_received": input
            }))
        }

        fn definition(&self) -> &ToolDefinition {
            &self.definition
        }

        fn validate_input(&self, _input: &Value) -> Result<()> {
            Ok(())
        }
    }

    // Mock protocol for testing
    struct MockProtocol {
        tools: Vec<ToolDefinition>,
    }

    impl MockProtocol {
        fn new() -> Self {
            Self {
                tools: vec![
                    ToolDefinition::new(
                        "mock_tool_1".to_string(),
                        "Mock Tool 1".to_string(),
                        "First mock tool".to_string(),
                        ToolCategory::Custom,
                        json!({"type": "object"}),
                        json!({"type": "object"}),
                        ExecutionMode::Wasm { permissions: WasmPermissions::default() },
                    ),
                    ToolDefinition::new(
                        "mock_tool_2".to_string(),
                        "Mock Tool 2".to_string(),
                        "Second mock tool".to_string(),
                        ToolCategory::Custom,
                        json!({"type": "object"}),
                        json!({"type": "object"}),
                        ExecutionMode::Wasm { permissions: WasmPermissions::default() },
                    ),
                ],
            }
        }
    }

    #[async_trait]
    impl ToolProtocol for MockProtocol {
        fn protocol_name(&self) -> &'static str {
            "mock"
        }

        async fn discover_tools(&self) -> Result<Vec<ToolDefinition>> {
            Ok(self.tools.clone())
        }

        async fn create_tool(&self, definition: &ToolDefinition) -> Result<Box<dyn Tool>> {
            if self.tools.iter().any(|t| t.id == definition.id) {
                Ok(Box::new(MockTool {
                    definition: definition.clone(),
                }))
            } else {
                Err(anyhow::anyhow!("Tool not found in mock protocol"))
            }
        }

        async fn health_check(&self) -> Result<ProtocolHealth> {
            Ok(ProtocolHealth {
                protocol_name: "mock".to_string(),
                healthy: true,
                error_message: None,
                tool_count: self.tools.len(),
                last_refresh: chrono::Utc::now(),
            })
        }
    }

    #[tokio::test]
    async fn test_tool_registry_refresh() {
        let storage = Arc::new(MemoryStorage::new());
        let mut registry = ToolRegistry::new(storage.clone());

        registry.add_protocol(Box::new(MockProtocol::new()));

        let discovered = registry.refresh_tools().await.unwrap();
        assert_eq!(discovered, 2);

        // Check that tools were stored
        let tools = storage.list_tools(None).await.unwrap();
        assert_eq!(tools.len(), 2);
    }

    #[tokio::test]
    async fn test_tool_registry_get_tool() {
        let storage = Arc::new(MemoryStorage::new());
        let mut registry = ToolRegistry::new(storage.clone());

        registry.add_protocol(Box::new(MockProtocol::new()));
        registry.refresh_tools().await.unwrap();

        // Test getting existing tool
        let tool = registry.get_tool("mock_tool_1").await.unwrap();
        assert!(tool.is_some());

        // Test getting non-existent tool
        let tool = registry.get_tool("nonexistent").await.unwrap();
        assert!(tool.is_none());
    }

    #[tokio::test]
    async fn test_tool_registry_execute() {
        let storage = Arc::new(MemoryStorage::new());
        let mut registry = ToolRegistry::new(storage.clone());

        registry.add_protocol(Box::new(MockProtocol::new()));
        registry.refresh_tools().await.unwrap();

        let context = ExecutionContext::new("test_user".to_string());
        let input = json!({"test": "data"});

        let result = registry.execute_tool("mock_tool_1", input.clone(), context).await.unwrap();

        assert_eq!(result["status"], "success");
        assert_eq!(result["input_received"], input);
    }

    #[tokio::test]
    async fn test_execution_context() {
        let mut context = ExecutionContext::new("test_user".to_string())
            .with_flow(Uuid::new_v4(), Some("node_1".to_string()))
            .with_timeout(Duration::from_secs(60))
            .with_variables({
                let mut vars = HashMap::new();
                vars.insert("key1".to_string(), json!("value1"));
                vars
            });

        assert_eq!(context.user_id, "test_user");
        assert!(context.flow_id.is_some());
        assert_eq!(context.node_id, Some("node_1".to_string()));
        assert_eq!(context.timeout, Duration::from_secs(60));
        assert_eq!(context.get_variable("key1"), Some(&json!("value1")));

        context.set_variable("key2".to_string(), json!("value2"));
        assert_eq!(context.get_variable("key2"), Some(&json!("value2")));
    }

    #[tokio::test]
    async fn test_tool_registry_health_check() {
        let storage = Arc::new(MemoryStorage::new());
        let mut registry = ToolRegistry::new(storage.clone());

        registry.add_protocol(Box::new(MockProtocol::new()));

        let health = registry.health_check().await.unwrap();
        assert!(health.healthy);
        assert_eq!(health.protocols.len(), 1);
        assert_eq!(health.protocols[0].protocol_name, "mock");
        assert!(health.protocols[0].healthy);
    }
}