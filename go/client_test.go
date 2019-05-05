package vcdiff

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	handlers "github.com/tmthrgd/httphandlers"
)

const testData = "The quick brown fox jumps over the lazy dog."

func TestClient(t *testing.T) {
	ds := FixedDictionary(NewDictionary([]byte(testData)))

	mux := http.NewServeMux()
	mux.Handle(DictionaryHandlerPath,
		DictionaryHandler(ds))

	mux.Handle("/test.txt", Handler(ds,
		handlers.ServeString("test.txt", time.Time{}, testData)))

	s := httptest.NewServer(mux)
	defer s.Close()

	c := Client(s.Client().Transport)

	resp, err := c.Get(s.URL + "/test.txt")
	require.NoError(t, err)
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, string(b), testData)

	assert.NoError(t, resp.Body.Close())
}
