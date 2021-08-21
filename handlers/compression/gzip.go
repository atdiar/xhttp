// Package compression defines a response compressing Handler.
// It compresses the body of the http response sent back to a client.
package compression

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/atdiar/xhttp"
)

// Gzipper defines the structure of the response compressing Handler.
type Gzipper struct {
	pool *sync.Pool // useful here to recycle gzip buffers
	skip map[string]bool
	next xhttp.Handler
}

// NewHandler returns a response compressing Handler.
func NewHandler() Gzipper {
	g := Gzipper{}
	g.skip = map[string]bool{
		"GET":     false,
		"POST":    false,
		"PUT":     false,
		"PATCH":   false,
		"DELETE":  false,
		"HEAD":    false,
		"OPTIONS": false,
	}
	g.pool = &sync.Pool{New: func() interface{} { return gzip.NewWriter(nil) }}
	return g
}

// Skip is used to disable gzip compression for a given http method.
func (g Gzipper) Skip(method string) Gzipper {
	if _, ok := g.skip[strings.ToUpper(method)]; !ok {
		panic(method + " is not a valid method")
	}
	g.skip[method] = true
	return g
}

// This is a type of wrapper around a http.ResponseWriter which buffers data
// before compressing the whole and writing.
type compressingWriter struct {
	io.WriteCloser
	http.ResponseWriter
	p *sync.Pool
}

func newcompressingWriter(w http.ResponseWriter, p *sync.Pool) compressingWriter {
	w1 := p.Get()
	w2 := w1.(*gzip.Writer)
	w2.Reset(w)
	return compressingWriter{w2, w, p}
}

// Write is using the gzip writer Write method.
func (cw compressingWriter) Write(b []byte) (int, error) {
	if cw.ResponseWriter.Header().Get("Content-Type") == "" {
		cw.ResponseWriter.Header().Set("Content-Type", http.DetectContentType(b))
		cw.ResponseWriter.Header().Del("Content-Length")
	}
	return cw.WriteCloser.Write(b)
}

// Close flushes the compressed bytestring to the underlying ResponseWriter.
// Then it releases the gzip.Writer, putting it back into the Pool.
func (cw compressingWriter) Close() error {
	z := cw.WriteCloser.(*gzip.Writer)
	err := z.Flush()
	cw.p.Put(z)
	return err
}

func (cw compressingWriter) Wrappee() http.ResponseWriter { return cw.ResponseWriter }

// ServeHTTP handles a http.Request by gzipping the http response body and
// setting the right http Headers.
func (g Gzipper) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if mustSkip, exist := g.skip[strings.ToUpper(req.Method)]; exist && mustSkip {
		if g.next != nil {
			g.next.ServeHTTP(w, req)
		}
		return
	}
	// We create a compressingWriter that will enable
	//the response writing w/ Compression.
	wc := newcompressingWriter(w, g.pool)

	w.Header().Add("Vary", "Accept-Encoding")
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		if g.next != nil {
			g.next.ServeHTTP(w, req)
		}
		return
	}
	wc.Header().Set("Content-Encoding", "gzip")
	// All the conditions are present : we shall compress the data before writing
	// it out.
	if g.next != nil {
		g.next.ServeHTTP(wc, req)
	}
	err := wc.Close()
	if err != nil {
		panic(err)
	}
}

// Link registers a next request Handler to be called by ServeHTTP method.
// It returns the result of the linking.
func (g Gzipper) Link(h xhttp.Handler) xhttp.HandlerLinker {
	g.next = h
	return g
}
