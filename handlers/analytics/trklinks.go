package analytics

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"time"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

type contextKey struct{}

type Link struct {
	Id      int64
	OwnerId string

	URL        *url.URL
	RedirectTo *url.URL

	Referer *url.URL

	ClickerSessionID string
	Time             time.Time
	ClickCount       int64

	session session.Interface

	Persist    func(interface{}) (*sql.Stmt, error)
	ContextKey *contextKey

	next xhttp.Handler
}

/*
1. Link creation with Uniform Resource Locator A new handler should be pushed for
the given generic route handler.
map[url]tLinkHandler

*/

type LinkServer struct {
	Handlers map[string]xhttp.Handler
}

func (l *LinkServer) Push(link *url.URL, h xhttp.Handler) {
	if l.Handlers == nil {
		l.Handlers = make(map[string]xhttp.Handler)
	}
	l.Handlers[link.String()] = h
}

func (l *linkServer) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {

}

func Generate() Link                                                                    {}
func (l *Link) String() string                                                          {}
func (l Link) ClickCount() int64                                                        {}
func (l *Link) Referer() string                                                         {}
func (l *Link) LoadStats(ctx context.Context, w http.ResponseWriter, r *http.Request)   {}
func (l *Link) UpdateStats(ctx context.Context, w http.ResponseWriter, r *http.Request) {}
func (l *link) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request)   {}
func (l *link) Link(hn xhttp.Handler) xhttp.HandlerLinker                               {}
