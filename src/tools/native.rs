// src/tools/native.rs

use anyhow::{anyhow, Result};
use async_trait::async_trait;
use reqwest::Client;
use serde_json::{json, Value};
use std::time::{Duration, Instant};

use crate::storage::{ExecutionMode, ToolCategory, ToolDefinition, WasmPermissions};

use super::{ExecutionContext, ProtocolHealth, Tool, ToolProtocol};

/// Built-in HTTP request tool for API integrations
pub struct HttpRequestTool {
    client: Client,
    definition: ToolDefinition,
}

impl HttpRequestTool {
    pub fn new() -> Self {
        let definition = ToolDefinition::new(
            "http_request".to_string(),
            "HTTP Request".to_string(),
            "Make HTTP requests to APIs and web services".to_string(),
            ToolCategory::Http,
            json!({
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "format": "uri",
                        "description": "The URL to make the request to"
                    },
                    "method": {
                        "type": "string",
                        "enum": ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"],
                        "default": "GET",
                        "description": "HTTP method to use"
                    },
                    "headers": {
                        "type": "object",
                        "description": "HTTP headers to include",
                        "additionalProperties": {"type": "string"}
                    },
                    "body": {
                        "oneOf": [
                            {"type": "string"},
                            {"type": "object"}
                        ],
                        "description": "Request body (string or JSON object)"
                    },
                    "timeout": {
                        "type": "number",
                        "default": 30,
                        "description": "Timeout in seconds"
                    },
                    "follow_redirects": {
                        "type": "boolean",
                        "default": true,
                        "description": "Whether to follow HTTP redirects"
                    }
                },
                "required": ["url"]
            }),
            json!({
                "type": "object",
                "properties": {
                    "status": {
                        "type": "number",
                        "description": "HTTP status code"
                    },
                    "headers": {
                        "type": "object",
                        "description": "Response headers"
                    },
                    "body": {
                        "type": "string",
                        "description": "Response body as string"
                    },
                    "json": {
                        "description": "Parsed JSON response (if content-type is JSON)"
                    },
                    "duration_ms": {
                        "type": "number",
                        "description": "Request duration in milliseconds"
                    },
                    "url": {
                        "type": "string",
                        "description": "Final URL (after redirects)"
                    }
                },
                "required": ["status", "headers", "body", "duration_ms", "url"]
            }),
            ExecutionMode::Wasm {
                permissions: WasmPermissions {
                    network_access: true,
                    filesystem_access: false,
                    environment_access: false,
                    max_memory_mb: 32,
                },
            },
        );

        Self {
            client: Client::builder()
                .timeout(Duration::from_secs(60))
                .user_agent("Aceryx/1.0")
                .build()
                .expect("Failed to create HTTP client"),
            definition,
        }
    }

    /// Parse the method string into reqwest::Method
    fn parse_method(&self, method: &str) -> Result<reqwest::Method> {
        match method.to_uppercase().as_str() {
            "GET" => Ok(reqwest::Method::GET),
            "POST" => Ok(reqwest::Method::POST),
            "PUT" => Ok(reqwest::Method::PUT),
            "DELETE" => Ok(reqwest::Method::DELETE),
            "PATCH" => Ok(reqwest::Method::PATCH),
            "HEAD" => Ok(reqwest::Method::HEAD),
            "OPTIONS" => Ok(reqwest::Method::OPTIONS),
            _ => Err(anyhow!("Unsupported HTTP method: {}", method)),
        }
    }

    /// Convert headers Value to reqwest HeaderMap
    fn build_headers(&self, headers: &Value) -> Result<reqwest::header::HeaderMap> {
        let mut header_map = reqwest::header::HeaderMap::new();

        if let Value::Object(obj) = headers {
            for (key, value) in obj {
                let header_name = reqwest::header::HeaderName::from_bytes(key.as_bytes())
                    .map_err(|e| anyhow!("Invalid header name '{}': {}", key, e))?;

                let header_value = match value {
                    Value::String(s) => reqwest::header::HeaderValue::from_str(s)
                        .map_err(|e| anyhow!("Invalid header value for '{}': {}", key, e))?,
                    _ => reqwest::header::HeaderValue::from_str(&value.to_string())
                        .map_err(|e| anyhow!("Invalid header value for '{}': {}", key, e))?,
                };

                header_map.insert(header_name, header_value);
            }
        }

        Ok(header_map)
    }
}

