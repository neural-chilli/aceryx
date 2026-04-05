package hostfns

import (
	"context"
	nethttp "net/http"
	"time"

	httpfw "github.com/neural-chilli/aceryx/internal/http"
	"github.com/neural-chilli/aceryx/internal/plugins"
)

type HTTPHost struct {
	ClientManager *httpfw.ClientManager
	TenantID      string
	PluginID      string
	AuthConfig    *httpfw.AuthConfig
	Ctx           context.Context
}

func NewHTTPHost(client *nethttp.Client, allowDomains []string, maxTimeout time.Duration) *HTTPHost {
	manager := httpfw.NewClientManager(httpfw.ClientConfig{SystemMaxTimeout: maxTimeout})
	if client != nil {
		manager.SetHTTPClient(client)
	}
	validator := httpfw.NewURLValidator(false)
	validator.SetAllowlist("default", allowDomains)
	manager.SetValidator(validator)
	return &HTTPHost{
		ClientManager: manager,
		TenantID:      "default",
		Ctx:           context.Background(),
	}
}

func (h *HTTPHost) HTTPRequest(method, rawURL string, headers map[string]string, body []byte, timeoutMS int) (plugins.HTTPResponse, error) {
	ctx := context.Background()
	if h != nil && h.Ctx != nil {
		ctx = h.Ctx
	}
	resp, err := h.ClientManager.Execute(ctx, httpfw.PluginHTTPRequest{
		TenantID:   h.TenantID,
		PluginID:   h.PluginID,
		Method:     method,
		URL:        rawURL,
		Headers:    headers,
		Body:       body,
		TimeoutMS:  timeoutMS,
		AuthConfig: h.AuthConfig,
	})
	if err != nil {
		return plugins.HTTPResponse{}, err
	}
	return plugins.HTTPResponse{
		StatusCode: resp.Status,
		Headers:    resp.Headers,
		Body:       resp.Body,
	}, nil
}
