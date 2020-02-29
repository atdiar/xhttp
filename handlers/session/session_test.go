package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atdiar/testcache"
	"github.com/atdiar/xhttp"
)

const (
	fakeSessionID  = "987653536787838"
	fakeSessionID2 = "sessionid123456"
)

func Multiplexer(t *testing.T) xhttp.ServeMux {

	r := xhttp.NewServeMux()

	s := New("GSID", "secret", SetStore(testcache.TestStore())) // ("thiusedfrtgju8975bj", testcache.TestStore())
	s.Cookie.Config.MaxAge = 86400
	r.USE(s)

	r.GET("/", xhttp.HandlerFunc(func(ctx context.Context, res http.ResponseWriter, req *http.Request) {
		// We do nothing here but a session cookie should have at least been set.
	}))

	r.POST("/", xhttp.HandlerFunc(func(ctx context.Context, res http.ResponseWriter, req *http.Request) {
		_, ok := ctx.Value(s.ContextKey).(http.Cookie)
		if !ok {
			//http.Error(res, ErrBadSession.Error(), 501)
			t.Error("The session was not correctly setup")
		}

		/*	_, err := s.Load(ctx, res, req)
			if err != nil {
				t.Error(err)
			}*/
		id, ok := s.Cookie.ID()
		if !ok {
			//http.Error(res, ErrNoID.Error(), 501)
			t.Errorf("Expected an id of %v but it is %v as we're getting %v \n Cookie maxage is %v ", fakeSessionID, ok, id, s.Cookie.Config.MaxAge)
		}
		res.Write([]byte(id))

	}))

	return r
}

func TestSession(t *testing.T) {
	r := Multiplexer(t)

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
	s := RetrieveCookie(w.Header(), "GSID")

	if s == nil {
		t.Error("The session cookie does not seem to have been set.")
	}
	if s.Name != "GSID" || s.Path != "/" || s.HttpOnly != true || s.Secure != true {
		t.Error("The session cookie does not seem to have been set correctly.")
	}
	if s.MaxAge != 86400 {
		t.Errorf("Session Cookie was uncorrectly set. Got %v and wanted %v", s.MaxAge, 86400)
	}

	// Second request
	// We send with the request the session cookie that was previously sent with
	// the response.
	// We expect a 30X response testifying that all went well.
	req2, err := http.NewRequest("POST", "http://example.com/", nil)
	if err != nil {
		t.Error(err)
	}

	r.ServeHTTP(w, req2)
	str := w.Body.String()

	// The session cookie should not change.
	s = RetrieveCookie(w.Header(), "GSID")
	if s == nil {
		t.Error("The session cookie does not seem to have been set.")
	}
	if s.Name != "GSID" || s.Path != "/" || s.HttpOnly != true || s.Secure != true {
		t.Error("The session cookie does not seem to have been set correctly.")
	}
	if s.MaxAge != 86400 {
		t.Error("Session Cookie was uncorrectly set.")
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
		Value:  "hjfhfhdfh:gjfjghfgjh",
	})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req3)

	ns := RetrieveCookie(w.Header(), "GSID")
	if ns == nil {
		t.Error("The session cookie does not seem to have been set.")
	}
	if ns.Name != "GSID" || ns.Path != "/" || ns.HttpOnly != true || ns.Secure != true {
		t.Error("The session cookie does not seem to have been set correctly.")
	}
	if ns.MaxAge != 86400 {
		t.Error("Session Cookie was uncorrectly set.")
	}
	if ns.Value == s.Value {
		t.Errorf("Expected a new cookie value: \n %s \n but got: \n %s \n", ns.Value, s.Value)
	}
	if body := w.Body.String(); body == str {
		t.Errorf("Expected different values but got the same id: %v vs %v", body, str)
	}
}

