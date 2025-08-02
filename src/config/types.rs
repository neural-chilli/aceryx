// src/config/types.rs

use serde::{Deserialize, Serialize};
use std::path::PathBuf;

/// Main configuration structure for Aceryx
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AceryxConfig {
    pub server: ServerConfig,
    pub storage: StorageConfig,
    pub tools: ToolsConfig,
    pub security: SecurityConfig,
    pub logging: LoggingConfig,
}

/// Server configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ServerConfig {
    /// Host to bind to (e.g., "0.0.0.0", "127.0.0.1")
    pub host: String,

    /// Port to bind to
    pub port: u16,

    /// Number of worker threads (None = use CPU count)
    pub workers: Option<usize>,

    /// Maximum number of concurrent connections
    pub max_connections: usize,

    /// Keep-alive timeout in seconds
    pub keep_alive: u64,

    /// Request timeout in seconds
    pub request_timeout: u64,
}

/// Storage backend configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StorageConfig {
    pub backend: StorageBackend,
    pub redis: Option<RedisConfig>,
    pub postgres: Option<PostgresConfig>,
}

/// Available storage backends
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum StorageBackend {
    Memory,
    Redis,
    Postgres,
}

/// Redis configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RedisConfig {
    /// Redis connection URL (e.g., "redis://localhost:6379")
    pub url: String,

    /// Connection pool size
    pub pool_size: u32,

    /// Connection timeout in seconds
    pub connect_timeout: u64,

    /// Command timeout in seconds
    pub command_timeout: u64,

    /// Database number (0-15)
    pub database: u8,

    /// Key prefix for all Aceryx keys
    pub key_prefix: String,
}

impl Default for RedisConfig {
    fn default() -> Self {
        Self {
            url: "redis://localhost:6379".to_string(),
            pool_size: 10,
            connect_timeout: 30,
            command_timeout: 30,
            database: 0,
            key_prefix: "aceryx:".to_string(),
        }
    }
}

/// PostgreSQL configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PostgresConfig {
    /// PostgreSQL connection URL
    pub url: String,

    /// Maximum number of connections in the pool
    pub max_connections: u32,

    /// Minimum number of connections in the pool
    pub min_connections: u32,

    /// Connection timeout in seconds
    pub connect_timeout: u64,

    /// Idle timeout in seconds
    pub idle_timeout: u64,

    /// Maximum connection lifetime in seconds
    pub max_lifetime: u64,
}

impl Default for PostgresConfig {
    fn default() -> Self {
        Self {
            url: "postgresql://user:pass@localhost/aceryx".to_string(),
            max_connections: 20,
            min_connections: 5,
            connect_timeout: 30,
            idle_timeout: 600,
            max_lifetime: 3600,
        }
    }
}

/// Tools and execution configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolsConfig {
    /// List of enabled tool protocols (e.g., ["native", "mcp", "openai"])
    pub enabled_protocols: Vec<String>,

    /// Native tools configuration
    pub native: NativeToolsConfig,

    /// Tool refresh interval in seconds (None = manual refresh only)
    pub refresh_interval: Option<u64>,

    /// Default execution timeout for tools in seconds
    pub execution_timeout: u64,

    /// Maximum number of concurrent tool executions
    pub max_concurrent_executions: usize,
}

/// Native tools configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NativeToolsConfig {
    /// List of enabled native tools
    pub enabled_tools: Vec<String>,
}

/// Security and authentication configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SecurityConfig {
    /// Authentication configuration
    pub authentication: Option<AuthenticationConfig>,

    /// CORS configuration
    pub cors: CorsConfig,

    /// Rate limiting configuration
    pub rate_limiting: Option<RateLimitConfig>,
}

/// Authentication methods
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum AuthenticationConfig {
    ApiKey { key: String },
    Jwt { secret: String },
}

/// CORS configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CorsConfig {
    /// Enable CORS
    pub enabled: bool,

    /// Allowed origins (["*"] for all)
    pub allow_origins: Vec<String>,

    /// Allowed HTTP methods
    pub allow_methods: Vec<String>,

    /// Allowed headers
    pub allow_headers: Vec<String>,
}

/// Rate limiting configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RateLimitConfig {
    /// Maximum requests per minute per client
    pub requests_per_minute: usize,

    /// Burst size (requests allowed above rate limit)
    pub burst_size: usize,
}

