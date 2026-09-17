// Package web embeds the kit's browser page: index.html plus app.js, the
// page's only script, and three stylesheets — rise-x-ui.css (the vendored
// Rise-X design system, a compiled Tailwind build), kit.css (the component
// bases that build omits) and style.css (the few rules only this app needs).
package web

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed index.html app.js style.css kit.css rise-x-ui.css favicon.ico favicons
var files embed.FS

// AssetNames are the static files Assets serves, so the router and the embed
// list can't drift apart. favicon.ico is listed at the root because that's
// the path browsers request regardless of what the page's <link> tags say.
var AssetNames = []string{
	"app.js", "style.css", "kit.css", "rise-x-ui.css",
	"favicon.ico", "favicons/favicon-16x16.png", "favicons/favicon-32x32.png", "favicons/apple-touch-icon.png",
}

// tokenPlaceholder is replaced with the server's per-process CSRF token when
// index.html is served, so app.js can read it without an extra handshake.
const tokenPlaceholder = "__RISEX_TOKEN__"

// Index renders index.html with the CSRF token injected.
func Index(token string) []byte {
	data, err := files.ReadFile("index.html")
	if err != nil {
		return nil
	}
	return []byte(strings.Replace(string(data), tokenPlaceholder, token, 1))
}

// Assets serves the files named in AssetNames as embedded static files.
func Assets() http.Handler {
	return http.FileServerFS(files)
}
