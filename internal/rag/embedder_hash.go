package rag

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
)

type HashEmbedder struct {
	dims int
}

func NewHashEmbedder(dims int) *HashEmbedder {
	if dims <= 0 {
		dims = 64
	}
	return &HashEmbedder{dims: dims}
}

func (e *HashEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		h := sha256.Sum256([]byte(text))
		vec := make([]float32, e.dims)
		for i := 0; i < e.dims; i++ {
			off := (i * 4) % len(h)
			v := binary.BigEndian.Uint32(h[off : off+4])
			vec[i] = float32(v%1000) / 1000.0
		}
		out = append(out, vec)
	}
	return out, nil
}

func (e *HashEmbedder) Dimensions() int   { return e.dims }
func (e *HashEmbedder) ModelName() string { return "hash-embedding-v1" }
