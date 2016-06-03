package xhttp_test

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
)

const (
	A = "A "
	B = "B "
	C = "C "
)

func Example() {
	s := xhttp.NewServeMux()
	m := xhttp.Chain(middlewareExample{A, nil}, middlewareExample{B, nil}, middlewareExample{C, nil})
	s.USE(m)

	s.GET("/", xhttp.HandlerFunc(func(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
		a, err := ctx.Get(A)
		if err != nil {
			fmt.Fprint(res, err) // shall make the test fail
		}
		fmt.Fprint(res, a)

		b, err := ctx.Get(B)
		if err != nil {
			fmt.Fprint(res, err) // shall make the test fail
		}
		fmt.Fprint(res, b)

		c, err := ctx.Get(C)
		if err != nil {
			fmt.Fprint(res, err) // shall make the test fail
		}
		fmt.Fprint(res, c)
	}))

	req, err := http.NewRequest("GET", "http://example.com/", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	fmt.Printf("%d - %s", w.Code, w.Body.String())
	// Output: 200 - OK OK OK A B C
}

// Let's create a catchAll Handler i.e. an object implementing HandlerLinker.
// It's also what people may call a catchall middleware.
// This should illustrate one of the form a HandlerLinker can take.
type middlewareExample struct {
	string
	next xhttp.Handler
}

func (m middlewareExample) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "OK ")
	ctx.Put(m.string, m.string)
	if m.next != nil {
		m.next.ServeHTTP(ctx, w, r)
	}
}

func (m middlewareExample) Link(h xhttp.Handler) xhttp.HandlerLinker {
	m.next = h
	return m
}
