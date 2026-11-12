package web

import "embed"

// Files contains the complete, locally served browser application.
//
//go:embed index.html app.js style.css vendor/*
var Files embed.FS
