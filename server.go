package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	handlers "github.com/tmthrgd/httphandlers"
)

func vcdiffHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := w.Header()
		hdr.Set("Vary", "Accept-Diff-Encoding")

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

func main() {
	addr := flag.String("addr", ":8090", "the address to listen on")
	flag.Parse()

	mux := http.NewServeMux()

	build := http.FileServer(http.Dir("build"))
	mux.Handle("/vcddec.js", build)
	mux.Handle("/vcddec.wasm", build)
	mux.Handle("/vcddec.wasm.map", build)

	root := http.FileServer(http.Dir(""))
	mux.Handle("/open-vcdiff/", root)
	mux.Handle("/output_string.h", root)
	mux.Handle("/build/vcddec_glue.cpp", root)

	if dir := os.Getenv("EMSCRIPTEN"); dir != "" {
		dir += "/"
		mux.Handle(dir, http.StripPrefix(dir,
			http.FileServer(http.Dir(dir))))
	}

	html := http.FileServer(http.Dir("html"))
	mux.Handle("/", html)
	mux.Handle("/test.txt", vcdiffHandler(html))
	mux.Handle("/test.dict", handlers.SetHeader(html, "Vary", "Expect-Diff-Hash"))

	fmt.Printf("Listening on %s\n", *addr)
	panic(http.ListenAndServe(*addr, mux))
}
