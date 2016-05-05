package xhttp_test

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
)

func Example() {
	s := xhttp.NewServeMux()

	s.USE(middlewareExample{})

	s.GET("/", xhttp.HandlerFunc(func(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
		ctx.Put("test", "OK")
		val, err := ctx.Get("test")
		if err != nil {
			panic(err) // for the sake of the example, we will panic. Not idiomatic.
		}
		fmt.Fprint(res, val)
	}))

	req, err := http.NewRequest("GET", "http://example.com/", nil)
	if err != nil {
		log.Fatal(err)
	}

	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)

	fmt.Printf("%d - %s", w.Code, w.Body.String())
	// Output: 200 - OK OK
}

// Let's create a catchAll Handler i.e. an object implementing HandlerLinker.
// It's also what people may call a catchall middleware.
// This should illustrate one of the form a HandlerLinker can take.
type middlewareExample struct {
	next xhttp.Handler
}

func (m middlewareExample) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "OK ")
	if m.next != nil {
		m.next.ServeHTTP(ctx, w, r)
	}
}

func (m middlewareExample) CallNext(h xhttp.Handler) xhttp.HandlerLinker {
	m.next = h
	return m
}
