package main

import (
	"embed"
	"io/fs"
	"net/http"
)

// The release binaries are distributed as a single executable. Embed the
// generated frontend so starting the executable from an installer shortcut
// does not depend on the process working directory containing static/.
//
//go:embed static
var embeddedWeb embed.FS

var webFiles, _ = fs.Sub(embeddedWeb, "static")

func serveWebPage(w http.ResponseWriter, r *http.Request, name string) {
	http.ServeFileFS(w, r, webFiles, name)
}

func webAssetHandler() http.Handler {
	return http.FileServer(http.FS(webFiles))
}
