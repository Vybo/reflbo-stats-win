// Package web is the settings page (spec §7), embedded in the exe.
package web

import "embed"

//go:embed index.html app.js style.css
var Files embed.FS
