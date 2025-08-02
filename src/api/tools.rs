// src/api/tools.rs

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::Json,
    routing::{get, post},
    Router,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use std::time::Duration;
use uuid::Uuid;

use crate::error::AceryxError;
use crate::storage::ToolCategory;
use crate::tools::{ExecutionContext, ToolRegistry};

type ApiResult<T> = Result<T, AceryxError>;

/// Create tool-related routes
pub fn create_routes(registry: Arc<ToolRegistry>) -> Router {
    Router::new()
        .route("/", get(list_tools))
        .route("/:id", get(get_tool))
        .route("/categories", get(list_categories))
        .route("/refresh", post(refresh_tools))
        .route("/execute/:id", post(execute_tool))
        .route("/health", get(|| async { "OK" }))  // Simplified health endpoint
        .with_state(registry)
}

// ============================================================================
// Request/Response Types
// ============================================================================

#[derive(Debug, Serialize, Deserialize)]
pub struct ToolListQuery {
    pub category: Option<String>,
    pub search: Option<String>,
    pub limit: Option<usize>,
    pub offset: Option<usize>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ToolExecutionRequest {
    pub input: serde_json::Value,
    pub timeout: Option<u64>, // Timeout in seconds
    pub context: Option<ExecutionContextRequest>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ExecutionContextRequest {
    pub flow_id: Option<Uuid>,
    pub node_id: Option<String>,
    pub user_id: Option<String>,
    pub variables: Option<std::collections::HashMap<String, serde_json::Value>>,
}

#[derive(Debug, Serialize)]
pub struct ToolExecutionResponse {
    pub success: bool,
    pub result: Option<serde_json::Value>,
    pub error: Option<String>,
    pub duration_ms: u64,
    pub tool_id: String,
    pub request_id: String,
}

#[derive(Debug, Serialize)]
pub struct RefreshResponse {
    pub success: bool,
    pub tools_discovered: usize,
    pub message: String,
    pub protocols: Vec<ProtocolStatus>,
}

#[derive(Debug, Serialize)]
pub struct ProtocolStatus {
    pub name: String,
    pub healthy: bool,
    pub error: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct CategoryInfo {
    pub name: String,
    pub display_name: String,
    pub description: String,
    pub tool_count: usize,
}

// ============================================================================
// Handler Functions
// ============================================================================

/// List available tools with optional filtering
async fn list_tools(
    Query(query): Query<ToolListQuery>,
    State(registry): State<Arc<ToolRegistry>>,
) -> ApiResult<Json<Vec<crate::storage::ToolDefinition>>> {
    // Parse category filter
    let category = if let Some(cat_str) = query.category {
        Some(parse_tool_category(&cat_str)?)
    } else {
        None
    };

    // Get tools from storage (via registry's storage)
    let storage = &registry.storage;
    let mut tools = storage.list_tools(category).await?;

    // Apply search filter if provided
    if let Some(search_term) = query.search {
        if !search_term.trim().is_empty() {
            tools = storage.search_tools(&search_term).await?;
        }
    }

    // Apply pagination
    if let Some(offset) = query.offset {
        if offset < tools.len() {
            tools = tools.into_iter().skip(offset).collect();
        } else {
            tools = Vec::new();
        }
    }

    if let Some(limit) = query.limit {
        tools.truncate(limit);
    }

    Ok(Json(tools))
}

/// Get a specific tool by ID
async fn get_tool(
    Path(id): Path<String>,
    State(registry): State<Arc<ToolRegistry>>,
) -> ApiResult<Json<crate::storage::ToolDefinition>> {
    let storage = &registry.storage;
    let tool = storage
        .get_tool(&id)
        .await?
        .ok_or_else(|| AceryxError::ToolNotFound { id: id.clone() })?;

    Ok(Json(tool))
}

/// List all available tool categories with counts
async fn list_categories(
    State(registry): State<Arc<ToolRegistry>>,
) -> ApiResult<Json<Vec<CategoryInfo>>> {
    let storage = &registry.storage;
    let all_tools = storage.list_tools(None).await?;

    let mut category_counts = std::collections::HashMap::new();
    for tool in &all_tools {
        *category_counts.entry(&tool.category).or_insert(0) += 1;
    }

    let categories = vec![
        CategoryInfo {
            name: "ai".to_string(),
            display_name: "AI & Machine Learning".to_string(),
            description: "Large language models and AI services".to_string(),
            tool_count: *category_counts.get(&ToolCategory::AI).unwrap_or(&0),
        },
        CategoryInfo {
            name: "http".to_string(),
            display_name: "HTTP & APIs".to_string(),
            description: "REST APIs, webhooks, and web services".to_string(),
            tool_count: *category_counts.get(&ToolCategory::Http).unwrap_or(&0),
        },
        CategoryInfo {
            name: "database".to_string(),
            display_name: "Database".to_string(),
            description: "SQL and NoSQL database operations".to_string(),
            tool_count: *category_counts.get(&ToolCategory::Database).unwrap_or(&0),
        },
        CategoryInfo {
            name: "files".to_string(),
            display_name: "File Operations".to_string(),
            description: "File and storage system operations".to_string(),
            tool_count: *category_counts.get(&ToolCategory::Files).unwrap_or(&0),
        },
        CategoryInfo {
            name: "messaging".to_string(),
            display_name: "Messaging".to_string(),
            description: "Email, chat, and messaging services".to_string(),
            tool_count: *category_counts.get(&ToolCategory::Messaging).unwrap_or(&0),
        },
        CategoryInfo {
            name: "enterprise".to_string(),
            display_name: "Enterprise Systems".to_string(),
            description: "Pega, SAP, Salesforce, and other enterprise platforms".to_string(),
            tool_count: *category_counts.get(&ToolCategory::Enterprise).unwrap_or(&0),
        },
        CategoryInfo {
            name: "custom".to_string(),
            display_name: "Custom Tools".to_string(),
            description: "User-defined and custom tools".to_string(),
            tool_count: *category_counts.get(&ToolCategory::Custom).unwrap_or(&0),
        },
    ];

    Ok(Json(categories))
}

/// Refresh tools from all protocols
async fn refresh_tools(
    State(registry): State<Arc<ToolRegistry>>,
) -> ApiResult<(StatusCode, Json<RefreshResponse>)> {
    let start_time = std::time::Instant::now();

    // Check protocol health first
    let registry_health = registry.health_check().await?;
    let protocol_statuses: Vec<ProtocolStatus> = registry_health
        .protocols
        .iter()
        .map(|p| ProtocolStatus {
            name: p.protocol_name.clone(),
            healthy: p.healthy,
            error: p.error_message.clone(),
        })
        .collect();

    // Refresh tools
    let tools_discovered = match registry.refresh_tools().await {
        Ok(count) => count,
        Err(e) => {
            return Ok((
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(RefreshResponse {
                    success: false,
                    tools_discovered: 0,
                    message: format!("Tool refresh failed: {}", e),
                    protocols: protocol_statuses,
                }),
            ));
        }
    };

    let duration = start_time.elapsed();

    Ok((
        StatusCode::OK,
        Json(RefreshResponse {
            success: true,
            tools_discovered,
            message: format!(
                "Successfully discovered {} tools in {}ms",
                tools_discovered,
                duration.as_millis()
            ),
            protocols: protocol_statuses,
        }),
    ))
}

/// Execute a tool with provided input
async fn execute_tool(
    Path(tool_id): Path<String>,
    State(registry): State<Arc<ToolRegistry>>,
    Json(request): Json<ToolExecutionRequest>,
) -> ApiResult<Json<ToolExecutionResponse>> {
    let start_time = std::time::Instant::now();
    let request_id = Uuid::new_v4();

    // Build execution context
    let mut context = ExecutionContext::new(
        request
            .context
            .as_ref()
            .and_then(|c| c.user_id.clone())
            .unwrap_or_else(|| "anonymous".to_string()),
    );

    // Set timeout
    if let Some(timeout_secs) = request.timeout {
        context = context.with_timeout(Duration::from_secs(timeout_secs));
    }

    // Set flow context if provided
    if let Some(ctx_req) = &request.context {
        if let Some(flow_id) = ctx_req.flow_id {
            context = context.with_flow(flow_id, ctx_req.node_id.clone());
        }
        if let Some(variables) = &ctx_req.variables {
            context = context.with_variables(variables.clone());
        }
    }

    context.request_id = request_id;

    // Execute the tool
    let execution_result = registry
        .execute_tool(&tool_id, request.input, context)
        .await;

    let duration = start_time.elapsed();

    let response = match execution_result {
        Ok(result) => ToolExecutionResponse {
            success: true,
            result: Some(result),
            error: None,
            duration_ms: duration.as_millis() as u64,
            tool_id: tool_id.clone(),
            request_id: request_id.to_string(),
        },
        Err(e) => ToolExecutionResponse {
            success: false,
            result: None,
            error: Some(e.to_string()),
            duration_ms: duration.as_millis() as u64,
            tool_id: tool_id.clone(),
            request_id: request_id.to_string(),
        },
    };

    // Log execution for monitoring
    if response.success {
        tracing::info!(
            "Tool execution successful: {} in {}ms",
            tool_id,
            response.duration_ms
        );
    } else {
        tracing::warn!(
            "Tool execution failed: {} in {}ms - {}",
            tool_id,
            response.duration_ms,
            response.error.as_ref().unwrap_or(&"Unknown error".to_string())
        );
    }

    Ok(Json(response))
}

// Remove the problematic registry_health handler for now
// TODO: Implement proper health endpoint that doesn't conflict with Handler trait

// ============================================================================
// Helper Functions
// ============================================================================

/// Parse a tool category string into ToolCategory enum
fn parse_tool_category(category: &str) -> Result<ToolCategory, AceryxError> {
    match category.to_lowercase().as_str() {
        "ai" => Ok(ToolCategory::AI),
        "http" => Ok(ToolCategory::Http),
        "database" => Ok(ToolCategory::Database),
        "files" => Ok(ToolCategory::Files),
        "messaging" => Ok(ToolCategory::Messaging),
        "enterprise" => Ok(ToolCategory::Enterprise),
        "custom" => Ok(ToolCategory::Custom),
        _ => Err(AceryxError::InvalidFlow {
            reason: format!("Unknown tool category: {}", category),
        }),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::memory::MemoryStorage;
    use crate::tools::native::NativeProtocol;
    use crate::tools::ToolRegistry;
    use axum::body::Body;
    use axum::http::{Method, Request, StatusCode};
    use tower::ServiceExt;

    async fn create_test_app() -> Router {
        let storage = Arc::new(MemoryStorage::new());
        let mut registry = ToolRegistry::new(storage);
        registry.add_protocol(Box::new(NativeProtocol::new()));
        registry.refresh_tools().await.unwrap();

        create_routes(Arc::new(registry))
    }

    #[tokio::test]
    async fn test_list_tools() {
        let app = create_test_app().await;

        let request = Request::builder()
            .method(Method::GET)
            .uri("/")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_get_tool() {
        let app = create_test_app().await;

        let request = Request::builder()
            .method(Method::GET)
            .uri("/http_request")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_get_nonexistent_tool() {
        let app = create_test_app().await;

        let request = Request::builder()
            .method(Method::GET)
            .uri("/nonexistent_tool")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }

    #[tokio::test]
    async fn test_list_categories() {
        let app = create_test_app().await;

        let request = Request::builder()
            .method(Method::GET)
            .uri("/categories")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_execute_tool() {
        let app = create_test_app().await;

        let request_body = serde_json::json!({
            "input": {
                "data": {"test": "value"},
                "operation": "validate"
            },
            "timeout": 30
        });

        let request = Request::builder()
            .method(Method::POST)
            .uri("/execute/json_transform")
            .header("content-type", "application/json")
            .body(Body::from(serde_json::to_string(&request_body).unwrap()))
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn test_refresh_tools() {
        let app = create_test_app().await;

        let request = Request::builder()
            .method(Method::POST)
            .uri("/refresh")
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[test]
    fn test_parse_tool_category() {
        assert!(matches!(parse_tool_category("ai"), Ok(ToolCategory::AI)));
        assert!(matches!(parse_tool_category("HTTP"), Ok(ToolCategory::Http)));
        assert!(matches!(parse_tool_category("Database"), Ok(ToolCategory::Database)));
        assert!(parse_tool_category("invalid").is_err());
    }
}