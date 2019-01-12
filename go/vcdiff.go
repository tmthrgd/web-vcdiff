package vcdiff

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	handlers "github.com/tmthrgd/httphandlers"
	"github.com/tmthrgd/httputils"
	openvcdiff "github.com/tmthrgd/web-vcdiff/go/internal/open-vcdiff"
)

func Handler(h http.Handler, opts ...Option) http.Handler {
	var c config
	for _, opt := range opts {
		opt(&c)
	}

	if c.dictionary == nil {
		panic("web-vcdiff: missing one of WithDictionary, WithFixedDictionary or WithReadFixedDictionary")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleError := func(fmt string, err error) {
			httputils.RequestLogf(r, fmt, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
		}

		hdr := w.Header()
		hdr.Add("Vary", "Accept-Diff-Encoding")

		if !strings.EqualFold(r.Header.Get("Accept-Diff-Encoding"), "vcdiff") {
			h.ServeHTTP(w, r)
			return
		}

		dict, dictURL, err := c.dictionary(r)
		if err != nil {
			handleError("dictionary callback failed: %v", err)
			return
		}

		digest := sha256.Sum256(dict)
		dictURL = fmt.Sprintf("%s#%x", dictURL, digest[:6])

		hdr.Set("Content-Diff-Encoding", "vcdiff")
		hdr.Set("Content-Diff-Dictionary", dictURL)

		dw := &responseWriter{
			ResponseWriter: w,
			req:            r,

			dictBytes: dict,
		}
		defer func() {
			dw.closeVCDIFF()
			if dw.err != nil {
				httputils.RequestLogf(r, "error VCDIFF compressing response: %v", dw.err)
			}
		}()

		var rw http.ResponseWriter = dw

		_, cok := w.(http.CloseNotifier)
		_, pok := w.(http.Pusher)
		switch {
		case cok && pok:
			rw = closeNotifyPusherResponseWriter{dw}
		case cok:
			rw = closeNotifyResponseWriter{dw}
		case pok:
			rw = pusherResponseWriter{dw}
		}

		h.ServeHTTP(rw, r)
	})
}

func DictionaryHandler(h http.Handler) http.Handler {
	return handlers.AddHeader(h, "Vary", "Expect-Diff-Hash")
}

type config struct {
	dictionary func(*http.Request) ([]byte, string, error)
}

type Option func(*config)

func WithReadFixedDictionary(dictionaryPath, dictionaryURL string) Option {
	dict, err := ioutil.ReadFile(dictionaryPath)
	return WithDictionary(func(*http.Request) ([]byte, string, error) {
		return dict, dictionaryURL, err
	})
}

func WithFixedDictionary(dictionary []byte, dictionaryURL string) Option {
	return WithDictionary(func(*http.Request) ([]byte, string, error) {
		return dictionary, dictionaryURL, nil
	})
}

func WithDictionary(dictionary func(*http.Request) (dictionary []byte, dictionaryURL string, err error)) Option {
	return func(c *config) {
		if c.dictionary != nil {
			panic("web-vcdiff: only one of WithDictionary, WithFixedDictionary or WithReadFixedDictionary may be specified")
		}

		c.dictionary = dictionary
	}
}

type responseWriter struct {
	http.ResponseWriter
	req *http.Request

	dictBytes []byte

	dict *openvcdiff.Dictionary
	enc  *openvcdiff.Encoder
	err  error

	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	if rw.err != nil || rw.wroteHeader {
		return
	}
	rw.wroteHeader = true

	rw.Header().Del("Content-Length")
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	if rw.enc == nil && rw.err == nil {
		rw.startVCDIFF()
	}
	if rw.err != nil {
		return 0, rw.err
	}

	n, err := rw.enc.Write(p)
	if err != nil {
		// We can't send an error response so bail here. For HTTP/2
		// this will send a RST_STREAM frame.
		rw.err = err
		panic(http.ErrAbortHandler)
	}

	return n, err
}

func (rw *responseWriter) startVCDIFF() {
	rw.dict, rw.err = openvcdiff.NewDictionary(rw.dictBytes)
	if rw.err == nil {
		rw.enc, rw.err = openvcdiff.NewEncoder(rw.ResponseWriter, rw.dict,
			openvcdiff.VCDFormatInterleaved)
	}

	if rw.err != nil && !rw.wroteHeader {
		http.Error(rw.ResponseWriter, http.StatusText(http.StatusInternalServerError),
			http.StatusInternalServerError)
	} else if rw.err != nil {
		// We can't send an error response so bail here. For HTTP/2
		// this will send a RST_STREAM frame.
		panic(http.ErrAbortHandler)
	}
}

func (rw *responseWriter) closeVCDIFF() {
	if rw.enc == nil {
		return
	}

	closeErr := rw.enc.Close()
	if rw.err == nil {
		rw.err = closeErr
	}

	rw.dict.Destroy()
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type (
	// Each of these structs is intentionally small (1 pointer wide) so
	// as to fit inside an interface{} without causing an allocaction.
	closeNotifyResponseWriter       struct{ *responseWriter }
	pusherResponseWriter            struct{ *responseWriter }
	closeNotifyPusherResponseWriter struct{ *responseWriter }
)

var (
	_ http.CloseNotifier = closeNotifyResponseWriter{}
	_ http.CloseNotifier = closeNotifyPusherResponseWriter{}
	_ http.Pusher        = pusherResponseWriter{}
	_ http.Pusher        = closeNotifyPusherResponseWriter{}
)

func (w closeNotifyResponseWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

func (w closeNotifyPusherResponseWriter) CloseNotify() <-chan bool {
	return w.ResponseWriter.(http.CloseNotifier).CloseNotify()
}

func (w pusherResponseWriter) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}

func (w closeNotifyPusherResponseWriter) Push(target string, opts *http.PushOptions) error {
	return w.ResponseWriter.(http.Pusher).Push(target, opts)
}
