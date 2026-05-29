package web

import (
	"embed"
	"io/fs"
)

//go:embed out out/_next
var embedded embed.FS

func StaticFS() (fs.FS, error) {
	return fs.Sub(embedded, "out")
}
