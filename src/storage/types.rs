// src/storage/types.rs

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use uuid::Uuid;

// ============================================================================
// Core Domain Types
// ============================================================================

/// Universal tool representation (protocol-agnostic)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolDefinition {
    pub id: String,
    pub name: String,
    pub description: String,
    pub category: ToolCategory,
    pub input_schema: serde_json::Value,  // JSON Schema
    pub output_schema: serde_json::Value, // JSON Schema
    pub execution_mode: ExecutionMode,
    pub metadata: HashMap<String, serde_json::Value>, // Protocol extensions
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl ToolDefinition {
    /// Create a new tool definition with current timestamps
    pub fn new(
        id: String,
        name: String,
        description: String,
        category: ToolCategory,
        input_schema: serde_json::Value,
        output_schema: serde_json::Value,
        execution_mode: ExecutionMode,
    ) -> Self {
        let now = Utc::now();
        Self {
            id,
            name,
            description,
            category,
            input_schema,
            output_schema,
            execution_mode,
            metadata: HashMap::new(),
            created_at: now,
            updated_at: now,
        }
    }

    /// Update the tool definition, setting updated_at to current time
    pub fn touch(&mut self) {
        self.updated_at = Utc::now();
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Hash)]
pub enum ToolCategory {
    AI,         // LLMs, ML models
    Http,       // REST APIs, webhooks
    Database,   // SQL, NoSQL queries
    Files,      // File operations, storage
    Messaging,  // Kafka, RabbitMQ, email
    Enterprise, // Pega, SAP, Salesforce
    Custom,     // User-defined tools
}

