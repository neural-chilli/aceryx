package frontendassets

import (
	"embed"
	"io/fs"
)

// Dist embeds the production frontend bundle.
//
//go:embed all:dist
var Dist embed.FS

func DistFS() fs.FS {
	sub, err := fs.Sub(Dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
