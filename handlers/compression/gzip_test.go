package compression

import (
	"github.com/atdiar/xhttp"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

const (
	Payload = "eightsix\n"
)

func Router() xhttp.Router {
	// Let's define a router that compresses the response
	// except for POST requests.
	// The response is just the aforementionned payload
	// concatenated 1024 times.
	r := xhttp.NewRouter()

	compressor := New().Skip("POST")

	r.Use(compressor)

	r.GET("/", xhttp.ConvertFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Length", strconv.Itoa(9*1024))
		for i := 0; i < 1024; i++ {
			res.Write([]byte(Payload))
		}
	}))

	r.POST("/", xhttp.ConvertFunc(func(res http.ResponseWriter, req *http.Request) {
		res.Header().Set("Content-Length", strconv.Itoa(9*1024))
		for i := 0; i < 1024; i++ {
			res.Write([]byte(Payload))
		}
	}))

	return r
}

func TestCompressHandler(t *testing.T) {
	// Handler instantiation
	r := Router()

	// Request definition
	req, err := http.NewRequest("GET", "http://example.com/foo", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Accept-Encoding", "gzip")

	// Response recording
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Testing that the response is correctly sent.
	if w.HeaderMap.Get("Content-Encoding") != "gzip" {
		t.Errorf("wrong content encoding, got %q want %q", w.HeaderMap.Get("Content-Encoding"), "gzip")
	}
	if w.HeaderMap.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("wrong content type, got %s want %s", w.HeaderMap.Get("Content-Type"), "text/plain; charset=utf-8")
	}
	if w.Body.Len() != 60 {
		t.Errorf("wrong len, got %d want %d", w.Body.Len(), 60)
	}
	if l := w.HeaderMap.Get("Content-Length"); l != "" {
		t.Errorf("wrong content-length. got %q expected %q", l, "")
	}

	// Second request is a POST request.
	// As defined, the compressing handler ignores this verb.
	// The response to a POST request shall not be compressed.

	// Request definition
	req, err = http.NewRequest("POST", "http://example.com/", nil)
	req.Header.Add("Accept-Encoding", "gzip")

	// Response recording
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Testing that the response is correctly sent.
	if enc := w.HeaderMap.Get("Content-Encoding"); enc != "" {
		t.Errorf("wrong content encoding, got %q want %q", enc, "")
	}
	if ct := w.HeaderMap.Get("Content-Type"); ct != "" {
		t.Errorf("wrong content type, got %q want %q", ct, "")
	}
	if w.Body.Len() != 1024*9 {
		t.Errorf("wrong len, got %d want %d", w.Body.Len(), 1024*9)
	}
	if l := w.HeaderMap.Get("Content-Length"); l != "9216" {
		t.Errorf("wrong content-length. got %q expected %d", l, 1024*9)
	}
}