impl std::fmt::Display for ToolCategory {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ToolCategory::AI => write!(f, "AI"),
            ToolCategory::Http => write!(f, "HTTP"),
            ToolCategory::Database => write!(f, "Database"),
            ToolCategory::Files => write!(f, "Files"),
            ToolCategory::Messaging => write!(f, "Messaging"),
            ToolCategory::Enterprise => write!(f, "Enterprise"),
            ToolCategory::Custom => write!(f, "Custom"),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum ExecutionMode {
    Wasm { permissions: WasmPermissions },
    Container { image: String, resources: ResourceLimits },
    Process { runtime: String, sandbox: ProcessSandbox },
    Native { binary_path: String, permissions: NativePermissions },
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WasmPermissions {
    pub network_access: bool,
    pub filesystem_access: bool,
    pub environment_access: bool,
    pub max_memory_mb: u32,
}

impl Default for WasmPermissions {
    fn default() -> Self {
        Self {
            network_access: true,
            filesystem_access: false,
            environment_access: false,
            max_memory_mb: 64,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ResourceLimits {
    pub cpu_cores: f32,
    pub memory_mb: u32,
    pub timeout_seconds: u32,
}

impl Default for ResourceLimits {
    fn default() -> Self {
        Self {
            cpu_cores: 1.0,
            memory_mb: 512,
            timeout_seconds: 30,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProcessSandbox {
    pub allowed_paths: Vec<String>,
    pub network_isolation: bool,
    pub user_namespace: bool,
}

impl Default for ProcessSandbox {
    fn default() -> Self {
        Self {
            allowed_paths: vec!["/tmp".to_string()],
            network_isolation: true,
            user_namespace: true,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NativePermissions {
    pub read_paths: Vec<String>,
    pub write_paths: Vec<String>,
    pub network_domains: Vec<String>,
}

impl Default for NativePermissions {
    fn default() -> Self {
        Self {
            read_paths: vec![],
            write_paths: vec![],
            network_domains: vec![],
        }
    }
}

// ============================================================================
// Flow Types
// ============================================================================

pub type FlowId = Uuid;

/// Flow storage representation
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Flow {
    pub id: FlowId,
    pub name: String,
    pub description: String,
    pub version: String,                              // Semantic versioning
    pub tags: Vec<String>,                            // Categorization
    pub reactflow_data: serde_json::Value,            // Raw ReactFlow JSON for UI
    pub nodes: Vec<FlowNode>,                         // Enriched nodes for execution
    pub edges: Vec<FlowEdge>,                         // Flow connections
    pub variables: HashMap<String, serde_json::Value>, // Flow-level variables
    pub triggers: Vec<FlowTrigger>,                   // What starts this flow
    pub created_by: String,                           // User/team ownership
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

impl Flow {
    /// Create a new flow with generated ID and timestamps
    pub fn new(
        name: String,
        description: String,
        created_by: String,
    ) -> Self {
        let now = Utc::now();
        Self {
            id: Uuid::new_v4(),
            name,
            description,
            version: "1.0.0".to_string(),
            tags: Vec::new(),
            reactflow_data: serde_json::json!({}),
            nodes: Vec::new(),
            edges: Vec::new(),
            variables: HashMap::new(),
            triggers: Vec::new(),
            created_by,
            created_at: now,
            updated_at: now,
        }
    }

    /// Update the flow, incrementing version and setting updated_at
    pub fn touch(&mut self) {
        self.updated_at = Utc::now();
        // Simple version increment - in production, use proper semver
        let parts: Vec<&str> = self.version.split('.').collect();
        if parts.len() == 3 {
            if let Ok(patch) = parts[2].parse::<u32>() {
                self.version = format!("{}.{}.{}", parts[0], parts[1], patch + 1);
            }
        }
    }

    /// Validate flow configuration
    pub fn validate(&self) -> Result<(), String> {
        if self.name.trim().is_empty() {
            return Err("Flow name cannot be empty".to_string());
        }

        if self.created_by.trim().is_empty() {
            return Err("Flow must have a creator".to_string());
        }

        // Validate node references in edges
        let node_ids: std::collections::HashSet<String> =
            self.nodes.iter().map(|n| n.id.clone()).collect();

        for edge in &self.edges {
            if !node_ids.contains(&edge.source_node) {
                return Err(format!("Edge references unknown source node: {}", edge.source_node));
            }
            if !node_ids.contains(&edge.target_node) {
                return Err(format!("Edge references unknown target node: {}", edge.target_node));
            }
        }

        Ok(())
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlowNode {
    pub id: String,
    pub tool_id: String,                          // References ToolDefinition.id
    pub display_name: String,                     // Human-readable name
    pub config: serde_json::Value,                // Node-specific configuration
    pub position: Position,                       // For ReactFlow rendering
    pub retry_policy: Option<RetryPolicy>,        // Error handling
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FlowEdge {
    pub id: String,
    pub source_node: String,
    pub target_node: String,
    pub source_handle: Option<String>,           // Output port
    pub target_handle: Option<String>,           // Input port
    pub condition: Option<String>,               // Conditional routing
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum FlowTrigger {
    Manual,                                      // User-initiated
    Webhook { path: String },                    // HTTP trigger
    Schedule { cron: String },                   // Time-based
    FileWatch { path: String },                  // File system events
    ApiCall { endpoint: String },                // REST API trigger
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Position {
    pub x: f64,
    pub y: f64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RetryPolicy {
    pub max_attempts: u32,
    pub initial_delay_ms: u64,
    pub max_delay_ms: u64,
    pub backoff_multiplier: f64,
}

impl Default for RetryPolicy {
    fn default() -> Self {
        Self {
            max_attempts: 3,
            initial_delay_ms: 1000,
            max_delay_ms: 30000,
            backoff_multiplier: 2.0,
        }
    }
}

// ============================================================================
// Query and Filter Types
// ============================================================================

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct FlowFilters {
    pub created_by: Option<String>,
    pub tags: Vec<String>,
    pub category: Option<String>,
    pub limit: Option<usize>,
    pub offset: Option<usize>,
}

impl FlowFilters {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn created_by(mut self, created_by: String) -> Self {
        self.created_by = Some(created_by);
        self
    }

    pub fn with_tags(mut self, tags: Vec<String>) -> Self {
        self.tags = tags;
        self
    }

    pub fn limit(mut self, limit: usize) -> Self {
        self.limit = Some(limit);
        self
    }

    pub fn offset(mut self, offset: usize) -> Self {
        self.offset = Some(offset);
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn test_tool_definition_creation() {
        let tool = ToolDefinition::new(
            "test_tool".to_string(),
            "Test Tool".to_string(),
            "A test tool".to_string(),
            ToolCategory::Http,
            json!({"type": "object"}),
            json!({"type": "object"}),
            ExecutionMode::Wasm { permissions: WasmPermissions::default() },
        );

        assert_eq!(tool.id, "test_tool");
        assert_eq!(tool.category, ToolCategory::Http);
        assert!(tool.created_at <= tool.updated_at);
    }

    #[test]
    fn test_flow_creation() {
        let flow = Flow::new(
            "Test Flow".to_string(),
            "A test flow".to_string(),
            "test_user".to_string(),
        );

        assert_eq!(flow.name, "Test Flow");
        assert_eq!(flow.version, "1.0.0");
        assert_eq!(flow.created_by, "test_user");
        assert!(flow.created_at <= flow.updated_at);
    }

    #[test]
    fn test_flow_validation() {
        let mut flow = Flow::new(
            "Test Flow".to_string(),
            "A test flow".to_string(),
            "test_user".to_string(),
        );

        assert!(flow.validate().is_ok());

        // Test empty name
        flow.name = "".to_string();
        assert!(flow.validate().is_err());

        // Reset and test invalid edge reference
        flow.name = "Test Flow".to_string();
        flow.edges.push(FlowEdge {
            id: "edge1".to_string(),
            source_node: "nonexistent".to_string(),
            target_node: "also_nonexistent".to_string(),
            source_handle: None,
            target_handle: None,
            condition: None,
        });

        assert!(flow.validate().is_err());
    }

    #[test]
    fn test_tool_category_display() {
        assert_eq!(ToolCategory::AI.to_string(), "AI");
        assert_eq!(ToolCategory::Http.to_string(), "HTTP");
        assert_eq!(ToolCategory::Enterprise.to_string(), "Enterprise");
    }

    #[test]
    fn test_flow_filters() {
        let filters = FlowFilters::new()
            .created_by("user1".to_string())
            .with_tags(vec!["test".to_string(), "demo".to_string()])
            .limit(10);

        assert_eq!(filters.created_by, Some("user1".to_string()));
        assert_eq!(filters.tags.len(), 2);
        assert_eq!(filters.limit, Some(10));
    }

    #[test]
    fn test_tool_category_hash() {
        use std::collections::HashMap;

        let mut map = HashMap::new();
        map.insert(ToolCategory::AI, 5);
        map.insert(ToolCategory::Http, 10);

        assert_eq!(map.get(&ToolCategory::AI), Some(&5));
        assert_eq!(map.get(&ToolCategory::Http), Some(&10));
    }
}