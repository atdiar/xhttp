package session

import (
	"bytes"
	"context"
	//"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atdiar/xhttp"
)

const (
	fakeSessionID  = "987653536787838"
	fakeSessionID2 = "sessionid123456"
	GSID           = "GSID"
)

func Multiplexer(t *testing.T) (xhttp.ServeMux, Handler) {

	r := xhttp.NewServeMux()

	s := New(GSID, "secret")
	s.Cookie.HttpCookie.MaxAge = 8640000
	r.USE(s)

	r.GET("/", xhttp.HandlerFunc(func(ctx context.Context, res http.ResponseWriter, req *http.Request) {
		// We do nothing here but a session cookie should have at least been set.
	}))

	r.POST("/", xhttp.HandlerFunc(func(ctx context.Context, res http.ResponseWriter, req *http.Request) {
		_, ok := ctx.Value(s.ContextKey).(http.Cookie)
		if !ok {
			t.Error("The session was not loaded")
		}

		id, ok := s.Cookie.ID()
		if !ok {
			//http.Error(res, ErrNoID.Error(), 501)
			t.Errorf("Expected an id of %v but it is %v as we're getting %v \n Cookie maxage is %v ", fakeSessionID, ok, id, s.Cookie.HttpCookie.MaxAge)
		}

		s.Put("test", []byte("test"), 86400*time.Minute)
		s.SetSessionCookie(ctx, res, req)

		res.Write([]byte(id))
	}))

	return r, s
}

func TestSession(t *testing.T) {
	r, sess := Multiplexer(t)

	// Initial request
	req1, err := http.NewRequest("GET", "http://example.com/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Response recording
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req1)

	// There was no session cookie sent with the request since it is the
	// initial one. However we expect a response that includes one, since
	// a session should have been generated.
	var s *http.Cookie
	wcookie := w.Result().Cookies()

	if len(wcookie) == 0 {
		t.Fatal("No cookie has been set, including session coookie.")
	}
	for _, c := range wcookie {
		if c.Name == GSID {
			s = c
			break
		}
	}

	if s == nil {
		t.Fatalf("The session cookie does not seem to have been set. Got %v", s)
	}
	if s.Name != "GSID" || s.Path != "/" || s.HttpOnly != true || s.Secure != true {
		t.Errorf("The session cookie does not seem to have been set correctly. Got %v", s)
	}
	if s.MaxAge != 8640000 {
		t.Errorf("Session Cookie was uncorrectly set. Got %v and wanted %v", s.MaxAge, 8640000)
	}

	// Second request
	// We send with the request the session cookie that was previously sent with
	// the response.
	// We expect a 30X response testifying that all went well.
	req2, err := http.NewRequest("POST", "http://example.com/", nil)
	if err != nil {
		t.Error(err)
	}
	// let's add the request cookie, created out of the previous response to req1
	req2.AddCookie(s)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, req2)
	str := w.Body.String()

	// The session cookie should not be set in the response as the session hasn't changed.
	scookie := w.Result().Cookies()[0]
	if scookie.Name != GSID {
		t.Fatal("Some additional cookie with a name I don't know has been set?!")
	}

	err = sess.Cookie.Decode(*scookie)
	if err != nil {
		t.Error(err)
	}

	bstr, err := sess.Get("test")

	if err != nil {
		t.Fatal("unable to retrieve new item put in the session cookie under the key <<test>>", err)
	}

	if bytes.Compare(bstr, []byte("test")) != 0 {
		t.Fatalf("Was expecting %s but got %s", "test", string(bstr))
	}

	// Third request
	// We change the cookie value. It should trigger session renewal.
	req3, err := http.NewRequest("POST", "http://example.com/bar", nil)
	if err != nil {
		t.Fatal(err)
	}
	req3.AddCookie(&http.Cookie{
		Name:   "GSID",
		Path:   "/",
		Secure: true,
		MaxAge: 0,
		Value:  "kdL7gHOcaXV22jM0ltklYPV2EWeEImgH/nwqYTwtuGo=:eyJzb21lIHZhbHVlIjp7IlYiOiJIZWxsbywgV29ybGQiLCJUIjoiMjAwOS0xMS0xMFQyMzowMDowMFoifX0=",
	})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req3)

	var ns *http.Cookie
	wcookie = w.Result().Cookies()

	if len(wcookie) == 0 {
		t.Fatal("No cookie has been set, including session coookie.")
	}

	for _, c := range wcookie {
		if c.Name == GSID {
			ns = c
			break
		}
	}
	if ns == nil {
		t.Error("The session cookie does not seem to have been set.")
	}
	if ns.Name != "GSID" || ns.Path != "/" || ns.HttpOnly != true || ns.Secure != true {
		t.Errorf("The session cookie does not seem to have been set correctly.`\n` Expected: `\n` %v but got: `\n` %v", wcookie, ns)
	}
	if ns.MaxAge != 8640000 {
		t.Error("Session Cookie was uncorrectly set.")
	}
	if ns.Value == s.Value {
		t.Errorf("Expected a new cookie value: \n %s \n but got: \n %s \n", ns.Value, s.Value)
	}
	if body := w.Body.String(); body == str {
		t.Errorf("Expected different values but got the same id: %v vs %v", body, str)
	}

}

func TestSessionInterface(t *testing.T) {
	s := New(GSID, "secret")
	_ = Interface(&s)
}
