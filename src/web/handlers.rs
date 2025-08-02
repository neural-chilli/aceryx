// src/web/handlers.rs - Fixed version

use std::sync::Arc;
use anyhow::Result;
use axum::{
    extract::{Path, Query, State},
    http::{header, HeaderMap, StatusCode},
    response::{Html, IntoResponse, Json, Response},
    routing::get,
    Router,
};
use serde::{Deserialize, Serialize};
use serde_json::json;
use tracing::{error, info};
use uuid::Uuid;

use super::static_assets::StaticAssets;
use super::templates::Templates;
use crate::error::AceryxError;
use crate::storage::{FlowStorage, FlowFilters, ToolCategory};
use crate::tools::ToolRegistry;

/// Application state containing storage and tool registry
#[derive(Clone)]
pub struct AppState {
    pub storage: Arc<dyn FlowStorage>,
    pub tool_registry: Arc<ToolRegistry>,
    pub templates: Templates,
}

/// Query parameters for flow listing
#[derive(Debug, Deserialize)]
pub struct FlowQueryParams {
    pub search: Option<String>,
    pub tags: Option<String>,
    pub user: Option<String>,
    pub limit: Option<usize>,
    pub offset: Option<usize>,
}

/// Query parameters for tool listing
#[derive(Debug, Deserialize)]
pub struct ToolQueryParams {
    pub category: Option<String>,
    pub search: Option<String>,
}

/// Create all web UI routes with enhanced handlers
pub fn create_routes(
    storage: Arc<dyn FlowStorage>,
    tool_registry: Arc<ToolRegistry>,
) -> Result<Router> {
    let templates = Templates::new()?;
    let state = AppState {
        storage,
        tool_registry,
        templates,
    };

    Ok(Router::new()
        // Dashboard and landing pages
        .route("/", get(dashboard_handler))
        .route("/dashboard", get(dashboard_handler))

        // Flow management routes
        .route("/flows", get(flows_list_handler))
        .route("/flows/new", get(flows_create_handler))
        .route("/flows/:id", get(flows_detail_handler))
        .route("/flows/:id/design", get(flows_design_handler))

        // Tool management routes
        .route("/tools", get(tools_registry_handler))
        .route("/tools/:id", get(tools_detail_handler))

        // System routes
        .route("/system", get(system_handler))
        .route("/health", get(health_handler))

        // Static assets
        .route("/static/*path", get(static_handler))

        // HTMX partial endpoints
        .route("/partials/flows", get(flows_partial_handler))
        .route("/partials/tools", get(tools_partial_handler))
        .route("/partials/flow-cards", get(flow_cards_partial_handler))
        .route("/partials/tool-grid", get(tool_grid_partial_handler))

        .with_state(state))
}

// ============================================================================
// Main Page Handlers
// ============================================================================

/// Dashboard/landing page handler
async fn dashboard_handler(
    headers: HeaderMap,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let health_info = gather_health_info(&state).await.unwrap_or_else(|_| {
        // Return a safe default if health check fails
        HealthInfo {
            storage: json!({
                "status": "unknown",
                "backend": "unknown",
                "flows": 0,
                "tools": 0
            }),
            tools: json!({
                "status": "unknown",
                "protocols": 0,
                "cached_tools": 0
            }),
            overall_status: "unknown".to_string(),
        }
    });

    let stats = gather_dashboard_stats(&state).await?;

    let context = json!({
        "title": "Aceryx Dashboard",
        "version": env!("CARGO_PKG_VERSION"),
        "health": health_info,
        "stats": stats,
        "features": [
            {
                "icon": "🎨",
                "title": "Visual Flow Designer",
                "description": "Drag-and-drop interface for building AI workflows",
                "count": stats.total_flows
            },
            {
                "icon": "🔗",
                "title": "Universal Tool Registry",
                "description": "Connect to any AI model or enterprise system",
                "count": stats.total_tools
            },
            {
                "icon": "⚡",
                "title": "High-Performance Execution",
                "description": "Rust-powered backend with enterprise-grade performance",
                "count": stats.active_protocols
            }
        ]
    });

    // Check if this is an HTMX request
    if is_htmx_request(&headers) {
        render_partial(&state.templates, "partials/dashboard_content.html", &context)
    } else {
        render_page(&state.templates, "pages/dashboard.html", &context)
    }
}

