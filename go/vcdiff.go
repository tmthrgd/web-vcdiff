package vcdiff

import (
	"net/http"
	"net/url"
	"strings"
)

func Handler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := w.Header()
		hdr.Add("Vary", "Accept-Diff-Encoding")

		if !strings.EqualFold(r.Header.Get("Accept-Diff-Encoding"), "vcdiff") {
			h.ServeHTTP(w, r)
			return
		}

		hdr.Set("Content-Diff-Encoding", "vcdiff")
		hdr.Set("Content-Diff-Dictionary", "/test.dict#b47cc0f104b6")

		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path += ".diff"

		h.ServeHTTP(w, r2)
	})
}
