// tests/integration_tests.rs

use anyhow::Result;
use serde_json::json;
use std::sync::Arc;
use tokio;

use aceryx::{
    storage::{memory::MemoryStorage, Flow, FlowFilters, ToolCategory, ToolDefinition, ExecutionMode, WasmPermissions},
    tools::{native::NativeProtocol, ExecutionContext, ToolRegistry},
    api,
};

/// Test the complete flow: storage -> tools -> API
#[tokio::test]
async fn test_end_to_end_flow() -> Result<()> {
    // Initialize storage
    let storage = Arc::new(MemoryStorage::new());

    // Create a test flow
    let flow = Flow::new(
        "Integration Test Flow".to_string(),
        "End-to-end test flow".to_string(),
        "test_user".to_string(),
    );
    let flow_id = storage.create_flow(flow).await?;

    // Verify flow was created
    let retrieved_flow = storage.get_flow(&flow_id).await?;
    assert!(retrieved_flow.is_some());
    assert_eq!(retrieved_flow.unwrap().name, "Integration Test Flow");

    // Initialize tool registry
    let mut tool_registry = ToolRegistry::new(storage.clone());
    tool_registry.add_protocol(Box::new(NativeProtocol::new()));

    // Refresh tools
    let discovered = tool_registry.refresh_tools().await?;
    assert!(discovered > 0);

    // Test tool execution
    let context = ExecutionContext::new("test_user".to_string());
    let input = json!({
        "data": {"name": "test", "value": 42},
        "operation": "validate"
    });

    let result = tool_registry.execute_tool("json_transform", input, context).await?;
    assert_eq!(result["valid"], true);

    Ok(())
}

/// Test API endpoints with real storage and tools
#[tokio::test]
async fn test_api_integration() -> Result<()> {
    use axum::{body::Body, http::{Method, Request, StatusCode}};
    use tower::ServiceExt;

    // Setup
    let storage = Arc::new(MemoryStorage::new());
    let mut tool_registry = ToolRegistry::new(storage.clone());
    tool_registry.add_protocol(Box::new(NativeProtocol::new()));
    tool_registry.refresh_tools().await?;

    // Create API router
    let app = api::create_api_router(storage.clone(), Arc::new(tool_registry));

    // Test flow creation
    let create_request = json!({
        "name": "API Test Flow",
        "description": "Flow created via API",
        "tags": ["api", "test"]
    });

    let request = Request::builder()
        .method(Method::POST)
        .uri("/api/v1/flows")
        .header("content-type", "application/json")
        .body(Body::from(serde_json::to_string(&create_request)?))
        .unwrap();

    let response = app.clone().oneshot(request).await.unwrap();
    assert_eq!(response.status(), StatusCode::CREATED);

    // Test flow listing
    let request = Request::builder()
        .method(Method::GET)
        .uri("/api/v1/flows")
        .body(Body::empty())
        .unwrap();

    let response = app.clone().oneshot(request).await.unwrap();
    assert_eq!(response.status(), StatusCode::OK);

    // Test tool listing
    let request = Request::builder()
        .method(Method::GET)
        .uri("/api/v1/tools")
        .body(Body::empty())
        .unwrap();

    let response = app.clone().oneshot(request).await.unwrap();
    assert_eq!(response.status(), StatusCode::OK);

    // Test tool execution
    let execute_request = json!({
        "input": {
            "data": {"test": "value"},
            "operation": "validate"
        }
    });

    let request = Request::builder()
        .method(Method::POST)
        .uri("/api/v1/tools/execute/json_transform")
        .header("content-type", "application/json")
        .body(Body::from(serde_json::to_string(&execute_request)?))
        .unwrap();

    let response = app.oneshot(request).await.unwrap();
    assert_eq!(response.status(), StatusCode::OK);

    Ok(())
}

/// Test storage backends with different data scenarios
#[tokio::test]
async fn test_storage_scenarios() -> Result<()> {
    let storage = Arc::new(MemoryStorage::new());

    // Test concurrent flow operations
    let handles: Vec<_> = (0..10)
        .map(|i| {
            let storage = storage.clone();
            tokio::spawn(async move {
                let flow = Flow::new(
                    format!("Concurrent Flow {}", i),
                    format!("Flow number {}", i),
                    format!("user_{}", i),
                );
                storage.create_flow(flow).await
            })
        })
        .collect();

    // Wait for all operations to complete
    for handle in handles {
        let result = handle.await??;
        // Each flow should have a unique ID
        assert!(storage.get_flow(&result).await?.is_some());
    }

    // Test filtering and search
    let filters = FlowFilters::default().limit(5);
    let flows = storage.list_flows(filters).await?;
    assert!(flows.len() <= 5);

    // Test search functionality
    let search_results = storage.search_flows("Concurrent").await?;
    assert_eq!(search_results.len(), 10);

    Ok(())
}

