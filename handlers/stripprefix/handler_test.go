package stripprefix

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atdiar/xhttp"
)

const (
	URL      = "http://example.com/Test/whatever"
	ShortURL = "http://example.com/whatever"
)

func TestStripprefix(t *testing.T) {
	// Let's create the prefix stripping request handler
	// note that "/Test" without the slash at the end works too.
	sp := NewHandler("/Test/")

	// Implement fileserving logic on get requests
	mux := xhttp.NewServeMux()
	mux.GET("/", sp)

	// Test on mock Server
	req, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	if req.URL.String() != URL {
		t.Errorf("Expected %s but got %s \n", URL, req.URL.String())
	}

	// Response recording
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Testing the response returned by the server.
	if req.URL.String() != ShortURL {
		t.Fatalf("Expected %s but got %s \n", URL, req.URL.String())
	}
}