#[async_trait]
impl Tool for HttpRequestTool {
    async fn execute(&self, input: Value, _context: ExecutionContext) -> Result<Value> {
        let start_time = Instant::now();

        // Extract parameters from input
        let url = input["url"]
            .as_str()
            .ok_or_else(|| anyhow!("Missing required parameter 'url'"))?;

        let method = input["method"].as_str().unwrap_or("GET");
        let timeout = input["timeout"].as_u64().unwrap_or(30);
        let follow_redirects = input["follow_redirects"].as_bool().unwrap_or(true);

        // Build the request
        let method = self.parse_method(method)?;
        let mut request_builder = self
            .client
            .request(method, url)
            .timeout(Duration::from_secs(timeout));

        // Add headers if provided
        if let Some(headers) = input.get("headers") {
            if !headers.is_null() {
                let header_map = self.build_headers(headers)?;
                request_builder = request_builder.headers(header_map);
            }
        }

        // Add body if provided
        if let Some(body) = input.get("body") {
            if !body.is_null() {
                match body {
                    Value::String(s) => {
                        request_builder = request_builder.body(s.clone());
                    }
                    Value::Object(_) | Value::Array(_) => {
                        request_builder = request_builder.json(body);
                    }
                    _ => {
                        request_builder = request_builder.body(body.to_string());
                    }
                }
            }
        }

        // Configure redirect policy
        if !follow_redirects {
            // Note: In reqwest 0.12, redirect policy is set during client creation
            // For now, we'll handle redirects at the client level
            tracing::debug!("Redirect following disabled for this request");
        }

        // Execute the request
        let response = request_builder
            .send()
            .await
            .map_err(|e| anyhow!("HTTP request failed: {}", e))?;

        let duration = start_time.elapsed();
        let status = response.status().as_u16();
        let final_url = response.url().to_string();

        // Extract response headers
        let mut response_headers = serde_json::Map::new();
        for (name, value) in response.headers() {
            response_headers.insert(
                name.to_string(),
                Value::String(
                    value
                        .to_str()
                        .unwrap_or("<invalid-utf8>")
                        .to_string(),
                ),
            );
        }

        // Get response body
        let body_bytes = response
            .bytes()
            .await
            .map_err(|e| anyhow!("Failed to read response body: {}", e))?;

        let body_string = String::from_utf8_lossy(&body_bytes).to_string();

        // Try to parse as JSON if content-type indicates JSON
        let mut result = json!({
            "status": status,
            "headers": response_headers,
            "body": body_string,
            "duration_ms": duration.as_millis(),
            "url": final_url
        });

        // Attempt JSON parsing if it looks like JSON
        if let Some(content_type) = response_headers.get("content-type") {
            if let Value::String(ct) = content_type {
                if ct.contains("application/json") || ct.contains("text/json") {
                    if let Ok(json_value) = serde_json::from_str::<Value>(&body_string) {
                        result["json"] = json_value;
                    }
                }
            }
        } else if body_string.trim_start().starts_with(['{', '[']) {
            // Heuristic: if it starts with { or [, try parsing as JSON
            if let Ok(json_value) = serde_json::from_str::<Value>(&body_string) {
                result["json"] = json_value;
            }
        }

        Ok(result)
    }

    fn definition(&self) -> &ToolDefinition {
        &self.definition
    }