/// Flow listing page handler
async fn flows_list_handler(
    headers: HeaderMap,
    Query(params): Query<FlowQueryParams>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let filters = build_flow_filters(&params);
    let flows = state.storage.list_flows(filters).await
        .map_err(|e| AceryxError::internal(format!("Failed to list flows: {}", e)))?;

    let users = get_unique_users(&state).await?;
    let available_tags = get_unique_tags(&state).await?;

    let context = json!({
        "title": "Flows - Aceryx",
        "flows": flows,
        "users": users,
        "available_tags": available_tags,
        "current_search": params.search.unwrap_or_default(),
        "current_tags": params.tags.unwrap_or_default(),
        "current_user": params.user.unwrap_or_default(),
        "total_flows": flows.len()
    });

    if is_htmx_request(&headers) {
        render_partial(&state.templates, "partials/flow_cards.html", &context)
    } else {
        render_page(&state.templates, "pages/flows/list.html", &context)
    }
}

/// Flow creation form handler
async fn flows_create_handler(
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let available_tools = state.storage.list_tools(None).await
        .map_err(|e| AceryxError::internal(format!("Failed to list tools: {}", e)))?;

    let context = json!({
        "title": "Create Flow - Aceryx",
        "available_tools": available_tools,
        "templates": get_flow_templates().await?
    });

    render_page(&state.templates, "pages/flows/create.html", &context)
}

/// Individual flow details handler
async fn flows_detail_handler(
    Path(id): Path<Uuid>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let flow = state.storage.get_flow(&id).await
        .map_err(|e| AceryxError::internal(format!("Failed to get flow: {}", e)))?
        .ok_or_else(|| AceryxError::FlowNotFound { id: id.to_string() })?;

    let versions = state.storage.list_flow_versions(&id).await
        .map_err(|e| AceryxError::internal(format!("Failed to list versions: {}", e)))?;

    // Get execution history (would be implemented with real execution tracking)
    let execution_history = get_flow_execution_history(&id).await?;

    let context = json!({
        "title": format!("{} - Flow Details", flow.name),
        "flow": flow,
        "versions": versions,
        "execution_history": execution_history,
        "can_edit": true, // Would be based on user permissions
        "can_execute": true
    });

    render_page(&state.templates, "pages/flows/detail.html", &context)
}

/// Flow designer page (ReactFlow container)
async fn flows_design_handler(
    Path(id): Path<Uuid>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let flow = state.storage.get_flow(&id).await
        .map_err(|e| AceryxError::internal(format!("Failed to get flow: {}", e)))?
        .ok_or_else(|| AceryxError::FlowNotFound { id: id.to_string() })?;

    let available_tools = state.storage.list_tools(None).await
        .map_err(|e| AceryxError::internal(format!("Failed to list tools: {}", e)))?;

    let context = json!({
        "title": format!("Design {} - Aceryx", flow.name),
        "flow": flow,
        "available_tools": available_tools,
        "tool_categories": get_tool_categories(&available_tools),
        "api_base": "/api/v1"
    });

    render_page(&state.templates, "pages/flows/design.html", &context)
}

