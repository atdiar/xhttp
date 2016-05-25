package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/localmemstore"
	"github.com/atdiar/xhttp"
)

const (
	Payload = "ok\n"
)

func Multiplexer() xhttp.ServeMux {

	r := xhttp.NewServeMux()

	sessionhandler := New("thiusedfrtgju8975bj", localmemstore.New())
	r.USE(sessionhandler)

	r.GET("/", xhttp.HandlerFunc(func(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
		res.Write([]byte(Payload))
	}))
	return r
}

func TestSession(t *testing.T) {
	r := Multiplexer()

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
