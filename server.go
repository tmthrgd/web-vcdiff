package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	handlers "github.com/tmthrgd/httphandlers"
)

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
	mux.Handle("/test.diff", handlers.SetHeaders(html, map[string]string{
		"Content-Diff-Encoding":   "vcdiff",
		"Content-Diff-Dictionary": "/test.dict#b47cc0f104b6",
	}))
	mux.Handle("/test.dict", handlers.SetHeader(html, "Vary", "Expect-Diff-Hash"))

	fmt.Printf("Listening on %s\n", *addr)
	panic(http.ListenAndServe(*addr, mux))
}
