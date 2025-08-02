// src/api/flows.rs

use axum::{
    extract::{Path, Query, State},
    http::StatusCode,
    response::Json,
    routing::{delete, get, post, put},
    Router,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use uuid::Uuid;

use crate::error::AceryxError;
use crate::storage::{Flow, FlowFilters, FlowId, FlowStorage};

type ApiResult<T> = Result<T, AceryxError>;

/// Create flow management routes
pub fn create_routes(storage: Arc<dyn FlowStorage>) -> Router {
    Router::new()
        .route("/", get(list_flows).post(create_flow))
        .route("/:id", get(get_flow).put(update_flow).delete(delete_flow))
        .route("/:id/versions", get(list_flow_versions).post(create_flow_version))
        .route("/:id/versions/:version", get(get_flow_version))
        .route("/search", get(search_flows))
        .with_state(storage)
}

// ============================================================================
// Request/Response Types
// ============================================================================

#[derive(Debug, Serialize, Deserialize)]
pub struct CreateFlowRequest {
    pub name: String,
    pub description: String,
    pub tags: Option<Vec<String>>,
    #[serde(default)]
    pub reactflow_data: serde_json::Value,
}

#[derive(Debug, Serialize)]
pub struct CreateFlowResponse {
    pub id: FlowId,
    pub message: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct UpdateFlowRequest {
    pub name: Option<String>,
    pub description: Option<String>,
    pub tags: Option<Vec<String>>,
    pub reactflow_data: Option<serde_json::Value>,
    pub nodes: Option<Vec<crate::storage::FlowNode>>,
    pub edges: Option<Vec<crate::storage::FlowEdge>>,
    pub variables: Option<std::collections::HashMap<String, serde_json::Value>>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct FlowListQuery {
    pub created_by: Option<String>,
    pub tags: Option<String>, // Comma-separated tags
    pub limit: Option<usize>,
    pub offset: Option<usize>,
}

impl From<FlowListQuery> for FlowFilters {
    fn from(query: FlowListQuery) -> Self {
        let tags = query
            .tags
            .map(|t| t.split(',').map(|s| s.trim().to_string()).collect())
            .unwrap_or_default();

        FlowFilters {
            created_by: query.created_by,
            tags,
            category: None,
            limit: query.limit,
            offset: query.offset,
        }
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SearchQuery {
    pub q: String,
    pub limit: Option<usize>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct CreateVersionRequest {
    pub version: String,
    pub description: Option<String>,
    pub changes: Option<String>,
}

// ============================================================================
// Handler Functions
// ============================================================================

/// List flows with optional filtering
async fn list_flows(
    Query(query): Query<FlowListQuery>,
    State(storage): State<Arc<dyn FlowStorage>>,
) -> ApiResult<Json<Vec<Flow>>> {
    let filters = FlowFilters::from(query);
    let flows = storage.list_flows(filters).await?;
    Ok(Json(flows))
}

/// Create a new flow
async fn create_flow(
    State(storage): State<Arc<dyn FlowStorage>>,
    Json(request): Json<CreateFlowRequest>,
) -> ApiResult<(StatusCode, Json<CreateFlowResponse>)> {
    // For now, we'll use a default user. In production, extract from auth context
    let created_by = "system".to_string(); // TODO: Extract from authentication

    let mut flow = Flow::new(request.name.clone(), request.description, created_by);

    if let Some(tags) = request.tags {
        flow.tags = tags;
    }

    flow.reactflow_data = request.reactflow_data;

    let flow_id = storage.create_flow(flow).await?;

    Ok((
        StatusCode::CREATED,
        Json(CreateFlowResponse {
            id: flow_id,
            message: format!("Flow '{}' created successfully", request.name),
        }),
    ))
}

/// Get a specific flow by ID
async fn get_flow(
    Path(id): Path<Uuid>,
    State(storage): State<Arc<dyn FlowStorage>>,
) -> ApiResult<Json<Flow>> {
    let flow = storage
        .get_flow(&id)
        .await?
        .ok_or_else(|| AceryxError::FlowNotFound { id: id.to_string() })?;

    Ok(Json(flow))
}

/// Update an existing flow
async fn update_flow(
    Path(id): Path<Uuid>,
    State(storage): State<Arc<dyn FlowStorage>>,
    Json(request): Json<UpdateFlowRequest>,
) -> ApiResult<Json<Flow>> {
    let mut flow = storage
        .get_flow(&id)
        .await?
        .ok_or_else(|| AceryxError::FlowNotFound { id: id.to_string() })?;

    // Apply updates
    if let Some(name) = request.name {
        flow.name = name;
    }
    if let Some(description) = request.description {
        flow.description = description;
    }
    if let Some(tags) = request.tags {
        flow.tags = tags;
    }
    if let Some(reactflow_data) = request.reactflow_data {
        flow.reactflow_data = reactflow_data;
    }
    if let Some(nodes) = request.nodes {
        flow.nodes = nodes;
    }
    if let Some(edges) = request.edges {
        flow.edges = edges;
    }
    if let Some(variables) = request.variables {
        flow.variables = variables;
    }

    storage.update_flow(flow.clone()).await?;
    Ok(Json(flow))
}

/// Delete a flow
async fn delete_flow(
    Path(id): Path<Uuid>,
    State(storage): State<Arc<dyn FlowStorage>>,
) -> ApiResult<StatusCode> {
    // Check if flow exists first
    storage
        .get_flow(&id)
        .await?
        .ok_or_else(|| AceryxError::FlowNotFound { id: id.to_string() })?;

    storage.delete_flow(&id).await?;
    Ok(StatusCode::NO_CONTENT)
}

/// Search flows
async fn search_flows(
    Query(query): Query<SearchQuery>,
    State(storage): State<Arc<dyn FlowStorage>>,
) -> ApiResult<Json<Vec<Flow>>> {
    let mut flows = storage.search_flows(&query.q).await?;

    // Apply limit if specified
    if let Some(limit) = query.limit {
        flows.truncate(limit);
    }

    Ok(Json(flows))
}

/// List versions for a flow
async fn list_flow_versions(
    Path(id): Path<Uuid>,
    State(storage): State<Arc<dyn FlowStorage>>,
) -> ApiResult<Json<Vec<String>>> {
    // Check if flow exists first
    storage
        .get_flow(&id)
        .await?
        .ok_or_else(|| AceryxError::FlowNotFound { id: id.to_string() })?;

    let versions = storage.list_flow_versions(&id).await?;
    Ok(Json(versions))
}

/// Create a new version of a flow
async fn create_flow_version(
    Path(id): Path<Uuid>,
    State(storage): State<Arc<dyn FlowStorage>>,
    Json(request): Json<CreateVersionRequest>,
) -> ApiResult<(StatusCode, Json<serde_json::Value>)> {
    let mut base_flow = storage
        .get_flow(&id)
        .await?
        .ok_or_else(|| AceryxError::FlowNotFound { id: id.to_string() })?;

    // Create new version
    base_flow.version = request.version.clone();
    if let Some(description) = request.description {
        base_flow.description = description;
    }

    let version = storage.create_flow_version(&id, base_flow).await?;

    Ok((
        StatusCode::CREATED,
        Json(serde_json::json!({
            "version": version,
            "message": "Flow version created successfully"
        })),
    ))
}

/// Get a specific version of a flow
async fn get_flow_version(
    Path((id, version)): Path<(Uuid, String)>,
    State(storage): State<Arc<dyn FlowStorage>>,
) -> ApiResult<Json<Flow>> {
    let flow = storage
        .get_flow_version(&id, &version)
        .await?
        .ok_or_else(|| AceryxError::FlowNotFound {
            id: format!("{}@{}", id, version)
        })?;

    Ok(Json(flow))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::storage::memory::MemoryStorage;
    use axum::body::Body;
    use axum::http::{Method, Request, StatusCode};
    use tower::ServiceExt;

    async fn create_test_app() -> Router {
        let storage = Arc::new(MemoryStorage::new());
        create_routes(storage)
    }

    #[tokio::test]
    async fn test_create_flow() {
        let app = create_test_app().await;

        let request_body = serde_json::json!({
            "name": "Test Flow",
            "description": "A test flow",
            "tags": ["test", "demo"]
        });

        let request = Request::builder()
            .method(Method::POST)
            .uri("/")
            .header("content-type", "application/json")
            .body(Body::from(serde_json::to_string(&request_body).unwrap()))
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::CREATED);
    }

    #[tokio::test]
    async fn test_list_flows() {
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
    async fn test_get_nonexistent_flow() {
        let app = create_test_app().await;

        let request = Request::builder()
            .method(Method::GET)
            .uri(&format!("/{}", Uuid::new_v4()))
            .body(Body::empty())
            .unwrap();

        let response = app.oneshot(request).await.unwrap();
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }
}