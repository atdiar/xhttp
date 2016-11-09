package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atdiar/xhttp"
)

const (
	URL      = "http://example.com/Test/whatever"
	ShortURL = "http://noexample.com/whatever"
)

func TestCORS(t *testing.T) {

	mux := xhttp.NewServeMux()

	cs := NewHandler().EnablePreflight(&mux, "/")
	cs.AllowedOrigins.Add(URL)
	cs.AllowedMethods.Add("GET", "POST")
	cs.AllowedHeaders.Add("*")
	cs.AllowCredentials = true
	cs.ExposeHeaders.Add("X-Test")

	mux.GET("/", cs)

	req, err := http.NewRequest("OPTIONS", "http://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", URL)
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "")

	req2, err := http.NewRequest("GET", "http://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Add("Origin", URL)
	req2.Header.Set("X-Not-Simple", "true")

	req3, err := http.NewRequest("OPTIONS", "http://example.com/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req3.Header.Add("Origin", ShortURL)
	req3.Header.Set("X-Not-Simple", "true")

	// First response recording:
	// we expect a result equivalent to the one obtained in response to a preflight
	// request.
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	method := w.Header().Get("Access-Control-Allow-Methods")
	if method != "GET" {
		t.Errorf("Expected %s but got %s", "GET", method)
		t.Log(w.Header())
	}
	headers, ok := w.HeaderMap["Access-Control-Allow-Headers"]
	if !ok {
		t.Error("Access-Control-Allow-Headers is missing from the response headers.\n")
	}
	if !cs.AllowedHeaders.Contains("*", false) {
		for _, header := range headers {
			if !cs.AllowedHeaders.Contains(header, false) {
				t.Errorf("Unexpectedly absent header %v\n", header)
			}
		}
	}
	cred := w.Header().Get("Access-Control-Allow-Credentials")
	if cred != "true" {
		t.Log(cred, w.HeaderMap)
		t.Errorf("Credentials header value is %s. Shows a discrepancy with the handler's credential flag which has been set to true.\n", cred)
	}

	// Second response recording
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req2)
	ori := w.Header().Get("Access-Control-Allow-Origin")
	if ori != URL {
		t.Errorf("Expected %s but got %s", URL, ori)
	}
	_, ok = w.HeaderMap["Access-Control-Expose-Headers"]
	if !ok {
		t.Error(" `Access-Control-Expose-Headers` header is missing.\n")
	}

	_, ok = w.HeaderMap["Access-Control-Allow-Credentials"]
	if !ok {
		t.Error(" `Access-Control-Allow-Credentials` header is missing.\n")
	}

	// Third response recording
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req3)
	_, ok = w.HeaderMap["Access-Control-Allow-Methods"]
	_, ok2 := w.HeaderMap["Access-Control-Allow-Headers"]
	if ok || ok2 {
		t.Errorf("Did not expect the header to be set since origin is not authorized.\n")
	}
}