/// Test tool protocol registration and discovery
#[tokio::test]
async fn test_tool_protocol_system() -> Result<()> {
    let storage = Arc::new(MemoryStorage::new());
    let mut registry = ToolRegistry::new(storage.clone());

    // Test initial state
    assert_eq!(registry.protocols().len(), 0);

    // Add native protocol
    registry.add_protocol(Box::new(NativeProtocol::new()));
    assert_eq!(registry.protocols().len(), 1);

    // Test tool discovery
    let discovered = registry.refresh_tools().await?;
    assert!(discovered > 0);

    // Verify tools are in storage
    let tools = storage.list_tools(None).await?;
    assert_eq!(tools.len(), discovered);

    // Test category filtering
    let http_tools = storage.list_tools(Some(ToolCategory::Http)).await?;
    assert!(http_tools.len() > 0);

    let ai_tools = storage.list_tools(Some(ToolCategory::AI)).await?;
    assert_eq!(ai_tools.len(), 0); // Native protocol has no AI tools

    // Test tool search
    let search_results = storage.search_tools("HTTP").await?;
    assert!(search_results.len() > 0);

    Ok(())
}

/// Test error handling and edge cases
#[tokio::test]
async fn test_error_scenarios() -> Result<()> {
    let storage = Arc::new(MemoryStorage::new());
    let tool_registry = Arc::new(ToolRegistry::new(storage.clone()));

    // Test non-existent flow
    let non_existent_id = uuid::Uuid::new_v4();
    let result = storage.get_flow(&non_existent_id).await?;
    assert!(result.is_none());

    // Test non-existent tool
    let result = tool_registry.get_tool("nonexistent_tool").await?;
    assert!(result.is_none());

    // Test tool execution with invalid input
    let context = ExecutionContext::new("test_user".to_string());
    let invalid_input = json!({
        "operation": "invalid_operation"
    });

    let result = tool_registry.execute_tool("json_transform", invalid_input, context).await;
    assert!(result.is_err());

    // Test invalid flow validation
    let mut invalid_flow = Flow::new(
        "".to_string(), // Empty name should fail validation
        "Description".to_string(),
        "user".to_string(),
    );

    let result = storage.create_flow(invalid_flow.clone()).await;
    assert!(result.is_err());

    Ok(())
}

/// Test configuration loading and validation
#[tokio::test]
async fn test_configuration() -> Result<()> {
    use aceryx::config::{AceryxConfig, StorageBackend};

    // Test default configuration
    let config = AceryxConfig::default();
    assert!(config.validate().is_ok());
    assert_eq!(config.storage.backend, StorageBackend::Memory);

    // Test development configuration
    let dev_config = AceryxConfig::development();
    assert!(dev_config.validate().is_ok());
    assert!(dev_config.is_development());

    // Test configuration serialization
    let toml_str = config.to_toml()?;
    let deserialized = AceryxConfig::from_toml(&toml_str)?;
    assert_eq!(config.server.port, deserialized.server.port);

    Ok(())
}

/// Test health checks and monitoring
#[tokio::test]
async fn test_health_monitoring() -> Result<()> {
    let storage = Arc::new(MemoryStorage::new());
    let mut tool_registry = ToolRegistry::new(storage.clone());
    tool_registry.add_protocol(Box::new(NativeProtocol::new()));

    // Test storage health
    let storage_health = storage.health_check().await?;
    assert!(storage_health.healthy);
    assert_eq!(storage_health.backend_type, "memory");

    // Test tool registry health
    let registry_health = tool_registry.health_check().await?;
    assert!(registry_health.healthy);
    assert_eq!(registry_health.protocols.len(), 1);

    Ok(())
}

/// Benchmark basic operations
#[tokio::test]
async fn test_performance_characteristics() -> Result<()> {
    let storage = Arc::new(MemoryStorage::new());

    // Measure flow creation performance
    let start = std::time::Instant::now();

    for i in 0..1000 {
        let flow = Flow::new(
            format!("Perf Test Flow {}", i),
            "Performance test".to_string(),
            "perf_user".to_string(),
        );
        storage.create_flow(flow).await?;
    }

    let creation_time = start.elapsed();
    println!("Created 1000 flows in {:?} ({:.2} flows/ms)",
             creation_time, 1000.0 / creation_time.as_millis() as f64);

    // Measure retrieval performance
    let start = std::time::Instant::now();
    let flows = storage.list_flows(FlowFilters::default()).await?;
    let retrieval_time = start.elapsed();

    assert_eq!(flows.len(), 1000);
    println!("Retrieved {} flows in {:?}", flows.len(), retrieval_time);

    // Basic performance assertions (adjust based on requirements)
    assert!(creation_time.as_millis() < 5000); // Should create 1000 flows in under 5 seconds
    assert!(retrieval_time.as_millis() < 100);  // Should retrieve 1000 flows in under 100ms

    Ok(())
}

