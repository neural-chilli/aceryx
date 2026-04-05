package httpfw

// PluginHTTPRequest is the normalized request envelope passed from host functions.
type PluginHTTPRequest struct {
	TenantID   string
	PluginID   string
	Method     string
	URL        string
	Headers    map[string]string
	Body       []byte
	TimeoutMS  int
	AuthConfig *AuthConfig
}

// PluginHTTPResponse is the normalized response envelope returned to host functions.
type PluginHTTPResponse struct {
	Status     int
	StatusText string
	Headers    map[string]string
	Body       []byte
	DurationMS int
}

// AuthConfig defines supported outbound auth injection strategies.
type AuthConfig struct {
	Type string

	// API key / bearer
	HeaderName string
	SecretRef  string

	// Basic auth
	UsernameRef string
	PasswordRef string

	// OAuth2 client credentials
	TokenEndpoint   string
	ClientIDRef     string
	ClientSecretRef string
	Scopes          []string

	// HMAC
	Algorithm       string
	SignatureHeader string
}