/// Tool registry page handler
async fn tools_registry_handler(
    headers: HeaderMap,
    Query(params): Query<ToolQueryParams>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let category = params.category.as_deref().and_then(|s| parse_tool_category(s).ok());
    let tools = state.storage.list_tools(category).await
        .map_err(|e| AceryxError::internal(format!("Failed to list tools: {}", e)))?;

    let filtered_tools = if let Some(search) = &params.search {
        if !search.trim().is_empty() {
            state.storage.search_tools(search).await
                .map_err(|e| AceryxError::internal(format!("Failed to search tools: {}", e)))?
        } else {
            tools
        }
    } else {
        tools
    };

    let tool_stats = calculate_tool_stats(&state).await?;
    let protocol_health = get_protocol_health(&state).await?;

    let context = json!({
        "title": "Tool Registry - Aceryx",
        "tools": filtered_tools,
        "tool_stats": tool_stats,
        "protocol_health": protocol_health,
        "current_category": params.category.unwrap_or_default(),
        "current_search": params.search.unwrap_or_default(),
        "categories": [
            {"id": "all", "name": "All Tools", "count": tool_stats.total_tools},
            {"id": "http", "name": "HTTP", "count": tool_stats.http_tools},
            {"id": "ai", "name": "AI", "count": tool_stats.ai_tools},
            {"id": "database", "name": "Database", "count": tool_stats.database_tools},
            {"id": "files", "name": "Files", "count": tool_stats.file_tools},
            {"id": "messaging", "name": "Messaging", "count": tool_stats.messaging_tools},
            {"id": "enterprise", "name": "Enterprise", "count": tool_stats.enterprise_tools},
            {"id": "custom", "name": "Custom", "count": tool_stats.custom_tools}
        ]
    });

    if is_htmx_request(&headers) {
        render_partial(&state.templates, "partials/tool_grid.html", &context)
    } else {
        render_page(&state.templates, "pages/tools/registry.html", &context)
    }
}

/// Individual tool details handler
async fn tools_detail_handler(
    Path(id): Path<String>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let tool = state.storage.get_tool(&id).await
        .map_err(|e| AceryxError::internal(format!("Failed to get tool: {}", e)))?
        .ok_or_else(|| AceryxError::ToolNotFound { id: id.clone() })?;

    let usage_stats = get_tool_usage_stats(&id).await?;
    let example_inputs = get_tool_examples(&tool).await?;

    let context = json!({
        "title": format!("{} - Tool Details", tool.name),
        "tool": tool,
        "usage_stats": usage_stats,
        "example_inputs": example_inputs,
        "can_execute": true
    });

    render_page(&state.templates, "pages/tools/detail.html", &context)
}

/// System overview handler
async fn system_handler(
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let health_info = gather_health_info(&state).await.unwrap_or_else(|_| {
        HealthInfo {
            storage: json!({"status": "unknown"}),
            tools: json!({"status": "unknown"}),
            overall_status: "unknown".to_string(),
        }
    });
    let system_info = gather_system_info(&state).await?;

    let context = json!({
        "title": "System Overview - Aceryx",
        "health": health_info,
        "system": system_info,
        "version": env!("CARGO_PKG_VERSION"),
        "features": get_enabled_features()
    });

    render_page(&state.templates, "pages/system/overview.html", &context)
}

// ============================================================================
// HTMX Partial Handlers
// ============================================================================

/// HTMX partial for flow listing
async fn flows_partial_handler(
    Query(params): Query<FlowQueryParams>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let filters = build_flow_filters(&params);
    let flows = state.storage.list_flows(filters).await
        .map_err(|e| AceryxError::internal(format!("Failed to list flows: {}", e)))?;

    let context = json!({
        "flows": flows
    });

    render_partial(&state.templates, "partials/flow_list.html", &context)
}

/// HTMX partial for tool listing
async fn tools_partial_handler(
    Query(params): Query<ToolQueryParams>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let category = params.category.as_deref().and_then(|s| parse_tool_category(s).ok());
    let tools = if let Some(search) = &params.search {
        state.storage.search_tools(search).await
    } else {
        state.storage.list_tools(category).await
    }.map_err(|e| AceryxError::internal(format!("Failed to list tools: {}", e)))?;

    let context = json!({
        "tools": tools
    });

    render_partial(&state.templates, "partials/tool_list.html", &context)
}

/// HTMX partial for flow cards
async fn flow_cards_partial_handler(
    Query(params): Query<FlowQueryParams>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let filters = build_flow_filters(&params);
    let flows = state.storage.list_flows(filters).await
        .map_err(|e| AceryxError::internal(format!("Failed to list flows: {}", e)))?;

    let context = json!({
        "flows": flows
    });

    render_partial(&state.templates, "components/flow_card.html", &context)
}

