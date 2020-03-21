package analytics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

type contextKey struct{}

type Link struct {
	Id      string
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
	SessionKey string

	Handlers map[string]xhttp.Handler
	Active   map[string]xhttp.Handler

	Links       map[string]Link
	ActiveLinks map[string]Link

	Session session.Interface
}

func (l *LinkServer) Load(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
	linksIDs, err := l.Session.Get(l.SessionKey)
	for k, handler := range l.Handlers {
		for _, link := range l.Links {
			if link.Id == k {
				if l.Active != nil {
					l.Active = make(map[string]xhttp.Handler)
				}
				l.Active[k] = handler

			}
			if l.ActiveLinks != nil {
				l.Active = make(map[string]Link
			}
			l.ActiveLinks[k] = l.Links[k]
		}
		}
	}
	return ctx
}

func (l *LinkServer) New(link Link, h xhttp.Handler) error {
	if l.Session == nil {
		return errors.New("session has not been correctly instantiated. Interface is nil.")
	}
	if l.Handlers == nil {
		l.Handlers = make(map[string]xhttp.Handler)
	}
	url := link.URL.String()
	l.Handlers[url] = h
	l.Active[url] = h
	l.Links[url] = link
	l.ActiveLinks[url]=link
	val, err := l.Session.Get(l.SessionKey)
	if err != nil {
		return err
	}
	nl, err := addLinkToJSON(val, link)
	if err != nil {
		return err
	}
	return l.Session.Put(l.SessionKey, nl, 0)
}

func addLinkToJSON(b []byte, link Link) ([]byte, error) {
	links := make([]Link, 0)
	err := json.Unmarshal(b, &links)
	if err != nil {
		return b, err
	}
	nlinks := append(links, link)
	b, err = json.Marshal(nlinks)
	if err != nil {
		return b, err
	}
	return b, nil
}

func(l *LinkServer) Activate(url string) error {
	link,ok:= l.Links[url]
	if !ok{
		return errors.New("TRLINKS: no link found for this url")
	}
	l.ActiveLinks[url] = link
	return nil
}

func(l *LinkServer) Deactivate(url string) error{
	link,ok:= l.Links[url]
	if !ok{
		return errors.New("TRLINKS link found for this url")
	}
	delete(l.ActiveLinks,url)
	return nil
}

func (l *LinkServer) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	url := r.URL.String()
	h, ok := l.Handlers[url]
}

func NewLink() Link                                                                     {}
func (l Link) ClickCount() int64                                                        {}
func (l *Link) Referer() string                                                         {}
func (l *Link) LoadStats(ctx context.Context, w http.ResponseWriter, r *http.Request)   {}
func (l *Link) UpdateStats(ctx context.Context, w http.ResponseWriter, r *http.Request) {}
func (l *link) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request)   {}
func (l *link) Link(hn xhttp.Handler) xhttp.HandlerLinker                               {}
