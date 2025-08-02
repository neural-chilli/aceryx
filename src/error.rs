// src/error.rs

use axum::{
    http::StatusCode,
    response::{IntoResponse, Response},
    Json,
};
use serde_json::json;
use thiserror::Error;

/// Main error type for Aceryx application
#[derive(Error, Debug)]
pub enum AceryxError {
    #[error("Flow not found: {id}")]
    FlowNotFound { id: String },

    #[error("Tool not found: {id}")]
    ToolNotFound { id: String },

    #[error("Invalid flow configuration: {reason}")]
    InvalidFlow { reason: String },

    #[error("Tool execution failed: {tool_id}, reason: {reason}")]
    ToolExecutionFailed { tool_id: String, reason: String },

    #[error("Validation error: {message}")]
    ValidationError { message: String },

    #[error("Authentication required")]
    AuthenticationRequired,

    #[error("Access denied: {reason}")]
    AccessDenied { reason: String },

    #[error("Rate limit exceeded")]
    RateLimitExceeded,

    #[error("Storage error: {0}")]
    Storage(#[from] anyhow::Error),

    #[error("Serialization error: {0}")]
    Serialization(#[from] serde_json::Error),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("HTTP client error: {0}")]
    HttpClient(#[from] reqwest::Error),

    #[error("Internal server error: {message}")]
    Internal { message: String },
}

impl AceryxError {
    /// Create a validation error with a custom message
    pub fn validation(message: impl Into<String>) -> Self {
        Self::ValidationError {
            message: message.into(),
        }
    }

    /// Create an internal error with a custom message
    pub fn internal(message: impl Into<String>) -> Self {
        Self::Internal {
            message: message.into(),
        }
    }

    /// Get the HTTP status code for this error
    pub fn status_code(&self) -> StatusCode {
        match self {
            AceryxError::FlowNotFound { .. } | AceryxError::ToolNotFound { .. } => {
                StatusCode::NOT_FOUND
            }
            AceryxError::InvalidFlow { .. } | AceryxError::ValidationError { .. } => {
                StatusCode::BAD_REQUEST
            }
            AceryxError::AuthenticationRequired => StatusCode::UNAUTHORIZED,
            AceryxError::AccessDenied { .. } => StatusCode::FORBIDDEN,
            AceryxError::RateLimitExceeded => StatusCode::TOO_MANY_REQUESTS,
            AceryxError::ToolExecutionFailed { .. } => StatusCode::INTERNAL_SERVER_ERROR,
            AceryxError::Storage(_)
            | AceryxError::Serialization(_)
            | AceryxError::Io(_)
            | AceryxError::HttpClient(_)
            | AceryxError::Internal { .. } => StatusCode::INTERNAL_SERVER_ERROR,
        }
    }

    /// Get the error code for API responses
    pub fn error_code(&self) -> &'static str {
        match self {
            AceryxError::FlowNotFound { .. } => "FLOW_NOT_FOUND",
            AceryxError::ToolNotFound { .. } => "TOOL_NOT_FOUND",
            AceryxError::InvalidFlow { .. } => "INVALID_FLOW",
            AceryxError::ValidationError { .. } => "VALIDATION_ERROR",
            AceryxError::AuthenticationRequired => "AUTHENTICATION_REQUIRED",
            AceryxError::AccessDenied { .. } => "ACCESS_DENIED",
            AceryxError::RateLimitExceeded => "RATE_LIMIT_EXCEEDED",
            AceryxError::ToolExecutionFailed { .. } => "TOOL_EXECUTION_FAILED",
            AceryxError::Storage(_) => "STORAGE_ERROR",
            AceryxError::Serialization(_) => "SERIALIZATION_ERROR",
            AceryxError::Io(_) => "IO_ERROR",
            AceryxError::HttpClient(_) => "HTTP_CLIENT_ERROR",
            AceryxError::Internal { .. } => "INTERNAL_ERROR",
        }
    }

    /// Check if this error should be logged as a warning vs error
    pub fn is_client_error(&self) -> bool {
        matches!(
            self,
            AceryxError::FlowNotFound { .. }
                | AceryxError::ToolNotFound { .. }
                | AceryxError::InvalidFlow { .. }
                | AceryxError::ValidationError { .. }
                | AceryxError::AuthenticationRequired
                | AceryxError::AccessDenied { .. }
                | AceryxError::RateLimitExceeded
        )
    }
}

impl IntoResponse for AceryxError {
    fn into_response(self) -> Response {
        let status = self.status_code();
        let error_code = self.error_code();
        let message = self.to_string();

        // Log the error appropriately
        if self.is_client_error() {
            tracing::warn!("Client error: {} ({})", message, error_code);
        } else {
            tracing::error!("Server error: {} ({})", message, error_code);
        }

        let body = Json(json!({
            "error": {
                "code": error_code,
                "message": message,
                "status": status.as_u16()
            },
            "timestamp": chrono::Utc::now().to_rfc3339(),
            "request_id": uuid::Uuid::new_v4().to_string()
        }));

        (status, body).into_response()
    }
}

/// Convert anyhow errors to AceryxError
impl From<anyhow::Error> for AceryxError {
    fn from(err: anyhow::Error) -> Self {
        AceryxError::Storage(err)
    }
}

// src/api/middleware.rs

use axum::{
    body::Body,
    extract::Request,
    http::{HeaderMap, Method, StatusCode},
    middleware::Next,
    response::Response,
};
use std::time::Instant;
use tracing::{info, warn};
use uuid::Uuid;

/// Request logging middleware
pub async fn request_logging(request: Request, next: Next) -> Response {
    let start = Instant::now();
    let method = request.method().clone();
    let uri = request.uri().clone();
    let request_id = Uuid::new_v4();

    // Extract user agent and other relevant headers
    let headers = request.headers();
    let user_agent = headers
        .get("user-agent")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("unknown");

    // Add request ID to request extensions for use in handlers
    let mut request = request;
    request.extensions_mut().insert(request_id);

    info!(
        request_id = %request_id,
        method = %method,
        uri = %uri,
        user_agent = user_agent,
        "Request started"
    );

    let response = next.run(request).await;
    let duration = start.elapsed();
    let status = response.status();

    // Log the response
    let log_level = if status.is_client_error() || status.is_server_error() {
        tracing::Level::WARN
    } else {