/// HTMX partial for tool grid
async fn tool_grid_partial_handler(
    Query(params): Query<ToolQueryParams>,
    State(state): State<AppState>,
) -> Result<impl IntoResponse, AceryxError> {
    let category = params.category.as_deref().and_then(|s| parse_tool_category(s).ok());
    let tools = if let Some(search) = &params.search {
        state.storage.search_tools(search).await
    } else {
        state.storage.list_tools(category).await
    }.map_err(|e| AceryxError::internal(format!("Failed to list tools: {}", e)))?;

    let context = json!({
        "tools": tools
    });

    render_partial(&state.templates, "components/tool_card.html", &context)
}

// ============================================================================
// Legacy Handlers for Backward Compatibility
// ============================================================================

/// Handle requests to the root path - redirect to dashboard
async fn root_handler() -> impl IntoResponse {
    axum::response::Redirect::to("/dashboard")
}

/// Health check endpoint with JSON response
async fn health_handler(State(state): State<AppState>) -> Json<serde_json::Value> {
    let health_info = gather_health_info(&state).await.unwrap_or_else(|_| {
        HealthInfo {
            storage: json!({"status": "unhealthy"}),
            tools: json!({"status": "unhealthy"}),
            overall_status: "unhealthy".to_string(),
        }
    });

    Json(json!({
        "status": "healthy",
        "service": "aceryx",
        "version": env!("CARGO_PKG_VERSION"),
        "timestamp": chrono::Utc::now().to_rfc3339(),
        "details": health_info
    }))
}

/// Serve static assets using rust-embed
async fn static_handler(
    axum::extract::Path(path): axum::extract::Path<String>,
) -> impl IntoResponse {
    match StaticAssets::get(&path) {
        Some(content) => {
            let mime_type = mime_guess::from_path(&path).first_or_octet_stream();

            Response::builder()
                .status(StatusCode::OK)
                .header(header::CONTENT_TYPE, mime_type.as_ref())
                .header(header::CACHE_CONTROL, "public, max-age=31536000") // 1 year cache
                .body(axum::body::Body::from(content.data))
                .unwrap()
        }
        None => Response::builder()
            .status(StatusCode::NOT_FOUND)
            .body(axum::body::Body::from("File not found"))
            .unwrap(),
    }
}

// ============================================================================
// Helper Functions
// ============================================================================

/// Check if request is from HTMX
fn is_htmx_request(headers: &HeaderMap) -> bool {
    headers.get("hx-request").is_some()
}

/// Render a full page template
fn render_page(
    templates: &Templates,
    template_name: &str,
    context: &serde_json::Value,
) -> Result<Html<String>, AceryxError> {
    match templates.render(template_name, context) {
        Ok(html) => Ok(Html(html)),
        Err(e) => {
            error!("Template rendering error for {}: {}", template_name, e);
            Err(AceryxError::internal(format!("Template error: {}", e)))
        }
    }
}

/// Render a partial template for HTMX
fn render_partial(
    templates: &Templates,
    template_name: &str,
    context: &serde_json::Value,
) -> Result<Html<String>, AceryxError> {
    match templates.render(template_name, context) {
        Ok(html) => Ok(Html(html)),
        Err(e) => {
            error!("Partial template rendering error for {}: {}", template_name, e);
            Err(AceryxError::internal(format!("Partial template error: {}", e)))
        }
    }
}

/// Build flow filters from query parameters
fn build_flow_filters(params: &FlowQueryParams) -> FlowFilters {
    let mut filters = FlowFilters::new();

    if let Some(user) = &params.user {
        if !user.trim().is_empty() {
            filters = filters.created_by(user.clone());
        }
    }

    if let Some(tags) = &params.tags {
        if !tags.trim().is_empty() {
            let tag_list: Vec<String> = tags
                .split(',')
                .map(|t| t.trim().to_string())
                .filter(|t| !t.is_empty())
                .collect();
            if !tag_list.is_empty() {
                filters = filters.with_tags(tag_list);
            }
        }
    }

    if let Some(limit) = params.limit {
        filters = filters.limit(limit);
    }

    if let Some(offset) = params.offset {
        filters = filters.offset(offset);
    }

    filters
}

