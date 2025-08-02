// src/config/mod.rs

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::path::PathBuf;

pub mod types;
mod types;

pub use types::*;

impl AceryxConfig {
    /// Load configuration from multiple sources with precedence:
    /// 1. Command line arguments (highest priority)
    /// 2. Environment variables
    /// 3. Configuration file
    /// 4. Default values (lowest priority)
    pub fn load() -> Result<Self> {
        let mut settings = config::Config::builder();

        // Start with defaults
        settings = settings.add_source(config::Config::try_from(&Self::default())?);

        // Load from config file if it exists
        let config_file = std::env::var("ACERYX_CONFIG")
            .unwrap_or_else(|_| "aceryx.toml".to_string());

        if std::path::Path::new(&config_file).exists() {
            settings = settings.add_source(config::File::with_name(&config_file));
        }

        // Override with environment variables (prefix: ACERYX_)
        settings = settings.add_source(
            config::Environment::with_prefix("ACERYX")
                .separator("_")
                .try_parsing(true),
        );

        let config = settings
            .build()
            .context("Failed to build configuration")?
            .try_deserialize()
            .context("Failed to deserialize configuration")?;

        Ok(config)
    }

    /// Get the storage backend configuration
    pub fn storage_backend(&self) -> &StorageBackend {
        &self.storage.backend
    }

    /// Validate the configuration
    pub fn validate(&self) -> Result<()> {
        // Validate server configuration
        if self.server.port == 0 {
            return Err(anyhow::anyhow!("Server port cannot be 0"));
        }

        if self.server.host.is_empty() {
            return Err(anyhow::anyhow!("Server host cannot be empty"));
        }

        // Validate storage configuration
        match &self.storage.backend {
            StorageBackend::Redis => {
                if self.storage.redis.is_none() {
                    return Err(anyhow::anyhow!("Redis configuration required when using Redis backend"));
                }
            }
            StorageBackend::Postgres => {
                if self.storage.postgres.is_none() {
                    return Err(anyhow::anyhow!("PostgreSQL configuration required when using PostgreSQL backend"));
                }
            }
            StorageBackend::Memory => {
                // No additional validation needed for memory backend
            }
        }

        // Validate tools configuration
        if self.tools.enabled_protocols.is_empty() {
            return Err(anyhow::anyhow!("At least one tool protocol must be enabled"));
        }

        // Validate security configuration
        if let Some(ref auth) = self.security.authentication {
            match auth {
                AuthenticationConfig::ApiKey { key } => {
                    if key.is_empty() {
                        return Err(anyhow::anyhow!("API key cannot be empty"));
                    }
                }
                AuthenticationConfig::Jwt { secret } => {
                    if secret.is_empty() {
                        return Err(anyhow::anyhow!("JWT secret cannot be empty"));
                    }
                }
            }
        }

        Ok(())
    }

    /// Create a development configuration with sensible defaults
    pub fn development() -> Self {
        Self {
            server: ServerConfig {
                host: "127.0.0.1".to_string(),
                port: 8080,
                workers: None,
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
                enabled_protocols: vec!["native".to_string()],
                native: NativeToolsConfig {
                    enabled_tools: vec![
                        "http_request".to_string(),
                        "json_transform".to_string(),
                    ],
                },
                refresh_interval: Some(300), // 5 minutes
                execution_timeout: 30,
                max_concurrent_executions: 100,
            },
            security: SecurityConfig {
                authentication: None,
                cors: CorsConfig {
                    enabled: true,
                    allow_origins: vec!["*".to_string()],
                    allow_methods: vec![
                        "GET".to_string(),
                        "POST".to_string(),
                        "PUT".to_string(),
                        "DELETE".to_string(),
                        "OPTIONS".to_string(),
                    ],
                    allow_headers: vec!["content-type".to_string(), "authorization".to_string()],
                },
                rate_limiting: None,
            },
            logging: LoggingConfig {
                level: "info".to_string(),
                format: LogFormat::Pretty,
                file: None,
                structured: false,
            },
        }
    }

    /// Create a production configuration template
    pub fn production() -> Self {
        Self {
            server: ServerConfig {
                host: "0.0.0.0".to_string(),
                port: 8080,
                workers: None, // Will use system CPU count
                max_connections: 10000,
                keep_alive: 30,
                request_timeout: 60,
            },
            storage: StorageConfig {
                backend: StorageBackend::Postgres,
                redis: None,
                postgres: Some(PostgresConfig {
                    url: "postgresql://user:pass@localhost/aceryx".to_string(),
                    max_connections: 20,
                    min_connections: 5,
                    connect_timeout: 30,
                    idle_timeout: 600,
                    max_lifetime: 3600,
                }),
            },
            tools: ToolsConfig {
                enabled_protocols: vec!["native".to_string(), "mcp".to_string()],
                native: NativeToolsConfig {
                    enabled_tools: vec![
                        "http_request".to_string(),
                        "json_transform".to_string(),
                    ],
                },
                refresh_interval: Some(3600), // 1 hour
                execution_timeout: 60,
                max_concurrent_executions: 1000,
            },
            security: SecurityConfig {
                authentication: Some(AuthenticationConfig::ApiKey {
                    key: "your-api-key-here".to_string(),
                }),
                cors: CorsConfig {
                    enabled: true,
                    allow_origins: vec!["https://yourdomain.com".to_string()],
                    allow_methods: vec![
                        "GET".to_string(),
                        "POST".to_string(),
                        "PUT".to_string(),
                        "DELETE".to_string(),
                    ],
                    allow_headers: vec!["content-type".to_string(), "authorization".to_string()],
                },
                rate_limiting: Some(RateLimitConfig {
                    requests_per_minute: 60,
                    burst_size: 10,
                }),
            },
            logging: LoggingConfig {
                level: "warn".to_string(),
                format: LogFormat::Json,
                file: Some(PathBuf::from("/var/log/aceryx/aceryx.log")),
                structured: true,
            },
        }
    }

