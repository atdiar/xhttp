package dynamux

import (
	"net/http"
	"net/http/httptest"
	"net/url"

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

var (
	test1 = 1
	test2 = 2
	test3 = test1 + test2
)

func CreateMuxes(t *testing.T) (xhttp.ServeMux, *Multiplexer) {
	mux := xhttp.NewServeMux()
	sess := session.New("somesess", "secret", session.SetStore(localmemstore.New()))
	sess.Cookie.SetID(ID)
	mux.USE(sess)

	linkhandler := func(l Link) xhttp.Handler {
		return xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
			test1 = test1 + test2
			t.Log(test1, test2, test3)
			if l.RedirectTo != nil {
				http.RedirectHandler(l.RedirectTo.String(), http.StatusPermanentRedirect)
			}
		})
	}
	dynamux, err := NewMultiplexer("sessionKey", sess, linkhandler)
	if err != nil {
		t.Fatal(err)
	}
	mux.GET("/", xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		t.Log("the catchall route")
	}))
	mux.GET("/atom/ray/", dynamux)
	mux.GET("/test/trueLink", xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(FWD))
	}))

	return mux, dynamux
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

	if test1 != test3 {
		t.Errorf("Expected %v but got %v", test3, test1)
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
	if body := w.Body.String(); body != PATH {
		t.Errorf("Expected %v but got %v", PATH, body)
	}

	if test1 != test3 {
		t.Errorf("Expected %v but got %v", test3, test1)
	}
}
