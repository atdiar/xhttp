// Package file enables the serving of a static file or an entire directory.
package file

import (
	"net/http"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
)

// Server is an xhttp adapter of a net/http handler that serves the content
// of a named file or directory.
// For further information, please refer to https://golang.org/pkg/net/http/#ServeFile
type Server struct {
	pathname string
	next     xhttp.Handler
}

// NewServer returns a http request handler in charge of serving the content of
// a file or directory.
func NewServer(path string) Server {
	return Server{
		pathname: path,
		next:     nil,
	}
}

func (s Server) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, s.pathname)
	if s.next != nil {
		s.next.ServeHTTP(ctx, w, r)
	}
}

// Link registers a next request Handler to be called by ServeHTTP method.
// It returns the result of the linking.
func (s Server) Link(nh xhttp.Handler) xhttp.HandlerLinker {
	s.next = nh
	return s
}
