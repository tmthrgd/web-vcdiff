package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	handlers "github.com/tmthrgd/httphandlers"
	vcdiff "github.com/tmthrgd/web-vcdiff/go"
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
	mux.Handle("/build/vcddec_glue.cpp", root)

	if dir := os.Getenv("EMSCRIPTEN"); dir != "" {
		dir += "/"
		mux.Handle(dir, http.StripPrefix(dir,
			http.FileServer(http.Dir(dir))))
	}

	js := http.FileServer(http.Dir("js"))
	mux.Handle("/decoder.js", js)
	mux.Handle("/vcdiff.js", js)

	d, err := vcdiff.ReadDictionary("html/test.dict")
	if err != nil {
		log.Fatal(err)
	}

	ds := vcdiff.FixedDictionary(d)
	mux.Handle(vcdiff.DictionaryHandlerPath,
		vcdiff.DictionaryHandler(ds))

	html := http.FileServer(http.Dir("html"))
	mux.Handle("/", html)
	mux.Handle("/test.txt", vcdiff.Handler(ds, html))

	log := handlers.AccessLog(mux, nil)

	fmt.Printf("Listening on %s\n", *addr)
	panic(http.ListenAndServe(*addr, log))
}