/// Logging configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LoggingConfig {
    /// Log level (trace, debug, info, warn, error)
    pub level: String,

    /// Log format
    pub format: LogFormat,

    /// Optional log file path
    pub file: Option<PathBuf>,

    /// Enable structured logging (JSON)
    pub structured: bool,
}

/// Log output formats
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum LogFormat {
    /// Human-readable format for development
    Pretty,

    /// Compact format
    Compact,

    /// JSON format for structured logging
    Json,
}

/// Environment-specific configuration helpers
impl AceryxConfig {
    /// Check if running in development mode
    pub fn is_development(&self) -> bool {
        matches!(self.storage.backend, StorageBackend::Memory) &&
            self.server.host == "127.0.0.1"
    }

    /// Check if authentication is enabled
    pub fn has_authentication(&self) -> bool {
        self.security.authentication.is_some()
    }

    /// Get the number of worker threads to use
    pub fn worker_threads(&self) -> usize {
        self.server.workers.unwrap_or_else(num_cpus::get)
    }

    /// Get the database URL for PostgreSQL
    pub fn postgres_url(&self) -> Option<&str> {
        self.storage.postgres.as_ref().map(|pg| pg.url.as_str())
    }

    /// Get the Redis URL
    pub fn redis_url(&self) -> Option<&str> {
        self.storage.redis.as_ref().map(|redis| redis.url.as_str())
    }

    /// Check if a specific tool protocol is enabled
    pub fn is_protocol_enabled(&self, protocol: &str) -> bool {
        self.tools.enabled_protocols.contains(&protocol.to_string())
    }

    /// Check if a specific native tool is enabled
    pub fn is_native_tool_enabled(&self, tool: &str) -> bool {
        self.tools.native.enabled_tools.contains(&tool.to_string())
    }

    /// Get log level as tracing filter
    pub fn log_filter(&self) -> String {
        format!("aceryx={},tower_http=info", self.logging.level)
    }
}

/// Configuration validation helpers
impl StorageConfig {
    pub fn validate(&self) -> Result<(), String> {
        match self.backend {
            StorageBackend::Redis => {
                if self.redis.is_none() {
                    return Err("Redis configuration is required when using Redis backend".to_string());
                }
            }
            StorageBackend::Postgres => {
                if self.postgres.is_none() {
                    return Err("PostgreSQL configuration is required when using PostgreSQL backend".to_string());
                }
            }
            StorageBackend::Memory => {
                // No additional validation needed
            }
        }
        Ok(())
    }
}

impl ServerConfig {
    pub fn validate(&self) -> Result<(), String> {
        if self.port == 0 {
            return Err("Server port cannot be 0".to_string());
        }

        if self.host.is_empty() {
            return Err("Server host cannot be empty".to_string());
        }

        if self.max_connections == 0 {
            return Err("Max connections must be greater than 0".to_string());
        }

        Ok(())
    }

    pub fn bind_address(&self) -> String {
        format!("{}:{}", self.host, self.port)
    }
}

impl ToolsConfig {
    pub fn validate(&self) -> Result<(), String> {
        if self.enabled_protocols.is_empty() {
            return Err("At least one tool protocol must be enabled".to_string());
        }

        if self.execution_timeout == 0 {
            return Err("Execution timeout must be greater than 0".to_string());
        }

        if self.max_concurrent_executions == 0 {
            return Err("Max concurrent executions must be greater than 0".to_string());
        }

        Ok(())
    }
}

impl SecurityConfig {
    pub fn validate(&self) -> Result<(), String> {
        if let Some(ref auth) = self.authentication {
            match auth {
                AuthenticationConfig::ApiKey { key } => {
                    if key.is_empty() {
                        return Err("API key cannot be empty".to_string());
                    }
                    if key.len() < 16 {
                        return Err("API key should be at least 16 characters long".to_string());
                    }
                }
                AuthenticationConfig::Jwt { secret } => {
                    if secret.is_empty() {
                        return Err("JWT secret cannot be empty".to_string());
                    }
                    if secret.len() < 32 {
                        return Err("JWT secret should be at least 32 characters long".to_string());
                    }
                }
            }
        }

        if let Some(ref rate_limit) = self.rate_limiting {
            if rate_limit.requests_per_minute == 0 {
                return Err("Rate limit requests per minute must be greater than 0".to_string());
            }
        }

        Ok(())
    }
}

