package channels

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

type AdapterEngine struct {
	gojaPool sync.Pool
}

func NewAdapterEngine() *AdapterEngine {
	ae := &AdapterEngine{}
	ae.gojaPool.New = func() any { return goja.New() }
	return ae
}

func (ae *AdapterEngine) Apply(config AdapterConfig, inboundRaw []byte) ([]byte, error) {
	if ae == nil {
		ae = NewAdapterEngine()
	}
	inbound := map[string]any{}
	if len(inboundRaw) > 0 {
		if err := jsonUnmarshalMap(inboundRaw, &inbound); err != nil {
			return nil, fmt.Errorf("decode inbound payload: %w", err)
		}
	}
	out := map[string]any{}
	for _, mapping := range config.Mappings {
		target := normalizeCaseDataPath(mapping.Target)
		if target == "" {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(mapping.Type)) {
		case "", "direct":
			source := normalizePayloadPath(mapping.Source)
			value, ok := lookupPath(inbound, source)
			if !ok {
				continue
			}
			setPath(out, target, value)
		case "constant":
			setPath(out, target, mapping.Value)
		case "expression":
			value, err := ae.evaluate(mapping.Expression, inbound)
			if err != nil {
				return nil, fmt.Errorf("evaluate adapter expression for %s: %w", mapping.Target, err)
			}
			setPath(out, target, value)
		default:
			return nil, fmt.Errorf("unsupported mapping type: %s", mapping.Type)
		}
	}
	return jsonMarshal(out)
}

func (ae *AdapterEngine) evaluate(expression string, inbound map[string]any) (any, error) {
	vm := ae.gojaPool.Get().(*goja.Runtime)
	defer ae.gojaPool.Put(vm)

	_ = vm.Set("payload", inbound)
	defer func() {
		_ = vm.Set("payload", nil)
		vm.ClearInterrupt()
	}()

	timer := time.AfterFunc(time.Second, func() {
		vm.Interrupt(fmt.Errorf("expression evaluation timed out"))
	})
	defer timer.Stop()

	expr := strings.TrimSpace(expression)
	if expr == "" {
		return nil, nil
	}
	result, err := vm.RunString("(function(){ return (" + expr + "); })()")
	if err != nil {
		return nil, err
	}
	return result.Export(), nil
}

func normalizeCaseDataPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "case.data.")
	path = strings.TrimPrefix(path, "data.")
	path = strings.TrimPrefix(path, "case.")
	return strings.Trim(path, ".")
}

func normalizePayloadPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "payload.")
	return strings.Trim(path, ".")
}