/// Parse tool category string
fn parse_tool_category(category: &str) -> Result<ToolCategory, AceryxError> {
    match category.to_lowercase().as_str() {
        "ai" => Ok(ToolCategory::AI),
        "http" => Ok(ToolCategory::Http),
        "database" => Ok(ToolCategory::Database),
        "files" => Ok(ToolCategory::Files),
        "messaging" => Ok(ToolCategory::Messaging),
        "enterprise" => Ok(ToolCategory::Enterprise),
        "custom" => Ok(ToolCategory::Custom),
        _ => Err(AceryxError::validation(format!("Unknown tool category: {}", category))),
    }
}

// ============================================================================
// Data Gathering Functions
// ============================================================================

/// Health information for dashboard
#[derive(Serialize)]
struct HealthInfo {
    storage: serde_json::Value,
    tools: serde_json::Value,
    overall_status: String,
}

/// Dashboard statistics
#[derive(Serialize)]
struct DashboardStats {
    total_flows: u64,
    total_tools: u64,
    active_protocols: usize,
    recent_executions: u64,
}

/// Tool statistics
#[derive(Serialize)]
struct ToolStats {
    total_tools: usize,
    http_tools: usize,
    ai_tools: usize,
    database_tools: usize,
    file_tools: usize,
    messaging_tools: usize,
    enterprise_tools: usize,
    custom_tools: usize,
}

