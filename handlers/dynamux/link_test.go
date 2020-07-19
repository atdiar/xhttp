package dynamux

import (
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"

	"testing"

	"context"

	"github.com/atdiar/localmemstore"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
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
	sess := session.New("somesess", "secret", session.SetStore(localmemstore.New()))
	mux.USE(sess)

	TestForwarder := func(l Link) xhttp.Handler {
		forwarder := httptest.NewServer(l.Proxy)
		return xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			test1 = test1 + test2
			w.Write([]byte(strconv.Itoa(test1)))
			if l.RedirectTo != nil {
				res, err := l.Client.Get(forwarder.URL)
				if err != nil {
					http.Error(w, "Could not fetch resource", http.StatusInternalServerError)
				}
				_, err = ioutil.ReadAll(res.Body)
				if err != nil {
					http.Error(w, "Could not read fetched response body", http.StatusInternalServerError)
				}
				w.Write([]byte(FWD))
			}
		})
	}

	dynamux, err := NewMultiplexer(strconv.Itoa(rand.Int()), &sess, TestForwarder)
	if err != nil {
		t.Fatal(err)
	}

	mux.GET("/atom/ray/", dynamux)
	mux.GET("/test/trueLink", xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		t.Log(mux.ServeMux.Handler(r))
		w.Write([]byte(FWD))
	}))

	return mux, dynamux
}

func TestLinkHandler(t *testing.T) {
	// Handler instantiation
	mux, dynamux := CreateMuxes(t)

	// the dynamux should handle link prefixes
	lnk := NewLink("linkid89645537y6", `/atom/ray/56/palmer/46`, nil, 0)
	err := dynamux.AddLink(lnk)
	if err != nil {
		t.Error(err)
	}

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

func TestLinkServerWithRedirect(t *testing.T) {
	// Handler instantiation
	mux, dynamux := CreateMuxes(t)

	u, err := url.Parse("http://www.example.com/test/trueLink")
	if err != nil {
		t.Error(err)
	}
	lnk := NewLink("linkid89695537y6", `/atom/ray/56/palmer/46`, u, 0)
	err = dynamux.AddLink(lnk)
	if err != nil {
		t.Error(err)
	}

	mux.GET("/", dynamux)

	// Request definition
	req, err := http.NewRequest("GET", "http://example.com/atom/ray/56/palmer/46", nil)
	if err != nil {
		t.Error(err)
	}

	// Response recording
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if body := w.Body.String(); body != (strconv.Itoa(test1) + FWD) {
		t.Errorf("Expected %v but got %v", FWD, body)
	}

	if test1 != test3+test2 {
		t.Errorf("Expected %v but got %v", test3+test2, test1)
	}
}

func TestLinkServerNoRedirect(t *testing.T) {
	// Handler instantiation
	mux, dynamux := CreateMuxes(t)

	lnk := NewLink("linkid89645537y6", `/atom/ray/56/palmer/46`, nil, 0)
	err := dynamux.AddLink(lnk)
	if err != nil {
		t.Error(err)
	}

	// Request definition
	req, err := http.NewRequest("GET", "http://example.com/atom/ray/56/palmer/46", nil)
	if err != nil {
		t.Error(err)
	}

	// Response recording
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if body := w.Body.String(); body != strconv.Itoa(test3+test2+test2) {
		t.Errorf("Expected %v but got %v", strconv.Itoa(test3+test2+test2), body)
	}

	if test1 != t0+test2+test2+test2 {
		t.Errorf("Expected %v but got %v", t0+test2+test2+test2, test1)
	}
}
