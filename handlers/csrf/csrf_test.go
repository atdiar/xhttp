package csrf

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

func TestAntiCSRF(t *testing.T) {
	//############################################################################
	// MOCK SERVER INITIALIZATION
	//############################################################################

	// Let's create the session and CSRF request handlers
	session := session.New("inhjfgnjikt9864687", session.DevStore())
	anticsrf := New(session).LaxMode()

	// Let's create the multiplexer and declare the routes
	r := xhttp.NewServeMux()
	r.USE(session, anticsrf)

	r.POST("/", xhttp.HandlerFunc(func(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
		token, err := anticsrf.TokenFromCtx(ctx)
		if err != nil {
			panic(err)
		}
		res.Write([]byte(token))
	}))

	r.GET("/", xhttp.HandlerFunc(func(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
		token, err := anticsrf.TokenFromCtx(ctx)
		if err != nil {
			panic(err)
		}
		res.Write([]byte(token))
	}))

	ts := httptest.NewServer(r)
	defer ts.Close()

	//############################################################################

	// Step 0: GET request
	// An anti-CSRF token shall be generated.
	req, err := http.NewRequest("GET", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimSpace(string(raw))

	s := RetrieveCookie(res.Header, "XSRF-TOKEN")
	if s == nil {
		t.Fatal("The anti-CSRF cookie does not exist. Weird!")
	}
	if body == "" {
		t.Fatal("Unexpected empty body.")
	}

	// Step 1: POST request.
	// Since no anti-CSRF token is sent, the server responds with a failure.
	// A new anti-CSRF token is generated.
	req, err = http.NewRequest("POST", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	body = strings.TrimSpace(string(raw))

	s = RetrieveCookie(res.Header, "XSRF-TOKEN")
	if s == nil {
		t.Fatal("The anti-CSRF cookie does not exist. Weird!")
	}
	if body != invalidToken {
		t.Fatalf("Expected failure: no antiCSRF header set. Got %v instead of %v", body, invalidToken)
	}

	// Step 2: second POST request
	// We send back the cookie and set the specific header.
	oldToken := s.Value
	req, err = http.NewRequest("POST", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(s)
	req.Header.Add(s.Name, s.Value)

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	body = strings.TrimSpace(string(raw))

	s = RetrieveCookie(res.Header, "XSRF-TOKEN")
	if s == nil {
		t.Fatal("The anti-CSRF cookie does not exist. Weird!")
	}
	if body == "" {
		t.Fatal("Unexpected empty body.")
	}

	if body == oldToken {
		t.Fatal("Expected a newly generated token but the previous one was sent.")
	}

	// Step 3: third POST request
	// We send back the cookie but the anti-CSRF header does not hold the right
	// value.
	req, err = http.NewRequest("POST", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(s)
	req.Header.Add(s.Name, "new value")

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	raw, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	body = strings.TrimSpace(string(raw))

	s = RetrieveCookie(res.Header, "XSRF-TOKEN")
	if s == nil {
		t.Fatal("The anti-CSRF cookie does not exist. Weird!")
	}
	if body == "" {
		t.Fatal("Unexpected empty body.")
	}
	if body != invalidToken {
		t.Fatal("Expected failure due to token invalidation.")
	}

	// Step 4: fourth POST requests
	// We send the request with the correct header but omitting the anti-csrf
	// cookie.
	req, err = http.NewRequest("POST", ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add(s.Name, s.Value)

	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	raw, err = ioutil.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	body = strings.TrimSpace(string(raw))

	s = RetrieveCookie(res.Header, "XSRF-TOKEN")
	if s == nil {
		t.Fatal("The anti-CSRF cookie does not exist. Weird!")
	}
	if body == "" {
		t.Fatal("Unexpected empty body.")
	}
	if body != invalidToken {
		t.Fatal("Expected failure due to token invalidation.")
	}
}

// #############################################################################
// The below is extracted from Go's standard library and is used simply to
// retrieve a cookie that has been set in a http.Header.
//
/*
Copyright (c) 2009 The Go Authors. All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google Inc. nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/
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
