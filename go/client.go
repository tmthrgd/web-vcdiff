package vcdiff

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	openvcdiff "go.tmthrgd.dev/web-vcdiff/go/internal/open-vcdiff"
)

func redirectIsError(req *http.Request, via []*http.Request) error {
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
	dictCache  sync.Map // map[string]func() ([]byte, error)
}

func RoundTripper(t http.RoundTripper) http.RoundTripper {
	if t == nil {
		t = http.DefaultTransport
	}

	return &roundTripper{
		t: t,

		dictClient: &http.Client{
			Transport:     t,
			CheckRedirect: redirectIsError,
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

	identifier := resp.Header.Get("Content-Diff-Dictionary")
	if identifier == "" || strings.HasPrefix(identifier, ";") {
		return nil, errors.New("vcdiff: missing required Content-Diff-Dictionary header")
	}

	var integrity string
	if idx := strings.Index(identifier, ";"); idx >= 0 {
		identifier, integrity = identifier[:idx], identifier[idx+1:]
	}

	dictURL := *resp.Request.URL
	dictURL.Path = DictionaryHandlerPath + identifier
	dictURL.RawQuery = ""

	dict, err := rt.getDict(dictURL.String())
	if err != nil {
		return nil, fmt.Errorf("vcdiff: dictionary fetch failed: %v", err)
	}

	if integrity != "" && !sriValid(integrity, dict) {
		return nil, errors.New("vcdiff: dictionary response failed subresource integrity check")
	}

	resp2 := *resp
	resp2.Body = &bodyDecoder{nil, resp.Body, dict}

	resp2.Header = cloneHeaders(resp.Header)
	resp2.Header.Del("Content-Diff-Encoding")
	resp2.Header.Del("Content-Diff-Dictionary")

	return &resp2, nil
}

func (rt *roundTripper) getDict(url string) ([]byte, error) {
	if fn, ok := rt.dictCache.Load(url); ok {
		return fn.(func() ([]byte, error))()
	}

	var (
		c = rt.dictClient

		once sync.Once
		body []byte
		err  error
	)
	fn, _ := rt.dictCache.LoadOrStore(url, func() ([]byte, error) {
		once.Do(func() {
			var resp *http.Response
			if resp, err = c.Get(url); err != nil {
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				err = errors.New(resp.Status)
				return
			}

			body, err = ioutil.ReadAll(resp.Body)
		})
		return body, err
	})
	return fn.(func() ([]byte, error))()
}

type bodyDecoder struct {
	pr *io.PipeReader

	body io.ReadCloser
	dict []byte
}

func (bd *bodyDecoder) Read(p []byte) (int, error) {
	if bd.pr == nil {
		if bd.body == nil {
			return 0, errors.New("http: read on closed response body")
		}

		pr, pw := io.Pipe()
		go decodeBody(pw, bd.body, bd.dict)
		*bd = bodyDecoder{pr: pr}
	}

	return bd.pr.Read(p)
}

func (bd *bodyDecoder) Close() error {
	var err error
	switch {
	case bd.pr != nil:
		err = bd.pr.Close()
	case bd.body != nil:
		err = bd.body.Close()
	}

	*bd = bodyDecoder{}
	return err
}

func decodeBody(pw *io.PipeWriter, body io.ReadCloser, dict []byte) {
	d := openvcdiff.NewDecoder(pw, dict)
	defer d.Close()

	_, err := io.Copy(d, body)

	if closeErr := body.Close(); err == nil {
		err = closeErr
	}

	if closeErr := d.Close(); err == nil {
		err = closeErr
	}

	pw.CloseWithError(err)
}

type sriAlg struct {
	newFn func() hash.Hash
	bits  int
}

var sriAlgs = map[string]sriAlg{
	"sha256": {sha256.New, 256},
	"sha384": {sha512.New384, 384},
	"sha512": {sha512.New, 512},
}

func sriValid(integrity string, b []byte) bool {
	var (
		bestAlg sriAlg
		digests []string
	)
	for _, integrity := range strings.Fields(integrity) {
		parts := strings.SplitN(integrity, "-", 2)
		alg, ok := sriAlgs[parts[0]]
		if len(parts) != 2 || !ok {
			continue
		}

		switch {
		case bestAlg.bits < alg.bits:
			bestAlg, digests = alg, parts[1:]
		case bestAlg.bits == alg.bits:
			digests = append(digests, parts[1])
		}
	}

	if bestAlg.newFn == nil {
		return true
	}

	h := bestAlg.newFn()
	h.Write(b)
	sum := h.Sum(nil)

	for _, digest := range digests {
		if idx := strings.Index(digest, "?"); idx >= 0 {
			digest = digest[:idx]
		}

		d, err := base64.StdEncoding.DecodeString(digest)
		if err == nil && bytes.Equal(sum, d) {
			return true
		}
	}

	return false
}
