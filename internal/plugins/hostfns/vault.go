package hostfns

import "fmt"

type VaultAccessor interface {
	Read(documentID string) ([]byte, error)
	Write(filename, contentType string, data []byte) (string, error)
}

type VaultHost struct {
	Accessor VaultAccessor
}

func (h *VaultHost) VaultRead(documentID string) ([]byte, error) {
	if h.Accessor == nil {
		return nil, fmt.Errorf("vault accessor not configured")
	}
	return h.Accessor.Read(documentID)
}

func (h *VaultHost) VaultWrite(filename, contentType string, data []byte) (string, error) {
	if h.Accessor == nil {
		return "", fmt.Errorf("vault accessor not configured")
	}
	return h.Accessor.Write(filename, contentType, data)
}
