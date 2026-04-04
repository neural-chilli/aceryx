package hostfns

import "fmt"

type CaseDataAccessor interface {
	Get(path string) ([]byte, error)
	Set(path string, value []byte) error
}

type CaseDataHost struct {
	Accessor CaseDataAccessor
}

func (h *CaseDataHost) CaseGet(path string) ([]byte, error) {
	if h.Accessor == nil {
		return nil, fmt.Errorf("case data accessor not configured")
	}
	return h.Accessor.Get(path)
}

func (h *CaseDataHost) CaseSet(path string, value []byte) error {
	if h.Accessor == nil {
		return fmt.Errorf("case data accessor not configured")
	}
	return h.Accessor.Set(path, value)
}