// #############################################################################
// The below is extracted from Go's standard library and is used simply to
// retrieve a cookie that has been set in a http.Header.
//
// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// #############################################################################
func RetrieveCookie(h http.Header, Name string) *http.Cookie {
	cookie := &http.Cookie{}
	for _, line := range h["Set-Cookie"] {
		parts := strings.Split(strings.TrimSpace(line), ";")
		if len(parts) == 1 && parts[0] == "" {
			continue
		}
		parts[0] = strings.TrimSpace(parts[0])
		j := strings.Index(parts[0], "=")
		if j < 0 {
			continue
		}
		name, value := parts[0][:j], parts[0][j+1:]
		if !isCookieNameValid(name) && name != Name {
			continue
		}
		value, success := parseCookieValue(value, true)
		if !success {
			continue
		}
		c := &http.Cookie{
			Name:  name,
			Value: value,
			Raw:   line,
		}
		for i := 1; i < len(parts); i++ {
			parts[i] = strings.TrimSpace(parts[i])
			if len(parts[i]) == 0 {
				continue
			}

			attr, val := parts[i], ""
			if j := strings.Index(attr, "="); j >= 0 {
				attr, val = attr[:j], attr[j+1:]
			}
			lowerAttr := strings.ToLower(attr)
			val, success = parseCookieValue(val, false)
			if !success {
				c.Unparsed = append(c.Unparsed, parts[i])
				continue
			}
			switch lowerAttr {
			case "secure":
				c.Secure = true
				continue
			case "httponly":
				c.HttpOnly = true
				continue
			case "domain":
				c.Domain = val
				continue
			case "max-age":
				secs, err := strconv.Atoi(val)
				if err != nil || secs != 0 && val[0] == '0' {
					break
				}
				if secs <= 0 {
					c.MaxAge = -1
				} else {
					c.MaxAge = secs
				}
				continue
			case "expires":
				c.RawExpires = val
				exptime, err := time.Parse(time.RFC1123, val)
				if err != nil {
					exptime, err = time.Parse("Mon, 02-Jan-2006 15:04:05 MST", val)
					if err != nil {
						c.Expires = time.Time{}
						break
					}
				}
				c.Expires = exptime.UTC()
				continue
			case "path":
				c.Path = val
				continue
			}
			c.Unparsed = append(c.Unparsed, parts[i])
		}
		cookie = c
	}
	return cookie
}

func parseCookieValue(raw string, allowDoubleQuote bool) (string, bool) {
	// Strip the quotes, if present.
	if allowDoubleQuote && len(raw) > 1 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		raw = raw[1 : len(raw)-1]
	}
	for i := 0; i < len(raw); i++ {
		if !validCookieValueByte(raw[i]) {
			return "", false
		}
	}
	return raw, true
}

func isCookieNameValid(raw string) bool {
	if raw == "" {
		return false
	}
	return strings.IndexFunc(raw, isNotToken) < 0
}

func validCookieValueByte(b byte) bool {
	return 0x20 <= b && b < 0x7f && b != '"' && b != ';' && b != '\\'
}

func isNotToken(r rune) bool {
	return !isToken(r)
}
func isToken(r rune) bool {
	i := int(r)
	return i < len(isTokenTable) && isTokenTable[i]
}

var isTokenTable = [127]bool{
	'!':  true,
	'#':  true,
	'$':  true,
	'%':  true,
	'&':  true,
	'\'': true,
	'*':  true,
	'+':  true,
	'-':  true,
	'.':  true,
	'0':  true,
	'1':  true,
	'2':  true,
	'3':  true,
	'4':  true,
	'5':  true,
	'6':  true,
	'7':  true,
	'8':  true,
	'9':  true,
	'A':  true,
	'B':  true,
	'C':  true,
	'D':  true,
	'E':  true,
	'F':  true,
	'G':  true,
	'H':  true,
	'I':  true,
	'J':  true,
	'K':  true,
	'L':  true,
	'M':  true,
	'N':  true,
	'O':  true,
	'P':  true,
	'Q':  true,
	'R':  true,
	'S':  true,
	'T':  true,
	'U':  true,
	'W':  true,
	'V':  true,
	'X':  true,
	'Y':  true,
	'Z':  true,
	'^':  true,
	'_':  true,
	'`':  true,
	'a':  true,
	'b':  true,
	'c':  true,
	'd':  true,
	'e':  true,
	'f':  true,
	'g':  true,
	'h':  true,
	'i':  true,
	'j':  true,
	'k':  true,
	'l':  true,
	'm':  true,
	'n':  true,
	'o':  true,
	'p':  true,
	'q':  true,
	'r':  true,
	's':  true,
	't':  true,
	'u':  true,
	'v':  true,
	'w':  true,
	'x':  true,
	'y':  true,
	'z':  true,
	'|':  true,
	'~':  true,
}
