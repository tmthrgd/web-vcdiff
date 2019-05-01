package vcdiff

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/tmthrgd/httputils"
	openvcdiff "github.com/tmthrgd/web-vcdiff/go/internal/open-vcdiff"
	"golang.org/x/net/http/httpguts"
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
		hdr := w.Header()
		hdr.Add("Vary", "Accept-Diff-Encoding, Accept-Diff-Dictionaries")

		if !strings.EqualFold(r.Header.Get("Accept-Diff-Encoding"), "vcdiff") {
			h.ServeHTTP(w, r)
			return
		}

		dict, err := c.dictionary(r)
		if err != nil {
			httputils.RequestLogf(r, "dictionary callback failed: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			return
		}

		dw := &responseWriter{
			rw:  w,
			req: r,

			bw: w,

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

type config struct {
	dictionary func(*http.Request) ([]byte, error)
}

type Option func(*config)

func WithReadFixedDictionary(dictionaryPath string) Option {
	dict, err := ioutil.ReadFile(dictionaryPath)
	return WithDictionary(func(*http.Request) ([]byte, error) {
		return dict, err
	})
}

func WithFixedDictionary(dictionary []byte) Option {
	return WithDictionary(func(*http.Request) ([]byte, error) {
		return dictionary, nil
	})
}

func WithDictionary(dictionary func(*http.Request) (dictionary []byte, err error)) Option {
	return func(c *config) {
		if c.dictionary != nil {
			panic("web-vcdiff: only one of WithDictionary, WithFixedDictionary or WithReadFixedDictionary may be specified")
		}

		c.dictionary = dictionary
	}
}

type responseWriter struct {
	rw  http.ResponseWriter
	req *http.Request

	mw *multipart.Writer
	bw io.Writer

	dictBytes []byte

	dict *openvcdiff.Dictionary
	enc  *openvcdiff.Encoder
	err  error

	wroteHeader bool
}

func (rw *responseWriter) Header() http.Header {
	return rw.rw.Header()
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	if rw.err != nil || rw.wroteHeader {
		return
	}
	rw.wroteHeader = true

	digest := sha256.Sum256(rw.dictBytes)
	hexDigest := hex.EncodeToString(digest[:8])

	hdr := rw.Header()
	hdr.Del("Content-Length")
	hdr.Del("Etag")

	if !bodyAllowedForStatus(statusCode) ||
		httpguts.HeaderValuesContainsToken(
			rw.req.Header["Accept-Diff-Dictionaries"], hexDigest) {
		hdr.Set("Content-Diff-Encoding", "vcdiff:"+hexDigest)

		rw.rw.WriteHeader(statusCode)
		return
	}

	rw.mw = multipart.NewWriter(rw.rw)

	cts := hdr["Content-Type"]
	hdr.Set("Content-Type", rw.mw.FormDataContentType())

	hdr.Set("Content-Diff-Encoding", "vcdiff:*"+hexDigest)

	rw.rw.WriteHeader(statusCode)

	pw, err := rw.mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":        {"application/octet-stream"},
		"Content-Disposition": {`form-data; name=dict; filename=d`},
	})
	if err != nil {
		return
	}

	_, rw.err = pw.Write(rw.dictBytes)
	if rw.err != nil {
		return
	}

	rw.bw, rw.err = rw.mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":        cts,
		"Content-Disposition": {`form-data; name=body; filename=b`},
	})
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}

	if rw.enc == nil && rw.err == nil {
		rw.startVCDIFF()
	}
	if rw.err != nil {
		return 0, rw.err
	}

	n, err := rw.enc.Write(p)
	if err != nil {
		rw.err = err
	}

	return n, err
}

func (rw *responseWriter) startVCDIFF() {
	rw.dict, rw.err = openvcdiff.NewDictionary(rw.dictBytes)
	if rw.err != nil {
		return
	}

	rw.enc, rw.err = openvcdiff.NewEncoder(rw.bw, rw.dict,
		openvcdiff.VCDFormatInterleaved)
}

func (rw *responseWriter) closeVCDIFF() {
	if rw.enc != nil {
		closeErr := rw.enc.Close()
		if rw.err == nil {
			rw.err = closeErr
		}

		rw.dict.Destroy()
	}

	if rw.mw != nil {
		closeErr := rw.mw.Close()
		if rw.err == nil {
			rw.err = closeErr
		}
	}
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.rw.(http.Flusher); ok {
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
	return w.rw.(http.CloseNotifier).CloseNotify()
}

func (w closeNotifyPusherResponseWriter) CloseNotify() <-chan bool {
	return w.rw.(http.CloseNotifier).CloseNotify()
}

func (w pusherResponseWriter) Push(target string, opts *http.PushOptions) error {
	return w.rw.(http.Pusher).Push(target, opts)
}

func (w closeNotifyPusherResponseWriter) Push(target string, opts *http.PushOptions) error {
	return w.rw.(http.Pusher).Push(target, opts)
}
