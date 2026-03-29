package cases

import (
	"fmt"
	"reflect"
	"regexp"
)

func ValidateCaseData(schema CaseTypeSchema, data map[string]interface{}) []ValidationError {
	errors := make([]ValidationError, 0)
	for field, def := range schema.Fields {
		v, ok := data[field]
		if !ok || v == nil {
			if def.Required {
				errors = append(errors, ValidationError{Field: field, Rule: "required", Message: "is required"})
			}
			continue
		}
		errors = append(errors, validateField(field, def, v)...)
	}
	return errors
}

func validateField(path string, def SchemaField, value interface{}) []ValidationError {
	errs := make([]ValidationError, 0)

	if !typeMatches(def.Type, value) {
		errs = append(errs, ValidationError{Field: path, Rule: "type", Message: fmt.Sprintf("must be type %s", def.Type), Value: value})
		return errs
	}

	if s, ok := value.(string); ok {
		if def.Pattern != "" {
			re, err := regexp.Compile(def.Pattern)
			if err != nil || !re.MatchString(s) {
				errs = append(errs, ValidationError{Field: path, Rule: "pattern", Message: "does not match required pattern", Value: value})
			}
		}
		if def.MinLength != nil && len(s) < *def.MinLength {
			errs = append(errs, ValidationError{Field: path, Rule: "minLength", Message: fmt.Sprintf("must be at least %d chars", *def.MinLength), Value: value})
		}
		if def.MaxLength != nil && len(s) > *def.MaxLength {
			errs = append(errs, ValidationError{Field: path, Rule: "maxLength", Message: fmt.Sprintf("must be at most %d chars", *def.MaxLength), Value: value})
		}
	}

	if n, ok := toFloat(value); ok {
		if def.Min != nil && n < *def.Min {
			errs = append(errs, ValidationError{Field: path, Rule: "min", Message: fmt.Sprintf("must be at least %v", *def.Min), Value: value})
		}
		if def.Max != nil && n > *def.Max {
			errs = append(errs, ValidationError{Field: path, Rule: "max", Message: fmt.Sprintf("must be at most %v", *def.Max), Value: value})
		}
	}

	if len(def.Enum) > 0 {
		match := false
		for _, allowed := range def.Enum {
			if reflect.DeepEqual(allowed, value) {
				match = true
				break
			}
		}
		if !match {
			errs = append(errs, ValidationError{Field: path, Rule: "enum", Message: "must be one of allowed values", Value: value})
		}
	}

	if def.Type == "object" {
		m, _ := value.(map[string]interface{})
		for k, child := range def.Properties {
			cv, ok := m[k]
			childPath := path + "." + k
			if !ok || cv == nil {
				if child.Required {
					errs = append(errs, ValidationError{Field: childPath, Rule: "required", Message: "is required"})
				}
				continue
			}
			errs = append(errs, validateField(childPath, child, cv)...)
		}
	}

	return errs
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case jsonNumber:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

type jsonNumber interface {
	Float64() (float64, error)
}

func typeMatches(t string, value interface{}) bool {
	switch t {
	case "string", "text":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := toFloat(value)
		return ok
	case "integer":
		switch f := value.(type) {
		case int, int32, int64:
			return true
		case float64:
			return float64(int64(f)) == f
		default:
			return false
		}
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	case "array":
		rv := reflect.ValueOf(value)
		return rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array
	default:
		return true
	}
}