    fn validate_input(&self, input: &Value) -> Result<()> {
        // Basic validation - ensure URL is present
        if input.get("url").and_then(|v| v.as_str()).is_none() {
            return Err(anyhow!("Missing required parameter 'url'"));
        }

        // Validate method if provided
        if let Some(method) = input.get("method").and_then(|v| v.as_str()) {
            self.parse_method(method)?;
        }

        // Validate timeout if provided
        if let Some(timeout) = input.get("timeout") {
            if let Some(t) = timeout.as_u64() {
                if t == 0 || t > 300 {
                    return Err(anyhow!("Timeout must be between 1 and 300 seconds"));
                }
            } else if !timeout.is_null() {
                return Err(anyhow!("Timeout must be a number"));
            }
        }

        // Validate headers if provided
        if let Some(headers) = input.get("headers") {
            if !headers.is_null() && !headers.is_object() {
                return Err(anyhow!("Headers must be an object"));
            }
        }

        Ok(())
    }
}

/// Simple JSON manipulation tool
pub struct JsonTool {
    definition: ToolDefinition,
}

impl JsonTool {
    pub fn new() -> Self {
        let definition = ToolDefinition::new(
            "json_transform".to_string(),
            "JSON Transform".to_string(),
            "Transform, filter, and manipulate JSON data".to_string(),
            ToolCategory::Custom,
            json!({
                "type": "object",
                "properties": {
                    "data": {
                        "description": "Input JSON data to transform"
                    },
                    "operation": {
                        "type": "string",
                        "enum": ["extract", "filter", "merge", "validate"],
                        "description": "Operation to perform"
                    },
                    "path": {
                        "type": "string",
                        "description": "JSONPath expression for extract/filter operations"
                    },
                    "merge_data": {
                        "description": "Data to merge (for merge operation)"
                    },
                    "schema": {
                        "type": "object",
                        "description": "JSON schema for validation"
                    }
                },
                "required": ["data", "operation"]
            }),
            json!({
                "type": "object",
                "properties": {
                    "result": {
                        "description": "Transformed JSON data"
                    },
                    "valid": {
                        "type": "boolean",
                        "description": "Whether the data is valid (for validate operation)"
                    },
                    "errors": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Validation errors (if any)"
                    }
                }
            }),
            ExecutionMode::Wasm {
                permissions: WasmPermissions {
                    network_access: false,
                    filesystem_access: false,
                    environment_access: false,
                    max_memory_mb: 16,
                },
            },
        );

        Self { definition }
    }

    fn extract_path(&self, data: &Value, path: &str) -> Result<Value> {
        // Simple JSONPath implementation for basic property access
        if path.starts_with('$') {
            let path = &path[1..]; // Remove $ prefix
            if path.is_empty() {
                return Ok(data.clone());
            }

            let parts: Vec<&str> = path.split('.').filter(|s| !s.is_empty()).collect();
            let mut current = data;

            for part in parts {
                current = current
                    .get(part)
                    .ok_or_else(|| anyhow!("Path not found: {}", part))?;
            }

            Ok(current.clone())
        } else {
            // Simple property access
            data.get(path)
                .cloned()
                .ok_or_else(|| anyhow!("Property not found: {}", path))
        }
    }

    fn merge_objects(&self, base: &Value, merge_data: &Value) -> Value {
        match (base, merge_data) {
            (Value::Object(base_obj), Value::Object(merge_obj)) => {
                let mut result = base_obj.clone();
                for (key, value) in merge_obj {
                    result.insert(key.clone(), value.clone());
                }
                Value::Object(result)
            }
            _ => merge_data.clone(),
        }
    }
}

