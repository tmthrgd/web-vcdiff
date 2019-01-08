package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
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

	mux.Handle("/", http.FileServer(http.Dir("html")))

	fmt.Printf("Listening on %s\n", *addr)
	panic(http.ListenAndServe(*addr, mux))
}
