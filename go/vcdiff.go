package vcdiff

import (
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/tmthrgd/httputils"
)

var vcdiffPath, _ = exec.LookPath("vcdiff")

func init() {
	if vcdiffPath == "" {
		vcdiffPath = "vcdiff"
	}
}

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

		dw := &responseWriter{
			ResponseWriter: w,
			req:            r,
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

	cmd   *exec.Cmd
	stdin io.WriteCloser
	err   error

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
	if rw.cmd == nil && rw.err == nil {
		rw.startVCDIFF()
	}
	if rw.err != nil {
		return 0, rw.err
	}

	n, err := rw.stdin.Write(p)
	if err != nil {
		// We can't send an error response so bail here. For HTTP/2
		// this will send a RST_STREAM frame.
		rw.err = err
		panic(http.ErrAbortHandler)
	}

	return n, err
}

func (rw *responseWriter) startVCDIFF() {
	cmd := exec.CommandContext(rw.req.Context(), vcdiffPath, "encode",
		"-dictionary", "html/test.dict", "-interleaved")
	cmd.Stdout = rw.ResponseWriter
	cmd.Stderr = logErrorWriter{rw.req}
	rw.cmd = cmd

	rw.stdin, rw.err = cmd.StdinPipe()
	if rw.err == nil {
		rw.err = cmd.Start()
	}

	if rw.err != nil && !rw.wroteHeader {
		http.Error(rw.ResponseWriter, http.StatusText(http.StatusBadGateway),
			http.StatusBadGateway)
	} else if rw.err != nil {
		// We can't send an error response so bail here. For HTTP/2
		// this will send a RST_STREAM frame.
		panic(http.ErrAbortHandler)
	}
}

func (rw *responseWriter) closeVCDIFF() {
	if rw.cmd == nil {
		return
	}

	closeErr := rw.stdin.Close()
	if rw.err == nil {
		rw.err = closeErr
	}

	waitErr := rw.cmd.Wait()
	if rw.err == nil {
		rw.err = waitErr
	}
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type logErrorWriter struct{ r *http.Request }

func (w logErrorWriter) Write(p []byte) (int, error) {
	httputils.RequestLogf(w.r, "%s", p)
	return len(p), nil
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
