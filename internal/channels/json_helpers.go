package channels

import "encoding/json"

func jsonMarshal(v any) ([]byte, error) {
	if v == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(v)
}

func jsonUnmarshalMap(raw []byte, out *map[string]any) error {
	if len(raw) == 0 {
		*out = map[string]any{}
		return nil
	}
	if *out == nil {
		*out = map[string]any{}
	}
	return json.Unmarshal(raw, out)
}