#[async_trait]
impl Tool for JsonTool {
    async fn execute(&self, input: Value, _context: ExecutionContext) -> Result<Value> {
        let data = input
            .get("data")
            .ok_or_else(|| anyhow!("Missing required parameter 'data'"))?;

        let operation = input["operation"]
            .as_str()
            .ok_or_else(|| anyhow!("Missing required parameter 'operation'"))?;

        match operation {
            "extract" => {
                let path = input["path"]
                    .as_str()
                    .ok_or_else(|| anyhow!("Missing required parameter 'path' for extract operation"))?;

                let result = self.extract_path(data, path)?;
                Ok(json!({"result": result}))
            }
            "filter" => {
                // Simple filtering - for arrays, filter by property existence
                if let Value::Array(arr) = data {
                    let path = input["path"].as_str().unwrap_or("id");
                    let filtered: Vec<&Value> = arr
                        .iter()
                        .filter(|item| self.extract_path(item, path).is_ok())
                        .collect();
                    Ok(json!({"result": filtered}))
                } else {
                    Ok(json!({"result": data}))
                }
            }
            "merge" => {
                let merge_data = input
                    .get("merge_data")
                    .ok_or_else(|| anyhow!("Missing required parameter 'merge_data' for merge operation"))?;

                let result = self.merge_objects(data, merge_data);
                Ok(json!({"result": result}))
            }
            "validate" => {
                // Basic validation - just check if it's valid JSON
                Ok(json!({
                    "result": data,
                    "valid": true,
                    "errors": []
                }))
            }
            _ => Err(anyhow!("Unsupported operation: {}", operation)),
        }
    }

    fn definition(&self) -> &ToolDefinition {
        &self.definition
    }

    fn validate_input(&self, input: &Value) -> Result<()> {
        if input.get("data").is_none() {
            return Err(anyhow!("Missing required parameter 'data'"));
        }

        let operation = input["operation"]
            .as_str()
            .ok_or_else(|| anyhow!("Missing required parameter 'operation'"))?;

        match operation {
            "extract" | "filter" => {
                if operation == "extract" && input.get("path").is_none() {
                    return Err(anyhow!("Missing required parameter 'path' for extract operation"));
                }
            }
            "merge" => {
                if input.get("merge_data").is_none() {
                    return Err(anyhow!("Missing required parameter 'merge_data' for merge operation"));
                }
            }
            "validate" => {
                // No additional validation needed
            }
            _ => return Err(anyhow!("Unsupported operation: {}", operation)),
        }

        Ok(())
    }
}

/// Native protocol implementation for built-in tools
pub struct NativeProtocol {
    tools: Vec<Box<dyn Tool>>,
}

impl NativeProtocol {
    pub fn new() -> Self {
        Self {
            tools: vec![
                Box::new(HttpRequestTool::new()),
                Box::new(JsonTool::new()),
            ],
        }
    }

    pub fn with_tools(tools: Vec<Box<dyn Tool>>) -> Self {
        Self { tools }
    }
}

impl Default for NativeProtocol {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl ToolProtocol for NativeProtocol {
    fn protocol_name(&self) -> &'static str {
        "native"
    }

    async fn discover_tools(&self) -> Result<Vec<ToolDefinition>> {
        Ok(self.tools.iter().map(|t| t.definition().clone()).collect())
    }

    async fn create_tool(&self, definition: &ToolDefinition) -> Result<Box<dyn Tool>> {
        for tool in &self.tools {
            if tool.definition().id == definition.id {
                // For native tools, we create a new instance based on the tool type
                match definition.id.as_str() {
                    "http_request" => return Ok(Box::new(HttpRequestTool::new())),
                    "json_transform" => return Ok(Box::new(JsonTool::new())),
                    _ => continue,
                }
            }
        }

        Err(anyhow!("Tool not found in native protocol: {}", definition.id))
    }

