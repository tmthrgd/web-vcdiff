package vcdiff

import (
	"crypto/sha512"
	"encoding/base64"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"

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
}

func NewDictionary(data []byte) *Dictionary {
	return &Dictionary{newDictionaryID(data), data}
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

func (d *Dictionary) checkValid() {
	if d.ID != newDictionaryID(d.Data) {
		panic("vcdiff: invalid dictionary identifier")
	}
}

type Dictionaries interface {
	Select(*http.Request) (*Dictionary, error)
	Find(DictionaryID) (*Dictionary, error)
}

func FixedDictionary(d *Dictionary) Dictionaries { return (*fixedDictionary)(d) }

type fixedDictionary Dictionary

func (d *fixedDictionary) Select(*http.Request) (*Dictionary, error) { return (*Dictionary)(d), nil }

func (d *fixedDictionary) Find(id DictionaryID) (*Dictionary, error) {
	if d.ID == id {
		return (*Dictionary)(d), nil
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

		dict, err := d.Find(id)
		if err != nil {
			httputils.RequestLogf(r, "dictionary callback failed: %v", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError),
				http.StatusInternalServerError)
			return
		} else if dict == nil {
			http.NotFound(w, r)
			return
		}

		dict.checkValid()

		hdr := w.Header()
		hdr.Set("Content-Type", "application/octet-stream")
		hdr.Set("Content-Length", strconv.Itoa(len(dict.Data)))
		hdr.Set("Cache-Control", "public, max-age=31536000, immutable")

		w.Write(dict.Data)
	}))
}