impl LoggingConfig {
    pub fn validate(&self) -> Result<(), String> {
        let valid_levels = ["trace", "debug", "info", "warn", "error"];
        if !valid_levels.contains(&self.level.as_str()) {
            return Err(format!(
                "Invalid log level '{}'. Must be one of: {}",
                self.level,
                valid_levels.join(", ")
            ));
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_storage_config_validation() {
        let mut config = StorageConfig {
            backend: StorageBackend::Memory,
            redis: None,
            postgres: None,
        };

        assert!(config.validate().is_ok());

        config.backend = StorageBackend::Redis;
        assert!(config.validate().is_err());

        config.redis = Some(RedisConfig::default());
        assert!(config.validate().is_ok());
    }

    #[test]
    fn test_server_config_validation() {
        let mut config = ServerConfig {
            host: "localhost".to_string(),
            port: 8080,
            workers: None,
            max_connections: 1000,
            keep_alive: 75,
            request_timeout: 30,
        };

        assert!(config.validate().is_ok());
        assert_eq!(config.bind_address(), "localhost:8080");

        config.port = 0;
        assert!(config.validate().is_err());

        config.port = 8080;
        config.host = "".to_string();
        assert!(config.validate().is_err());
    }

    #[test]
    fn test_tools_config_validation() {
        let mut config = ToolsConfig {
            enabled_protocols: vec!["native".to_string()],
            native: NativeToolsConfig {
                enabled_tools: vec!["http_request".to_string()],
            },
            refresh_interval: Some(300),
            execution_timeout: 30,
            max_concurrent_executions: 100,
        };

        assert!(config.validate().is_ok());

        config.enabled_protocols.clear();
        assert!(config.validate().is_err());

        config.enabled_protocols.push("native".to_string());
        config.execution_timeout = 0;
        assert!(config.validate().is_err());
    }

    #[test]
    fn test_security_config_validation() {
        let mut config = SecurityConfig {
            authentication: None,
            cors: CorsConfig {
                enabled: true,
                allow_origins: vec!["*".to_string()],
                allow_methods: vec!["GET".to_string()],
                allow_headers: vec!["content-type".to_string()],
            },
            rate_limiting: None,
        };

        assert!(config.validate().is_ok());

        config.authentication = Some(AuthenticationConfig::ApiKey {
            key: "short".to_string(),
        });
        assert!(config.validate().is_err());

        config.authentication = Some(AuthenticationConfig::ApiKey {
            key: "this-is-a-long-enough-api-key".to_string(),
        });
        assert!(config.validate().is_ok());
    }

    #[test]
    fn test_logging_config_validation() {
        let mut config = LoggingConfig {
            level: "info".to_string(),
            format: LogFormat::Pretty,
            file: None,
            structured: false,
        };

        assert!(config.validate().is_ok());

        config.level = "invalid".to_string();
        assert!(config.validate().is_err());
    }

    #[test]
    fn test_aceryx_config_helpers() {
        let config = AceryxConfig {
            server: ServerConfig {
                host: "127.0.0.1".to_string(),
                port: 8080,
                workers: Some(4),
                max_connections: 1000,
                keep_alive: 75,
                request_timeout: 30,
            },
            storage: StorageConfig {
                backend: StorageBackend::Memory,
                redis: None,
                postgres: None,
            },
            tools: ToolsConfig {
                enabled_protocols: vec!["native".to_string(), "mcp".to_string()],
                native: NativeToolsConfig {
                    enabled_tools: vec!["http_request".to_string()],
                },
                refresh_interval: Some(300),
                execution_timeout: 30,
                max_concurrent_executions: 100,
            },
            security: SecurityConfig {
                authentication: Some(AuthenticationConfig::ApiKey {
                    key: "test-api-key-12345".to_string(),
                }),
                cors: CorsConfig {
                    enabled: true,
                    allow_origins: vec!["*".to_string()],
                    allow_methods: vec!["GET".to_string()],
                    allow_headers: vec!["content-type".to_string()],
                },
                rate_limiting: None,
            },
            logging: LoggingConfig {
                level: "debug".to_string(),
                format: LogFormat::Pretty,
                file: None,
                structured: false,
            },
        };

        assert!(config.is_development());
        assert!(config.has_authentication());
        assert_eq!(config.worker_threads(), 4);
        assert!(config.is_protocol_enabled("native"));
        assert!(config.is_protocol_enabled("mcp"));
        assert!(!config.is_protocol_enabled("openai"));
        assert!(config.is_native_tool_enabled("http_request"));
        assert!(!config.is_native_tool_enabled("file_read"));
    }
}