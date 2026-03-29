package cases

import (
	"database/sql/driver"
	"strings"
)

func (a pqStringArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	parts := make([]string, 0, len(a))
	for _, v := range a {
		escaped := strings.ReplaceAll(v, `"`, `\\"`)
		parts = append(parts, `"`+escaped+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}

func (a pqUUIDArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return "{}", nil
	}
	parts := make([]string, 0, len(a))
	for _, id := range a {
		parts = append(parts, `"`+id.String()+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}", nil
}
