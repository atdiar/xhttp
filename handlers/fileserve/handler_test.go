package fileserve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atdiar/xhttp"

	"io/ioutil"
	"os"
)

func TestFileserving(t *testing.T) {

	// Create temporary file
	content := []byte("temporary file's content")
	tmpfile, err := ioutil.TempFile("", "example")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Create fileserving request handler
	fsh := New(tmpfile.Name())

	// Implement fileserving logic on get requests
	mux := xhttp.NewServeMux()
	mux.GET("/", fsh)

	// Test on mock Server
	req, err := http.NewRequest("GET", "http://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Response recording
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Testing the response returned by the server.
	if response := w.Body.String(); response != string(content) {
		t.Fatalf("Expected: %v but got: %v \n", content, response)
	}

}
