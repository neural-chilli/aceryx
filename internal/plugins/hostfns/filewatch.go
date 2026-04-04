package hostfns

import (
	"fmt"

	"github.com/neural-chilli/aceryx/internal/plugins"
)

func FileWatch(_ string, _ []byte, _ string) (plugins.FileEvent, error) {
	return plugins.FileEvent{}, fmt.Errorf("not implemented")
}
