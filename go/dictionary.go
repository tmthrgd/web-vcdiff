package vcdiff

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	handlers "github.com/tmthrgd/httphandlers"
	"github.com/tmthrgd/httputils"
)

type DictionaryID [16]byte

func newDictionaryID(data []byte) DictionaryID {
	digest := sha512.Sum512_256(data)
	var id DictionaryID
	copy(id[:], digest[:])
	return id
}

func (id DictionaryID) encode() string {
	return base64.RawURLEncoding.EncodeToString(id[:])
}

func (id *DictionaryID) decode(s string) bool {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) != len(id) {
		return false
	}

	copy(id[:], b)
	return true
}

type Dictionary struct {
	ID   DictionaryID
	Data []byte

	gzipOnce sync.Once
	gzipData []byte
}

func NewDictionary(data []byte) *Dictionary {
	return &Dictionary{
		ID:   newDictionaryID(data),
		Data: data,
	}
}

func ReadDictionary(path string) (*Dictionary, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return NewDictionary(data), nil
}

func (d *Dictionary) gzip() {
	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)

	_, err := w.Write(d.Data)
	if err == nil && w.Close() == nil &&
		buf.Len() < len(d.Data) {
		d.gzipData = buf.Bytes()
	}
}

type Dictionaries interface {
	Select(*http.Request) (*Dictionary, error)
	Find(context.Context, DictionaryID) (*Dictionary, error)
}

func FixedDictionary(d *Dictionary) Dictionaries { return fixedDictionary{d} }

type fixedDictionary struct{ d *Dictionary }

func (fd fixedDictionary) Select(*http.Request) (*Dictionary, error) { return fd.d, nil }

func (fd fixedDictionary) Find(ctx context.Context, id DictionaryID) (*Dictionary, error) {
	if fd.d.ID == id {
		return fd.d, nil
	}

	return nil, nil
}

const DictionaryHandlerPath = "/.well-known/web-vcdiff/d/"

func DictionaryHandler(d Dictionaries) http.Handler {
	return handlers.NeverModified(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id DictionaryID
		if !strings.HasPrefix(r.URL.Path, DictionaryHandlerPath) ||
			!id.decode(r.URL.Path[len(DictionaryHandlerPath):]) {
			http.NotFound(w, r)
			return
		} else if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed),
				http.StatusMethodNotAllowed)
			return
		}

		dict, err := d.Find(r.Context(), id)
		if err != nil {
			httputils.RequestLogf(r, "dictionary callback failed: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			return
		} else if dict == nil {
			http.NotFound(w, r)
			return
		}

		if dict.ID != id || dict.ID != newDictionaryID(dict.Data) {
			panic("vcdiff: invalid dictionary identifier")
		}

		hdr := w.Header()
		hdr.Set("Content-Type", "application/octet-stream")
		hdr.Set("Cache-Control", "public, max-age=31536000, immutable")
		hdr.Set("Last-Modified", "Mon, 01 Jan 2001 01:00:00 GMT")

		data := dict.Data
		if httputils.Negotiate(r.Header, "Accept-Encoding", "gzip") == "gzip" {
			dict.gzipOnce.Do(dict.gzip)

			if dict.gzipData != nil {
				hdr.Set("Content-Encoding", "gzip")
				data = dict.gzipData
			}
		}

		hdr.Set("Content-Length", strconv.Itoa(len(data)))
		w.Write(data)
	}))
}
