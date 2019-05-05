package vcdiff

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	openvcdiff "github.com/tmthrgd/web-vcdiff/go/internal/open-vcdiff"
)

func redirectAsError(req *http.Request, via []*http.Request) error {
	return errors.New("vcdiff: redirects are prohibitted for dictionary responses")
}

func cloneHeaders(h http.Header) http.Header {
	h2 := make(http.Header, len(h))
	for k, v := range h {
		h2[k] = v
	}

	return h2
}

type roundTripper struct {
	t http.RoundTripper

	dictClient *http.Client
	cache      sync.Map // map[string]func() ([]byte, error)
}

func RoundTripper(t http.RoundTripper) http.RoundTripper {
	if t == nil {
		t = http.DefaultTransport
	}

	return &roundTripper{
		t: t,

		dictClient: &http.Client{
			Transport:     t,
			CheckRedirect: redirectAsError,
		},
	}
}

func Client(t http.RoundTripper) *http.Client {
	return &http.Client{
		Transport: RoundTripper(t),
	}
}

func (rt *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := *req

	req2.Header = cloneHeaders(req.Header)
	req2.Header.Set("Accept-Diff-Encoding", "vcdiff")

	resp, err := rt.t.RoundTrip(&req2)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(resp.Header.Get("Content-Diff-Encoding"), "vcdiff") {
		return resp, nil
	}

	dictHdr := resp.Header.Get("Content-Diff-Dictionary")
	if dictHdr == "" {
		return nil, errors.New("vcdiff: missing required Content-Diff-Dictionary header")
	}

	dictURL := *resp.Request.URL
	dictURL.Path = DictionaryHandlerPath + dictHdr
	dictURL.RawQuery = ""

	dict, err := rt.getDict(dictURL.String())
	if err != nil {
		return nil, fmt.Errorf("vcdiff: dictionary fetch failed: %v", err)
	}

	pr, pw := io.Pipe()
	d, err := openvcdiff.NewDecoder(pw, dict)
	if err != nil {
		return nil, err
	}

	go copyBody(d, resp.Body, pw)

	resp2 := *resp
	resp2.Body = struct{ io.ReadCloser }{pr}

	resp2.Header = cloneHeaders(resp.Header)
	resp2.Header.Del("Content-Diff-Encoding")
	resp2.Header.Del("Content-Diff-Dictionary")

	return &resp2, nil
}

func (rt *roundTripper) getDict(url string) ([]byte, error) {
	c := rt.dictClient
	fn, _ := rt.cache.LoadOrStore(url, func() ([]byte, error) {
		resp, err := c.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, errors.New(resp.Status)
		}

		return ioutil.ReadAll(resp.Body)
	})
	return fn.(func() ([]byte, error))()
}

func copyBody(d *openvcdiff.Decoder, body io.ReadCloser, pw *io.PipeWriter) {
	_, err := io.Copy(d, body)

	if closeErr := body.Close(); err == nil {
		err = closeErr
	}

	if closeErr := d.Close(); err == nil {
		err = closeErr
	}

	pw.CloseWithError(err)
}
