package xhttp_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/atdiar/xhttp"
)

const (
	A = "A rainbow "
	B = "Be very "
	C = "Colorful "
)

func Example() {
	s := xhttp.NewServeMux()
	m := xhttp.Chain(middlewareExample{A, nil}, middlewareExample{B, nil}, middlewareExample{C, nil})
	s.USE(m)

	s.GET("/go/14", xhttp.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		a := req.Context().Value(A)
		if a == nil {
			fmt.Fprint(res, "Couldn't find a value in the context object for A") // shall make the test fail
		}
		fmt.Fprint(res, a)

		b := req.Context().Value(B)
		if b == nil {
			fmt.Fprint(res, "Couldn't find a value in the context object for B") // shall make the test fail
		}
		fmt.Fprint(res, b)

		c := req.Context().Value(C)
		if c == nil {
			fmt.Fprint(res, "Couldn't find a value in the context object for C") // shall make the test fail
		}
		fmt.Fprint(res, c)
	}))

	s.GET("/test", xhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "test")
	}))

	s.GET("/test/3564", xhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))

	s.POST("/test/3564", xhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "this is a post request")
	}))

	req, err := http.NewRequest("GET", "http://example.com/go/14", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	fmt.Printf("%d - %s", w.Code, w.Body.String())
	// Output: 200 - OK OK OK A rainbow Be very Colorful
}

// Let's create a catchAll Handler i.e. an object implementing HandlerLinker.
// It's also what people may call a catchall middleware.
// This should illustrate one of the form a HandlerLinker can take.
type middlewareExample struct {
	string
	next xhttp.Handler
}

func (m middlewareExample) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "OK ")
	c := context.WithValue(r.Context(), m.string, m.string)
	r = r.WithContext(c)
	if m.next != nil {
		m.next.ServeHTTP(w, r)
	}
}

func (m middlewareExample) Link(h xhttp.Handler) xhttp.HandlerLinker {
	m.next = h
	return m
}
