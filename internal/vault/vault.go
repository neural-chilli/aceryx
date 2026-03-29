// Package vault implements the document storage layer with content-hash
// addressing, metadata management, and RBAC-controlled access.
package vault

// VaultStore defines the interface for document storage backends.
type VaultStore interface {
	Put(tenantID, hash, ext string, data []byte) (uri string, err error)
	Get(uri string) ([]byte, error)
	Delete(uri string) error
	SignedURL(uri string, expirySeconds int) (string, error)
}
