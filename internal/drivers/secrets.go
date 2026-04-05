package drivers

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

type SecretStore interface {
	Get(ctx context.Context, tenantID, key string) (string, error)
}

func ResolveSecrets(ctx context.Context, tenantID string, secretStore SecretStore, config interface{}) error {
	if secretStore == nil || config == nil {
		return nil
	}
	rv := reflect.ValueOf(config)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("config must be a non-nil pointer")
	}
	return resolveStructSecrets(ctx, tenantID, secretStore, rv.Elem())
}

func resolveStructSecrets(ctx context.Context, tenantID string, secretStore SecretStore, v reflect.Value) error {
	if !v.IsValid() {
		return nil
	}
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}
		if fv.Kind() == reflect.Struct || (fv.Kind() == reflect.Pointer && !fv.IsNil() && fv.Elem().Kind() == reflect.Struct) {
			if err := resolveStructSecrets(ctx, tenantID, secretStore, fv); err != nil {
				return err
			}
			continue
		}
		if fv.Kind() != reflect.String {
			continue
		}

		tagSecret := strings.EqualFold(field.Tag.Get("secret"), "true")
		namedSecret := strings.HasSuffix(strings.ToLower(field.Name), "secret")
		if !tagSecret && !namedSecret {
			continue
		}

		secretRef := strings.TrimSpace(fv.String())
		if secretRef == "" {
			continue
		}
		key := strings.TrimPrefix(secretRef, "secret://")
		key = strings.TrimPrefix(key, "secret:")
		resolved, err := secretStore.Get(ctx, tenantID, key)
		if err != nil {
			return fmt.Errorf("resolve secret %q: %w", key, err)
		}
		fv.SetString(resolved)
	}
	return nil
}
