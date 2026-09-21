package main

import (
	"net/http"
	"net/url"
	"strings"
)

// assetHandler serves what the frontend loads by URL rather than by RPC:
// catalog artwork under /mediaitems/ and profile pictures under
// /profiles/picture/<slug>. The desktop asset server and -server mount the
// same handler, so the two never drift.
func assetHandler(app *App) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mediaitems/", http.StripPrefix("/mediaitems/", artworkHandler(
		func() string { return app.metadataPath },
		thumbCacheDir(),
	)))
	mux.HandleFunc(profilePictureRoute, func(w http.ResponseWriter, r *http.Request) {
		slug, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, profilePictureRoute))
		if err != nil || slug == "" || strings.ContainsAny(slug, "/\\") {
			http.NotFound(w, r)
			return
		}
		path := app.profilePicture(slug)
		if path == "" {
			http.NotFound(w, r)
			return
		}
		// The URL carries the file's modification time, so a picture that has
		// not changed can be cached for good and one that has gets a new URL.
		w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
		http.ServeFile(w, r, path)
	})
	return mux
}
