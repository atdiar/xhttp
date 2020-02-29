// Package content enables one to serve a data range out of a http requested file.
// It is useful for media that are susceptible to a range request such as audio
// or video files.
//
// Example: https://stackoverflow.com/questions/8293687/sample-http-range-request-session
package content

import (
	"io"
	"net/http"
	"time"

	"context"

	"github.com/atdiar/xhttp"
)

// Server is an adapter for xhttp of a net/http handler that serves content.
// For further information, please refer to https://golang.org/pkg/net/http/#ServeContent
type Server struct {
	name    string
	modtime time.Time
	content io.ReadSeeker
	next    xhttp.Handler
}

// NewServer returns a http request handler in charge of serving content.
func NewServer(name string, modtime time.Time, content io.ReadSeeker) Server {
	return Server{
		name:    name,
		modtime: modtime,
		content: content,
		next:    nil,
	}
}

func (s Server) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	http.ServeContent(w, r, s.name, s.modtime, s.content)
	if s.next != nil {
		s.next.ServeHTTP(ctx, w, r)
	}
}

// Link registers a next request Handler to be called by ServeHTTP method.
// It returns the result of the linking.
func (s Server) Link(h xhttp.Handler) xhttp.HandlerLinker {
	s.next = h
	return s
}