/// Test concurrent access patterns
#[tokio::test]
async fn test_concurrent_access() -> Result<()> {
    let storage = Arc::new(MemoryStorage::new());

    // Create a flow to work with
    let flow = Flow::new(
        "Concurrent Test Flow".to_string(),
        "Test concurrent access".to_string(),
        "test_user".to_string(),
    );
    let flow_id = storage.create_flow(flow).await?;

    // Spawn multiple concurrent readers
    let read_handles: Vec<_> = (0..50)
        .map(|_| {
            let storage = storage.clone();
            let flow_id = flow_id;
            tokio::spawn(async move {
                storage.get_flow(&flow_id).await
            })
        })
        .collect();

    // Spawn multiple concurrent listers
    let list_handles: Vec<_> = (0..20)
        .map(|_| {
            let storage = storage.clone();
            tokio::spawn(async move {
                storage.list_flows(FlowFilters::default()).await
            })
        })
        .collect();

    // Wait for all operations
    for handle in read_handles {
        let result = handle.await??;
        assert!(result.is_some());
    }

    for handle in list_handles {
        let result = handle.await??;
        assert!(result.len() >= 1);
    }

    Ok(())
}

/// Test the native tools with various inputs
#[tokio::test]
async fn test_native_tools_comprehensive() -> Result<()> {
    let storage = Arc::new(MemoryStorage::new());
    let mut registry = ToolRegistry::new(storage.clone());
    registry.add_protocol(Box::new(NativeProtocol::new()));
    registry.refresh_tools().await?;

    let context = ExecutionContext::new("test_user".to_string());

    // Test JSON tool with various operations
    let test_cases = vec![
        (json!({
            "data": {"name": "Alice", "age": 30},
            "operation": "extract",
            "path": "name"
        }), "extract"),
        (json!({
            "data": {"base": "value"},
            "operation": "merge",
            "merge_data": {"new": "data"}
        }), "merge"),
        (json!({
            "data": {"valid": "json"},
            "operation": "validate"
        }), "validate"),
    ];

    for (input, operation) in test_cases {
        let result = registry.execute_tool("json_transform", input, context.clone()).await?;
        println!("JSON {} operation result: {}", operation, result);
        assert!(result.is_object());
    }

    Ok(())
}

/// Helper function to create test data
async fn create_test_flows(storage: &Arc<MemoryStorage>, count: usize) -> Result<Vec<uuid::Uuid>> {
    let mut flow_ids = Vec::new();

    for i in 0..count {
        let mut flow = Flow::new(
            format!("Test Flow {}", i),
            format!("Test flow number {}", i),
            format!("user_{}", i % 3), // Create flows for 3 different users
        );

        // Add some variety in tags
        if i % 2 == 0 {
            flow.tags.push("even".to_string());
        } else {
            flow.tags.push("odd".to_string());
        }

        if i % 5 == 0 {
            flow.tags.push("milestone".to_string());
        }

        let flow_id = storage.create_flow(flow).await?;
        flow_ids.push(flow_id);
    }

    Ok(flow_ids)
}

#[tokio::test]
async fn test_filtering_and_pagination() -> Result<()> {
    let storage = Arc::new(MemoryStorage::new());
    let _flow_ids = create_test_flows(&storage, 20).await?;

    // Test pagination
    let page1 = storage.list_flows(FlowFilters::default().limit(5)).await?;
    assert_eq!(page1.len(), 5);

    let page2 = storage.list_flows(FlowFilters::default().limit(5).offset(5)).await?;
    assert_eq!(page2.len(), 5);

    // Ensure different pages have different flows
    let page1_ids: std::collections::HashSet<_> = page1.iter().map(|f| f.id).collect();
    let page2_ids: std::collections::HashSet<_> = page2.iter().map(|f| f.id).collect();
    assert!(page1_ids.is_disjoint(&page2_ids));

    // Test filtering by user
    let user_flows = storage.list_flows(
        FlowFilters::default().created_by("user_0".to_string())
    ).await?;
    assert!(user_flows.len() > 0);
    assert!(user_flows.iter().all(|f| f.created_by == "user_0"));

    // Test filtering by tags
    let even_flows = storage.list_flows(
        FlowFilters::default().with_tags(vec!["even".to_string()])
    ).await?;
    assert!(even_flows.len() > 0);
    assert!(even_flows.iter().all(|f| f.tags.contains(&"even".to_string())));

    Ok(())
}