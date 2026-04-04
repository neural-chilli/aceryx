package wasm

import (
	"encoding/json"
	"fmt"
)

const ABIVersion = 1

type hostEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

func decodeEnvelope(raw []byte, out interface{}) error {
	if len(raw) == 0 {
		return fmt.Errorf("empty host response")
	}
	env := hostEnvelope{}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode host response: %w", err)
	}
	if !env.OK {
		if env.Error == "" {
			return fmt.Errorf("host call failed")
		}
		return fmt.Errorf(env.Error)
	}
	if out == nil {
		return nil
	}
	if len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decode host data: %w", err)
	}
	return nil
}
