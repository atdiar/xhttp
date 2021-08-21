package dynamux

import (
	"net/http"
	"net/http/httptest"
	//"net/http/httputil"

	"net/url"
	"strconv"

	"testing"

	"github.com/atdiar/xhttp"
)

const (
	ID   = "ID80734658065432"
	FWD  = "FORWARDED"
	PATH = "/atom/ray/"
)

// 124891=no po
var (
	t0    = 189645
	test1 = 189645
	test2 = 2
	test3 = test1 + test2
)

func CreateMuxes(t *testing.T) (xhttp.ServeMux, *Multiplexer) {
	mux := xhttp.NewServeMux()
	dynamux := NewMultiplexer()

	mux.GET("/atom/ray/", dynamux)
	mux.GET("/test/trueLink", xhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(FWD))
	}))

	return mux, dynamux
}

func TestLinkHandler(t *testing.T) {
	// Handler instantiation
	mux, dynamux := CreateMuxes(t)

	u, err := url.Parse("http://www.example.com/test/trueLink")
	if err != nil {
		t.Error(err)
	}

	// the dynamux should handle link prefixes
	lnk := NewLink("linkid89645537y6", `/atom/ray/56/palmer/46`, u, 0, false).WithHandler(xhttp.HandlerFunc(func( w http.ResponseWriter, r *http.Request) {
		test1 = test1 + test2
		w.Write([]byte(strconv.Itoa(test1)))
	}))

	dynamux.AddLink(lnk)

	// Request definition
	req, err := http.NewRequest("GET", "http://example.com/atom/ray/", nil)
	if err != nil {
		t.Error(err)
	}

	// Response recording
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if body := w.Body.String(); body != strconv.Itoa(test3) {
		t.Errorf("Expected %v but got %v", test3, body)
	}

	if test1 != test3 {
		t.Errorf("Expected %v but got %v", test3, test1)
	}
}

// Test with url proxying
//
//
func TestLinkServerWithRedirect(t *testing.T) {
	// Handler instantiation
	mux, dynamux := CreateMuxes(t)

	// target server
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//t.Fatal("error")
		w.Write([]byte(FWD))
	}))
	defer s.Close()
	urlserv, err := url.Parse(s.URL)
	if err != nil {
		t.Error(err)
	}

	// link creation
	lnk := NewLink("linkid89645537y6", `/atom/ray/56/palmer/46`, urlserv, 0, true).WithHandler(xhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		test1 = test1 + test2
	}))

	dynamux.AddLink(lnk)

	// Request definition
	req, err := http.NewRequest("GET", "http://example.com/atom/ray/56/palmer/46", nil)
	if err != nil {
		t.Error(err)
	}

	// Response recording
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if body := w.Body.String(); body != FWD {
		t.Errorf("Expected %v but got %v", FWD, body)
	}

	if test1 != test3+test2 {
		t.Errorf("Expected %v but got %v", test3+test2, test1)
	}
}
