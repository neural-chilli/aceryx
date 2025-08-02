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

    #[error("Serialization error: {0}")]
    Serialization(#[from] serde_json::Error),

    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),

    #[error("HTTP client error: {0}")]
    HttpClient(#[from] reqwest::Error),

    #[error("Storage error: {0}")]
    StorageError(String),

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
            AceryxError::StorageError(_)
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
            AceryxError::StorageError(_) => "STORAGE_ERROR",
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
        AceryxError::StorageError(err.to_string())
    }
}

// Middleware functions

use axum::{
    extract::Request,
    middleware::Next,
};
use std::time::Instant;
use uuid::Uuid;

/// Request logging middleware
pub async fn request_logging(request: Request, next: Next) -> Response {
    let start = Instant::now();
    let method = request.method().clone();
    let uri = request.uri().clone();
    let request_id = Uuid::new_v4();

    // Extract user agent from headers before we modify the request
    let user_agent = request
        .headers()
        .get("user-agent")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("unknown")
        .to_string();

    // Convert to mut after we've extracted what we need
    let mut request = request;

    // Add request ID to request extensions for use in handlers
    request.extensions_mut().insert(request_id);

    tracing::info!(
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
    if status.is_client_error() || status.is_server_error() {
        tracing::warn!(
            request_id = %request_id,
            method = %method,
            uri = %uri,
            status = %status,
            duration_ms = duration.as_millis(),
            "Request completed with error"
        );
    } else {
        tracing::info!(
            request_id = %request_id,
            method = %method,
            uri = %uri,
            status = %status,
            duration_ms = duration.as_millis(),
            "Request completed successfully"
        );
    }

    response
}

/// Error handling middleware
pub async fn error_handling(request: Request, next: Next) -> Response {
    let response = next.run(request).await;

    // If the response is already an error, pass it through
    if response.status().is_client_error() || response.status().is_server_error() {
        return response;
    }

    response
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_aceryx_error_status_codes() {
        assert_eq!(
            AceryxError::FlowNotFound {
                id: "test".to_string()
            }
                .status_code(),
            StatusCode::NOT_FOUND
        );

        assert_eq!(
            AceryxError::ValidationError {
                message: "test".to_string()
            }
                .status_code(),
            StatusCode::BAD_REQUEST
        );

        assert_eq!(
            AceryxError::AuthenticationRequired.status_code(),
            StatusCode::UNAUTHORIZED
        );

        assert_eq!(
            AceryxError::RateLimitExceeded.status_code(),
            StatusCode::TOO_MANY_REQUESTS
        );
    }

    #[test]
    fn test_aceryx_error_codes() {
        assert_eq!(
            AceryxError::FlowNotFound {
                id: "test".to_string()
            }
                .error_code(),
            "FLOW_NOT_FOUND"
        );

        assert_eq!(
            AceryxError::ToolNotFound {
                id: "test".to_string()
            }
                .error_code(),
            "TOOL_NOT_FOUND"
        );

        assert_eq!(
            AceryxError::ValidationError {
                message: "test".to_string()
            }
                .error_code(),
            "VALIDATION_ERROR"
        );
    }

    #[test]
    fn test_client_error_classification() {
        let client_error = AceryxError::FlowNotFound {
            id: "test".to_string(),
        };
        assert!(client_error.is_client_error());

        let server_error = AceryxError::Internal {
            message: "test".to_string(),
        };
        assert!(!server_error.is_client_error());
    }

    #[test]
    fn test_error_helper_methods() {
        let validation_error = AceryxError::validation("test message");
        assert!(matches!(validation_error, AceryxError::ValidationError { .. }));

        let internal_error = AceryxError::internal("test message");
        assert!(matches!(internal_error, AceryxError::Internal { .. }));
    }

    #[test]
    fn test_error_conversion() {
        let anyhow_error = anyhow::anyhow!("test error");
        let aceryx_error: AceryxError = anyhow_error.into();
        assert!(matches!(aceryx_error, AceryxError::StorageError(_)));
    }
}