package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func NormalizeKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "/")
	return key
}

func JoinKey(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = NormalizeKey(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, "/")
}

func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