/// Gather comprehensive health information
async fn gather_health_info(state: &AppState) -> Result<HealthInfo, AceryxError> {
    let storage_health = match state.storage.health_check().await {
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

    let tools_health = match state.tool_registry.health_check().await {
        Ok(health) => {
            // Convert ProtocolHealth to serializable format
            let protocol_details: Vec<serde_json::Value> = health.protocols.iter().map(|p| {
                json!({
                    "protocol_name": p.protocol_name,
                    "healthy": p.healthy,
                    "error_message": p.error_message,
                    "tool_count": p.tool_count,
                    "last_refresh": p.last_refresh.to_rfc3339()
                })
            }).collect();

            json!({
                "status": if health.healthy { "healthy" } else { "unhealthy" },
                "protocols": health.protocols.len(),
                "cached_tools": health.cached_tools,
                "protocol_details": protocol_details
            })
        }
        Err(e) => {
            error!("Tool registry health check failed: {}", e);
            json!({
                "status": "unhealthy",
                "error": e.to_string()
            })
        }
    };

    let overall_status = if storage_health["status"] == "healthy" && tools_health["status"] == "healthy" {
        "healthy".to_string()
    } else {
        "unhealthy".to_string()
    };

    Ok(HealthInfo {
        storage: storage_health,
        tools: tools_health,
        overall_status,
    })
}

/// Gather dashboard statistics
async fn gather_dashboard_stats(state: &AppState) -> Result<DashboardStats, AceryxError> {
    let storage_health = state.storage.health_check().await
        .map_err(|e| AceryxError::internal(format!("Failed to get storage health: {}", e)))?;

    let registry_health = state.tool_registry.health_check().await
        .map_err(|e| AceryxError::internal(format!("Failed to get registry health: {}", e)))?;

    Ok(DashboardStats {
        total_flows: storage_health.total_flows,
        total_tools: storage_health.total_tools,
        active_protocols: registry_health.protocols.len(),
        recent_executions: 0, // Would be implemented with execution tracking
    })
}

/// Get unique users for filter dropdown
async fn get_unique_users(state: &AppState) -> Result<Vec<String>, AceryxError> {
    let flows = state.storage.list_flows(FlowFilters::default()).await
        .map_err(|e| AceryxError::internal(format!("Failed to list flows: {}", e)))?;

    let mut users: Vec<String> = flows
        .into_iter()
        .map(|f| f.created_by)
        .collect::<std::collections::HashSet<_>>()
        .into_iter()
        .collect();

    users.sort();
    Ok(users)
}

/// Get unique tags for filter dropdown
async fn get_unique_tags(state: &AppState) -> Result<Vec<String>, AceryxError> {
    let flows = state.storage.list_flows(FlowFilters::default()).await
        .map_err(|e| AceryxError::internal(format!("Failed to list flows: {}", e)))?;

    let mut tags: Vec<String> = flows
        .into_iter()
        .flat_map(|f| f.tags)
        .collect::<std::collections::HashSet<_>>()
        .into_iter()
        .collect();

    tags.sort();
    Ok(tags)
}

/// Calculate tool statistics by category
async fn calculate_tool_stats(state: &AppState) -> Result<ToolStats, AceryxError> {
    let tools = state.storage.list_tools(None).await
        .map_err(|e| AceryxError::internal(format!("Failed to list tools: {}", e)))?;

    let mut stats = ToolStats {
        total_tools: tools.len(),
        http_tools: 0,
        ai_tools: 0,
        database_tools: 0,
        file_tools: 0,
        messaging_tools: 0,
        enterprise_tools: 0,
        custom_tools: 0,
    };

    for tool in tools {
        match tool.category {
            ToolCategory::Http => stats.http_tools += 1,
            ToolCategory::AI => stats.ai_tools += 1,
            ToolCategory::Database => stats.database_tools += 1,
            ToolCategory::Files => stats.file_tools += 1,
            ToolCategory::Messaging => stats.messaging_tools += 1,
            ToolCategory::Enterprise => stats.enterprise_tools += 1,
            ToolCategory::Custom => stats.custom_tools += 1,
        }
    }

    Ok(stats)
}

/// Get protocol health information
async fn get_protocol_health(state: &AppState) -> Result<serde_json::Value, AceryxError> {
    let health = state.tool_registry.health_check().await
        .map_err(|e| AceryxError::internal(format!("Failed to get protocol health: {}", e)))?;

    // Convert ProtocolHealth to serializable format
    let protocol_details: Vec<serde_json::Value> = health.protocols.iter().map(|p| {
        json!({
            "protocol_name": p.protocol_name,
            "healthy": p.healthy,
            "error_message": p.error_message,
            "tool_count": p.tool_count,
            "last_refresh": p.last_refresh.to_rfc3339()
        })
    }).collect();

    Ok(json!({
        "protocols": protocol_details,
        "overall_healthy": health.healthy
    }))
}

/// Get tool categories for designer
fn get_tool_categories(tools: &[crate::storage::ToolDefinition]) -> serde_json::Value {
    let mut categories = std::collections::HashMap::new();

    for tool in tools {
        let category_name = tool.category.to_string();
        categories
            .entry(category_name)
            .or_insert_with(Vec::new)
            .push(&tool.id);
    }

    json!(categories)
}

/// Get flow templates for creation
async fn get_flow_templates() -> Result<serde_json::Value, AceryxError> {
    Ok(json!([
        {
            "id": "blank",
            "name": "Blank Flow",
            "description": "Start with an empty flow"
        },
        {
            "id": "http_api",
            "name": "HTTP API Integration",
            "description": "Template for integrating with REST APIs"
        },
        {
            "id": "data_processing",
            "name": "Data Processing Pipeline",
            "description": "Template for data transformation workflows"
        }
    ]))
}

/// Get flow execution history (placeholder)
async fn get_flow_execution_history(_id: &Uuid) -> Result<serde_json::Value, AceryxError> {
    Ok(json!([
        {
            "id": "exec-1",
            "started_at": chrono::Utc::now().to_rfc3339(),
            "status": "completed",
            "duration_ms": 1250
        }
    ]))
}

/// Get tool usage statistics (placeholder)
async fn get_tool_usage_stats(_id: &str) -> Result<serde_json::Value, AceryxError> {
    Ok(json!({
        "total_executions": 42,
        "success_rate": 0.95,
        "avg_duration_ms": 850
    }))
}

/// Get tool example inputs
async fn get_tool_examples(tool: &crate::storage::ToolDefinition) -> Result<serde_json::Value, AceryxError> {
    match tool.id.as_str() {
        "http_request" => Ok(json!([
            {
                "name": "Simple GET Request",
                "input": {
                    "url": "https://httpbin.org/get",
                    "method": "GET"
                }
            },
            {
                "name": "POST with JSON",
                "input": {
                    "url": "https://httpbin.org/post",
                    "method": "POST",
                    "headers": {"Content-Type": "application/json"},
                    "body": {"message": "Hello, World!"}
                }
            }
        ])),
        "json_transform" => Ok(json!([
            {
                "name": "Extract Property",
                "input": {
                    "data": {"user": {"name": "Alice", "age": 30}},
                    "operation": "extract",
                    "path": "user.name"
                }
            }
        ])),
        _ => Ok(json!([])),
    }
}

/// Gather system information
async fn gather_system_info(_state: &AppState) -> Result<serde_json::Value, AceryxError> {
    Ok(json!({
        "target_arch": std::env::consts::ARCH,
        "target_os": std::env::consts::OS,
        "memory_usage": get_memory_usage(),
        "uptime": get_uptime()
    }))
}

/// Get enabled features
fn get_enabled_features() -> serde_json::Value {
    json!([
        {
            "name": "Memory Storage",
            "enabled": true,
            "description": "In-memory storage backend"
        },
        {
            "name": "Redis Storage",
            "enabled": cfg!(feature = "redis-storage"),
            "description": "Redis storage backend"
        },
        {
            "name": "PostgreSQL Storage",
            "enabled": cfg!(feature = "postgres-storage"),
            "description": "PostgreSQL storage backend"
        },
        {
            "name": "AI Agents",
            "enabled": cfg!(feature = "ai-agents"),
            "description": "AI agent integration"
        }
    ])
}

/// Get memory usage (placeholder)
fn get_memory_usage() -> serde_json::Value {
    json!({
        "rss": "45.2 MB",
        "heap": "12.8 MB"
    })
}

/// Get uptime (placeholder)
fn get_uptime() -> String {
    "2h 34m".to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::memory::MemoryStorage;
    use crate::tools::{native::NativeProtocol, ToolRegistry};

    async fn create_test_state() -> AppState {
        let storage = Arc::new(MemoryStorage::new());
        let mut tool_registry = ToolRegistry::new(storage.clone());
        tool_registry.add_protocol(Box::new(NativeProtocol::new()));
        tool_registry.refresh_tools().await.unwrap();

        AppState {
            storage,
            tool_registry: Arc::new(tool_registry),
            templates: Templates::new().unwrap(),
        }
    }

    #[tokio::test]
    async fn test_gather_health_info() {
        let state = create_test_state().await;
        let health = gather_health_info(&state).await.unwrap();

        assert_eq!(health.overall_status, "healthy");
        assert_eq!(health.storage["status"], "healthy");
    }

    #[tokio::test]
    async fn test_build_flow_filters() {
        let params = FlowQueryParams {
            search: None,
            tags: Some("test,demo".to_string()),
            user: Some("alice".to_string()),
            limit: Some(10),
            offset: Some(5),
        };

        let filters = build_flow_filters(&params);
        assert_eq!(filters.created_by, Some("alice".to_string()));
        assert_eq!(filters.tags.len(), 2);
        assert_eq!(filters.limit, Some(10));
        assert_eq!(filters.offset, Some(5));
    }

    #[tokio::test]
    async fn test_calculate_tool_stats() {
        let state = create_test_state().await;
        let stats = calculate_tool_stats(&state).await.unwrap();

        assert!(stats.total_tools > 0);
        assert!(stats.http_tools > 0); // Native protocol includes HTTP tools
    }

    #[test]
    fn test_parse_tool_category() {
        assert!(matches!(parse_tool_category("http"), Ok(ToolCategory::Http)));
        assert!(matches!(parse_tool_category("AI"), Ok(ToolCategory::AI)));
        assert!(parse_tool_category("invalid").is_err());
    }

    #[test]
    fn test_is_htmx_request() {
        let mut headers = HeaderMap::new();
        assert!(!is_htmx_request(&headers));

        headers.insert("hx-request", "true".parse().unwrap());
        assert!(is_htmx_request(&headers));
    }
}