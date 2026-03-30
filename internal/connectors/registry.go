package connectors

import (
	"sort"
	"sync"
)

type Registry struct {
	mu         sync.RWMutex
	connectors map[string]Connector
}

func NewRegistry() *Registry {
	return &Registry{connectors: make(map[string]Connector)}
}

func (r *Registry) Register(c Connector) {
	if c == nil {
		return
	}
	meta := c.Meta()
	if meta.Key == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connectors[meta.Key] = c
}

func (r *Registry) Get(key string) (Connector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.connectors[key]
	return c, ok
}

func (r *Registry) List() []ConnectorMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnectorMeta, 0, len(r.connectors))
	for _, c := range r.connectors {
		out = append(out, c.Meta())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func (r *Registry) Describe() []ConnectorDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ConnectorDescriptor, 0, len(r.connectors))
	for _, c := range r.connectors {
		actions := c.Actions()
		actionSummaries := make([]ActionSummary, 0, len(actions))
		for _, a := range actions {
			actionSummaries = append(actionSummaries, ActionSummary{
				Key:          a.Key,
				Name:         a.Name,
				Description:  a.Description,
				InputSchema:  a.InputSchema,
				OutputSchema: a.OutputSchema,
			})
		}
		out = append(out, ConnectorDescriptor{
			Meta:     c.Meta(),
			Auth:     c.Auth(),
			Triggers: c.Triggers(),
			Actions:  actionSummaries,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.Key < out[j].Meta.Key })
	return out
}

func (r *Registry) GetAction(connectorKey, actionKey string) (ActionSpec, bool) {
	c, ok := r.Get(connectorKey)
	if !ok {
		return ActionSpec{}, false
	}
	for _, a := range c.Actions() {
		if a.Key == actionKey {
			return a, true
		}
	}
	return ActionSpec{}, false
}
