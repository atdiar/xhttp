package panic

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atdiar/xhttp"
)

var Payload = "Panicked"

func TestHandler(t *testing.T) {
	mux := xhttp.NewServeMux()
	mux.USE(NewHandler(func(msg interface{}, w http.ResponseWriter, r *http.Request) {
		// do something simple
		_, _ = fmt.Fprint(w, msg)
	}))

	mux.GET("/", xhttp.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// Let's just panic here and see if it is going to get handled as we expect.
		panic(Payload)
	}))

	// Request definition
	req, err := http.NewRequest("GET", "http://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Response recording
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Testing the response returned by the server.
	if response := w.Body.String(); response != Payload {
		t.Fatalf("Expected: %v but got: %v \n", Payload, response)
	}
}
