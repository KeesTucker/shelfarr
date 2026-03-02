package main

import (
	"net/http"
	"os"
)

// spaHandler serves the frontend from dir. Any path that doesn't correspond
// to a real file falls back to index.html so client-side routing works.
func spaHandler(dir string) http.Handler {
	root := http.Dir(dir)
	fileServer := http.FileServer(root)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := root.Open(r.URL.Path)
		if err != nil {
			if os.IsNotExist(err) {
				// SPA fallback: let the client router handle unknown paths.
				r2 := r.Clone(r.Context())
				r2.URL.Path = "/"
				fileServer.ServeHTTP(w, r2)
				return
			}
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, r)
	})
}
