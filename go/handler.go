package vcdiff

import (
	"net/http"

	"github.com/tmthrgd/httputils"
	openvcdiff "github.com/tmthrgd/web-vcdiff/go/internal/open-vcdiff"
	"golang.org/x/net/http/httpguts"
)

func Handler(d Dictionaries, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Diff-Encoding")

		if !httpguts.HeaderValuesContainsToken(r.Header["Accept-Diff-Encoding"], "vcdiff") {
			h.ServeHTTP(w, r)
			return
		}

		dict, err := d.Select(r)
		if err != nil {
			httputils.RequestLogf(r, "dictionary callback failed: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			return
		} else if dict == nil {
			h.ServeHTTP(w, r)
			return
		}

		if dict.ID != newDictionaryID(dict.Data) {
			panic("vcdiff: invalid dictionary identifier")
		}

		dw := &responseWriter{
			ResponseWriter: w,
			req:            r,

			selectedDict: dict,
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

type responseWriter struct {
	http.ResponseWriter
	req *http.Request

	selectedDict *Dictionary

	dict *openvcdiff.Dictionary
	enc  *openvcdiff.Encoder
	err  error

	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	if rw.wroteHeader {
		return
	}
	rw.wroteHeader = true

	hdr := rw.Header()
	hdr.Del("Content-Length")
	hdr.Del("Etag")

	// RFC 7232 section 4.1:
	//  a sender SHOULD NOT generate representation metadata other than the
	//  above listed fields unless said metadata exists for the purpose of
	//  guiding cache updates (e.g., Last-Modified might be useful if the
	//  response does not have an ETag field).
	if statusCode != http.StatusNotModified {
		hdr.Set("Content-Diff-Encoding", "vcdiff")
		hdr.Set("Content-Diff-Dictionary", rw.selectedDict.ID.encode())
	}

	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(p []byte) (int, error) {
	if !rw.wroteHeader {
		rw.sniffContentType(p)
		rw.WriteHeader(http.StatusOK)
	}
	if len(p) == 0 {
		return 0, rw.err
	}
	if rw.enc == nil && rw.err == nil {
		rw.startVCDIFF()
	}
	if rw.err != nil {
		return 0, rw.err
	}

	n, err := rw.enc.Write(p)
	rw.err = err
	return n, err
}

func (rw *responseWriter) sniffContentType(p []byte) {
	hdr := rw.Header()
	_, haveType := hdr["Content-Type"]
	if haveType || len(p) == 0 {
		return
	}

	hdr.Set("Content-Type", http.DetectContentType(p))
}

func (rw *responseWriter) startVCDIFF() {
	rw.dict, rw.err = openvcdiff.NewDictionary(rw.selectedDict.Data)
	if rw.err != nil {
		return
	}

	rw.enc, rw.err = openvcdiff.NewEncoder(rw.ResponseWriter, rw.dict,
		openvcdiff.VCDFormatInterleaved)
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
