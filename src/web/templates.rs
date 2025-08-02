// src/web/templates.rs - Fixed version with better error handling

use anyhow::Result;
use minijinja::{Environment, Value};
use rust_embed::RustEmbed;

/// Embedded template files
#[derive(RustEmbed)]
#[folder = "web/templates/"]
struct TemplateAssets;

/// Template rendering engine with Minijinja
#[derive(Clone)]
pub struct Templates {
    env: Environment<'static>,
}

impl Templates {
    /// Create a new template engine with embedded templates
    pub fn new() -> Result<Self> {
        let mut env = Environment::new();

        // Load all embedded templates with better error handling
        for file_path in TemplateAssets::iter() {
            if let Some(template_file) = TemplateAssets::get(&file_path) {
                match std::str::from_utf8(&template_file.data) {
                    Ok(template_str) => {
                        if let Err(e) = env.add_template_owned(file_path.to_string(), template_str.to_string()) {
                            tracing::warn!("Failed to load template {}: {}", file_path, e);
                        } else {
                            tracing::debug!("Loaded template: {}", file_path);
                        }
                    }
                    Err(e) => {
                        tracing::warn!("Template {} contains invalid UTF-8: {}", file_path, e);
                    }
                }
            }
        }

        // Add a simple default template if no templates were loaded
        env.add_template_owned(
            "default.html".to_string(),
            r#"<!DOCTYPE html>
<html>
<head>
    <title>Aceryx</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .error { color: #d32f2f; }
    </style>
</head>
<body>
    <h1>🍁 Aceryx</h1>
    <p>Templates not yet configured. Server is running successfully.</p>
    <p><strong>Available endpoints:</strong></p>
    <ul>
        <li><a href="/health">Health Check</a></li>
        <li><a href="/api/v1/flows">API - Flows</a></li>
        <li><a href="/api/v1/tools">API - Tools</a></li>
    </ul>
</body>
</html>"#.to_string(),
        )?;

        // Add custom template functions/filters if needed
        env.add_function("asset_url", asset_url_helper);

        Ok(Self { env })
    }

    /// Render a template with the given context
    pub fn render(&self, template_name: &str, context: &serde_json::Value) -> Result<String> {
        // Try to get the requested template first
        match self.env.get_template(template_name) {
            Ok(template) => {
                match template.render(context) {
                    Ok(rendered) => Ok(rendered),
                    Err(e) => {
                        tracing::error!("Template rendering error for {}: {}", template_name, e);
                        // Fallback to a simple error template
                        Ok(self.render_error_fallback(template_name, &e.to_string()))
                    }
                }
            }
            Err(_) => {
                tracing::warn!("Template {} not found, using fallback", template_name);
                // Use fallback template
                Ok(self.render_fallback(template_name, context))
            }
        }
    }

    /// Render a fallback template when the requested template is not found
    fn render_fallback(&self, template_name: &str, context: &serde_json::Value) -> String {
        format!(
            r#"<!DOCTYPE html>
<html>
<head>
    <title>Aceryx - Template Missing</title>
    <style>
        body {{ font-family: Arial, sans-serif; margin: 40px; background: #f8f9fa; }}
        .container {{ max-width: 800px; margin: 0 auto; background: white; padding: 2rem; border-radius: 8px; }}
        .error {{ color: #dc3545; }}
        pre {{ background: #f8f9fa; padding: 1rem; border-radius: 4px; overflow: auto; }}
    </style>
</head>
<body>
    <div class="container">
        <h1>🍁 Aceryx</h1>
        <h2 class="error">Template Not Found</h2>
        <p>The template <code>{}</code> was not found. This is expected during development.</p>
        <h3>Available Endpoints:</h3>
        <ul>
            <li><a href="/health">Health Check (JSON)</a></li>
            <li><a href="/api/v1/flows">API - List Flows</a></li>
            <li><a href="/api/v1/tools">API - List Tools</a></li>
            <li><a href="/api/v1/system/info">API - System Info</a></li>
        </ul>
        <h3>Context Data:</h3>
        <pre>{}</pre>
        <p><em>Note: Template files will be added in the next development phase.</em></p>
    </div>
</body>
</html>"#,
            template_name,
            serde_json::to_string_pretty(context).unwrap_or_else(|_| "{}".to_string())
        )
    }

    /// Render an error fallback when template rendering fails
    fn render_error_fallback(&self, template_name: &str, error: &str) -> String {
        format!(
            r#"<!DOCTYPE html>
<html>
<head>
    <title>Aceryx - Template Error</title>
    <style>
        body {{ font-family: Arial, sans-serif; margin: 40px; background: #f8f9fa; }}
        .container {{ max-width: 800px; margin: 0 auto; background: white; padding: 2rem; border-radius: 8px; }}
        .error {{ color: #dc3545; }}
    </style>
</head>
<body>
    <div class="container">
        <h1>🍁 Aceryx</h1>
        <h2 class="error">Template Rendering Error</h2>
        <p>Failed to render template: <code>{}</code></p>
        <p><strong>Error:</strong> {}</p>
        <p><a href="/health">← Check System Health</a></p>
    </div>
</body>
</html>"#,
            template_name, error
        )
    }
}

/// Template helper function to generate asset URLs
fn asset_url_helper(_state: &minijinja::State, path: String) -> Result<Value, minijinja::Error> {
    Ok(Value::from(format!("/static/{}", path)))
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn test_templates_creation() {
        let templates = Templates::new();
        assert!(templates.is_ok());
    }

    #[test]
    fn test_fallback_rendering() {
        let templates = Templates::new().unwrap();
        let context = json!({"title": "Test"});

        // This should use fallback since template doesn't exist
        let result = templates.render("nonexistent.html", &context);
        assert!(result.is_ok());
        assert!(result.unwrap().contains("Template Not Found"));
    }

    #[test]
    fn test_error_fallback() {
        let templates = Templates::new().unwrap();
        let error_html = templates.render_error_fallback("test.html", "Test error");
        assert!(error_html.contains("Template Rendering Error"));
        assert!(error_html.contains("Test error"));
    }
}