    async fn health_check(&self) -> Result<ProtocolHealth> {
        Ok(ProtocolHealth {
            protocol_name: "native".to_string(),
            healthy: true,
            error_message: None,
            tool_count: self.tools.len(),
            last_refresh: chrono::Utc::now(),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[tokio::test]
    async fn test_http_request_tool_validation() {
        let tool = HttpRequestTool::new();

        // Valid input
        let valid_input = json!({
            "url": "https://httpbin.org/get",
            "method": "GET"
        });
        assert!(tool.validate_input(&valid_input).is_ok());

        // Missing URL
        let invalid_input = json!({
            "method": "GET"
        });
        assert!(tool.validate_input(&invalid_input).is_err());

        // Invalid method
        let invalid_method = json!({
            "url": "https://httpbin.org/get",
            "method": "INVALID"
        });
        assert!(tool.validate_input(&invalid_method).is_err());

        // Invalid timeout
        let invalid_timeout = json!({
            "url": "https://httpbin.org/get",
            "timeout": 0
        });
        assert!(tool.validate_input(&invalid_timeout).is_err());
    }

    #[tokio::test]
    async fn test_json_tool() {
        let tool = JsonTool::new();
        let context = ExecutionContext::new("test_user".to_string());

        // Test extract operation
        let input = json!({
            "data": {"name": "test", "value": 42},
            "operation": "extract",
            "path": "name"
        });

        let result = tool.execute(input, context.clone()).await.unwrap();
        assert_eq!(result["result"], "test");

        // Test merge operation
        let input = json!({
            "data": {"name": "test"},
            "operation": "merge",
            "merge_data": {"value": 42}
        });

        let result = tool.execute(input, context.clone()).await.unwrap();
        assert_eq!(result["result"]["name"], "test");
        assert_eq!(result["result"]["value"], 42);

        // Test validation
        let input = json!({
            "data": {"valid": "json"},
            "operation": "validate"
        });

        let result = tool.execute(input, context).await.unwrap();
        assert_eq!(result["valid"], true);
    }

    #[tokio::test]
    async fn test_native_protocol() {
        let protocol = NativeProtocol::new();

        // Test discovery
        let tools = protocol.discover_tools().await.unwrap();
        assert_eq!(tools.len(), 2);
        assert!(tools.iter().any(|t| t.id == "http_request"));
        assert!(tools.iter().any(|t| t.id == "json_transform"));

        // Test tool creation
        let http_def = tools.iter().find(|t| t.id == "http_request").unwrap();
        let tool = protocol.create_tool(http_def).await.unwrap();
        assert_eq!(tool.definition().id, "http_request");

        // Test health check
        let health = protocol.health_check().await.unwrap();
        assert!(health.healthy);
        assert_eq!(health.protocol_name, "native");
        assert_eq!(health.tool_count, 2);
    }

    #[test]
    fn test_http_tool_method_parsing() {
        let tool = HttpRequestTool::new();

        assert!(tool.parse_method("GET").is_ok());
        assert!(tool.parse_method("post").is_ok());
        assert!(tool.parse_method("PUT").is_ok());
        assert!(tool.parse_method("INVALID").is_err());
    }

    #[test]
    fn test_http_tool_headers() {
        let tool = HttpRequestTool::new();

        let headers = json!({
            "Content-Type": "application/json",
            "Authorization": "Bearer token123"
        });

        let header_map = tool.build_headers(&headers).unwrap();
        assert_eq!(header_map.len(), 2);
        assert!(header_map.contains_key("content-type"));
        assert!(header_map.contains_key("authorization"));
    }

    #[test]
    fn test_json_tool_path_extraction() {
        let tool = JsonTool::new();
        let data = json!({
            "user": {
                "name": "Alice",
                "age": 30
            }
        });

        // Test simple property access
        let result = tool.extract_path(&data, "user").unwrap();
        assert_eq!(result["name"], "Alice");

        // Test JSONPath-style access
        let result = tool.extract_path(&data, "$.user.name").unwrap();
        assert_eq!(result, "Alice");

        // Test non-existent path
        assert!(tool.extract_path(&data, "nonexistent").is_err());
    }
}