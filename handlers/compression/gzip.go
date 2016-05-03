// Package compression defines a handler of incoming http requests
// which compresses the body of the response sent to the client.
package compression

import (
	"compress/gzip"
	"github.com/atdiar/context"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

// Handler defines the structure of the response compressing handler.
type Handler struct {
	pool *sync.Pool
	skip map[string]bool
}

// New returns a compressing xhttp.Handler object.
func New() Handler {
	h := Handler{}
	h.skip = map[string]bool{
		"GET":     false,
		"POST":    false,
		"PUT":     false,
		"PATCH":   false,
		"DELETE":  false,
		"HEAD":    false,
		"OPTIONS": false,
	}
	h.pool = &sync.Pool{New: func() interface{} { return gzip.NewWriter(nil) }}
	return h
}

// Skip is used to disable gzip compression for a given http method.
func (h Handler) Skip(method string) Handler {
	if _, ok := h.skip[strings.ToUpper(method)]; !ok {
		log.Panicf("%s is not a valid method", method)
	}
	h.skip[method] = true
	return h
}

// This is a wrapper around a http.ResponseWriter.
// It implements the xhttp.RWWrapper interface.
type compressingWriter struct {
	io.WriteCloser
	http.ResponseWriter
	p *sync.Pool
}

// Unwrap is the exported method implemented by wrappers around
// http.ResponseWriter objects.
// It returns the wrappee.
func (cw compressingWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
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

// ServeHTTP handles a http.Request by gzipping the http response body and
// setting the right http Headers.
func (h Handler) ServeHTTP(res http.ResponseWriter, req *http.Request, ctx context.Object) (http.ResponseWriter, bool) {
	if mustSkip, exist := h.skip[req.Method]; exist && mustSkip {
		return res, false
	}
	// We create a compressingWriter that will enable
	//the response writing w/ Compression.
	wc := newcompressingWriter(res, h.pool)

	res.Header().Add("Vary", "Accept-Encoding")
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
		return res, false
	}
	res.Header().Set("Content-Encoding", "gzip")

	return wc, false
}

// Finalize flushes the response to the underlying buffer.
// (If the response happens to have been compressed.)
func (h Handler) Finalize(w http.ResponseWriter, r *http.Request, ctx context.Object) error {
	cw, ok := w.(compressingWriter)
	if !ok {
		return nil
	}
	return cw.Close()
}