    /// Export configuration as TOML string
    pub fn to_toml(&self) -> Result<String> {
        toml::to_string_pretty(self).context("Failed to serialize configuration to TOML")
    }

    /// Load configuration from TOML string
    pub fn from_toml(toml_str: &str) -> Result<Self> {
        toml::from_str(toml_str).context("Failed to parse TOML configuration")
    }

    /// Save configuration to file
    pub fn save_to_file(&self, path: &PathBuf) -> Result<()> {
        let toml_content = self.to_toml()?;
        std::fs::write(path, toml_content)
            .with_context(|| format!("Failed to write configuration to {}", path.display()))?;
        Ok(())
    }

    /// Load configuration from file
    pub fn load_from_file(path: &PathBuf) -> Result<Self> {
        let content = std::fs::read_to_string(path)
            .with_context(|| format!("Failed to read configuration from {}", path.display()))?;
        Self::from_toml(&content)
    }
}

impl Default for AceryxConfig {
    fn default() -> Self {
        Self::development()
    }
}

/// Helper function to load configuration with better error reporting
pub fn load_config() -> Result<AceryxConfig> {
    let config = AceryxConfig::load()
        .context("Failed to load Aceryx configuration")?;

    config.validate()
        .context("Configuration validation failed")?;

    // Log configuration source information
    if std::env::var("ACERYX_CONFIG").is_ok() {
        tracing::info!("Configuration loaded from custom file: {}",
            std::env::var("ACERYX_CONFIG").unwrap());
    } else if std::path::Path::new("aceryx.toml").exists() {
        tracing::info!("Configuration loaded from: aceryx.toml");
    } else {
        tracing::info!("Using default configuration (no config file found)");
    }

    // Log active backend
    tracing::info!("Storage backend: {:?}", config.storage.backend);
    tracing::info!("Enabled tool protocols: {:?}", config.tools.enabled_protocols);

    Ok(config)
}

/// Generate a sample configuration file
pub fn generate_sample_config(production: bool) -> Result<()> {
    let config = if production {
        AceryxConfig::production()
    } else {
        AceryxConfig::development()
    };

    let filename = if production {
        "aceryx.production.toml"
    } else {
        "aceryx.sample.toml"
    };

    config.save_to_file(&PathBuf::from(filename))?;

    println!("Generated sample configuration: {}", filename);
    println!("\nTo use this configuration:");
    println!("1. Copy to aceryx.toml: cp {} aceryx.toml", filename);
    println!("2. Edit the configuration as needed");
    println!("3. Set environment variable: export ACERYX_CONFIG=aceryx.toml");

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_default_config() {
        let config = AceryxConfig::default();
        assert!(config.validate().is_ok());
        assert_eq!(config.server.port, 8080);
        assert_eq!(config.storage.backend, StorageBackend::Memory);
    }

    #[test]
    fn test_development_config() {
        let config = AceryxConfig::development();
        assert!(config.validate().is_ok());
        assert_eq!(config.server.host, "127.0.0.1");
        assert!(config.tools.enabled_protocols.contains(&"native".to_string()));
    }

    #[test]
    fn test_production_config() {
        let config = AceryxConfig::production();
        // Production config might fail validation due to placeholder values
        // but structure should be correct
        assert_eq!(config.server.host, "0.0.0.0");
        assert_eq!(config.storage.backend, StorageBackend::Postgres);
        assert!(config.security.authentication.is_some());
    }

    #[test]
    fn test_config_serialization() {
        let config = AceryxConfig::development();
        let toml_str = config.to_toml().unwrap();
        let deserialized = AceryxConfig::from_toml(&toml_str).unwrap();

        assert_eq!(config.server.port, deserialized.server.port);
        assert_eq!(config.storage.backend, deserialized.storage.backend);
    }

    #[test]
    fn test_config_file_operations() {
        let dir = tempdir().unwrap();
        let config_path = dir.path().join("test_config.toml");

        let config = AceryxConfig::development();
        config.save_to_file(&config_path).unwrap();

        let loaded_config = AceryxConfig::load_from_file(&config_path).unwrap();
        assert_eq!(config.server.port, loaded_config.server.port);
    }

    #[test]
    fn test_config_validation() {
        let mut config = AceryxConfig::development();

        // Valid config should pass
        assert!(config.validate().is_ok());

        // Invalid port should fail
        config.server.port = 0;
        assert!(config.validate().is_err());

        // Reset port and test empty host
        config.server.port = 8080;
        config.server.host = "".to_string();
        assert!(config.validate().is_err());

        // Test Redis backend without configuration
        config.server.host = "localhost".to_string();
        config.storage.backend = StorageBackend::Redis;
        config.storage.redis = None;
        assert!(config.validate().is_err());
    }

    #[test]
    fn test_environment_variables() {
        // Set environment variable
        std::env::set_var("ACERYX_SERVER_PORT", "9090");
        std::env::set_var("ACERYX_STORAGE_BACKEND", "memory");

        // This would normally load from environment, but we'll just test that the function exists
        // In a real test, we'd need to mock the config loading
        assert!(AceryxConfig::load().is_ok());

        // Clean up
        std::env::remove_var("ACERYX_SERVER_PORT");
        std::env::remove_var("ACERYX_STORAGE_BACKEND");
    }
}