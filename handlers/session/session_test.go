package session

import (
	"github.com/atdiar/context"
	"github.com/atdiar/xhttp"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	Payload = "ok\n"
)

func Router() xhttp.Router {

	r := xhttp.NewRouter()

	sessionhandler := New("thiusedfrtgju8975bj", DefaultStore)
	r.Use(sessionhandler)

	r.GET("/", xhttp.HandlerFunc(func(res http.ResponseWriter, req *http.Request, ctx context.Object) (http.ResponseWriter, bool) {
		res.Write([]byte(Payload))
		return res, false
	}))
	return r
}

func TestSession(t *testing.T) {
	r := Router()

	// Request definition
	req, err := http.NewRequest("GET", "http://example.com/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Response recording
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// The payload shall not be sent as there was no session cookie
	// The expected response should be "Failed to load session." with a 500 http.status
	t.Log(w)

}
