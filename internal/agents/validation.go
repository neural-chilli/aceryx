package agents

import (
	"fmt"
	"reflect"
)

func validateOutputAgainstSchema(output map[string]any, schema map[string]FieldDef) error {
	if len(schema) == 0 {
		return nil
	}
	for field, def := range schema {
		value, ok := output[field]
		if !ok {
			return fmt.Errorf("missing required field %q", field)
		}
		if err := validateField(field, value, def); err != nil {
			return err
		}
	}
	return nil
}

func validateField(field string, value any, def FieldDef) error {
	kind := def.Type
	if kind == "" {
		kind = "string"
	}
	switch kind {
	case "text", "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field %s expected string", field)
		}
	case "number":
		n, ok := asFloat(value)
		if !ok {
			return fmt.Errorf("field %s expected number", field)
		}
		if def.Min != nil && n < *def.Min {
			return fmt.Errorf("field %s below min", field)
		}
		if def.Max != nil && n > *def.Max {
			return fmt.Errorf("field %s above max", field)
		}
	case "array":
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return fmt.Errorf("field %s expected array", field)
		}
		if def.Items != "" {
			for i := 0; i < rv.Len(); i++ {
				if err := validateField(fmt.Sprintf("%s[%d]", field, i), rv.Index(i).Interface(), FieldDef{Type: def.Items}); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("field %s unsupported schema type %s", field, kind)
	}

	if len(def.Enum) > 0 {
		matched := false
		for _, option := range def.Enum {
			if reflect.DeepEqual(option, value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("field %s must be one of enum values", field)
		}
	}
	return nil
}
