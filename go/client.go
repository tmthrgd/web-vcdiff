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

	integrity := resp.Header.Get("Content-Diff-Dictionary-Integrity")
	if integrity != "" && !sriValid(integrity, dict) {
		return nil, errors.New("vcdiff: dictionary response failed subresource integrity check")
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
	resp2.Header.Del("Content-Diff-Dictionary-Integrity")

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

	for i, digest := range digests {
		if idx := strings.Index(digest, "?"); idx > 0 {
			digest = digest[:idx]
		}

		d, err := base64.StdEncoding.DecodeString(digest)
		if err != nil || h.Size() != len(d) {
			continue
		}

		if i > 0 {
			h.Reset()
		}

		h.Write(b)

		if bytes.Equal(h.Sum(nil), d) {
			return true
		}
	}

	return false
}
