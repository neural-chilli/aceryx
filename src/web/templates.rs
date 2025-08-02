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

        // Load all embedded templates
        for file_path in TemplateAssets::iter() {
            if let Some(template_file) = TemplateAssets::get(&file_path) {
                let template_str = std::str::from_utf8(&template_file.data)?;
                env.add_template_owned(file_path.to_string(), template_str.to_string())?;
            }
        }

        // Add custom template functions/filters if needed
        env.add_function("asset_url", asset_url_helper);

        Ok(Self { env })
    }

    /// Render a template with the given context
    pub fn render(&self, template_name: &str, context: &serde_json::Value) -> Result<String> {
        let template = self.env.get_template(template_name)?;
        let rendered = template.render(context)?;
        Ok(rendered)
    }
}

/// Template helper function to generate asset URLs
fn asset_url_helper(_state: &minijinja::State, path: String) -> Result<Value, minijinja::Error> {
    Ok(Value::from(format!("/static/{}", path)))
}
