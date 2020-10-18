// Package dynamux defines a multiplexing helper for url generated at runtime.
package dynamux

import (
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/atdiar/xhttp"
)

type contextKey struct{}

// Link defines the structure of a url generated at runtime which can collect
// application stats.
// This is an indirection url which enables processing to be done before the
// static resource is fetched or redirected to.
// Typical use would be url creation for uploaded resources.
// Persisting(storage, update, deletion) Links and retrieving them from the
// udatabase are tasks left to the user of this library.
type Link struct {
	UID string

	Path        string
	Destination *url.URL
	Proxy       *httputil.ReverseProxy `json:"-"`
	Client      *http.Client           `json:"-"`
	Active      bool
	// Owner string // whom the link was created on behalf of
	// RessourceID string
	// Referer string
	//ClickerSessionID string
	//ClickCount       int64
	CreatedAt time.Time
	MaxAge    time.Duration

	Handler xhttp.Handler

	contextKey *contextKey
}

// NewLink returns an indirection link pointing to a resource (destination URL).
// It is used by a Multiplexer which can then insert custom request handling for
// such dynamically generated links.
// maxage <0 means the link is expired
// maxage = 0 means the link doesn not expire
func NewLink(id string, path string, dest *url.URL, maxage time.Duration, proxy bool) Link {
	if proxy {
		return Link{id, path, dest, httputil.NewSingleHostReverseProxy(dest), &http.Client{}, true, time.Now().UTC(), maxage, nil, new(contextKey)}
	}
	return Link{id, path, dest, nil, nil, true, time.Now().UTC(), maxage, nil, new(contextKey)}
}

// WithHandler provides the link with a middleware request handling function that
// should trigger before any redirection or requets proxying for instance.
// Can be used typically to record link statistics.
func (l Link) WithHandler(h xhttp.Handler) Link {
	l.Handler = h
	return l
}

func (l Link) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if !l.Active {
		http.Error(w, "Error: Link is inactive", http.StatusNotFound)
		return
	}

	if l.MaxAge < 0 {
		http.Error(w, "Error: Link has expired", http.StatusNotFound)
		return
	}

	if time.Now().UTC().Before(l.CreatedAt.Add(l.MaxAge)) {
		http.Error(w, "Error: Link has expired", http.StatusNotFound)
		return
	}

	if l.Handler != nil {
		l.Handler.ServeHTTP(ctx, w, r)
	}

	if l.Proxy != nil {
		// l.Client should have been set
		forwarder := httptest.NewServer(l.Proxy)
		res, err := l.Client.Get(forwarder.URL)
		if err != nil {
			http.Error(w, "Could not fetch resource", http.StatusInternalServerError)
			return
		}
		b, err := ioutil.ReadAll(res.Body)
		if err != nil {
			http.Error(w, "Could not read fetched response body", http.StatusInternalServerError)
			return
		}
		w.Write(b)
		return
	}
	http.Redirect(w, r, l.Destination.String(), http.StatusTemporaryRedirect)
}

/* The way it should work:
1. Link is generated (an ID for the link is created and the link itself is  probably
a hash based version (salt+pepper){
}
2.The link is inserted in the Multiplexer object
3. On request, the Multiplexer calls its request handler ( for instance to record
statistics such as number of clicks) and then redirects via the
destination url.

*/

// Multiplexer is used to handle dynamically generated URLs.
type Multiplexer struct {
	mu *sync.RWMutex

	Links map[string]Link
}

// NewMultiplexer creates a new dynamic link handler for serving requests to these
// runtime generated links.
func NewMultiplexer() *Multiplexer {
	m := &Multiplexer{new(sync.RWMutex), make(map[string]Link)}
	return m
}

// AddLink inserts a new Link into the Multiplexer.
func (m *Multiplexer) AddLink(links ...Link) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, lnk := range links {
		m.Links[lnk.Path] = lnk
	}
}

func pathExists(url *url.URL, m *Multiplexer) (bool, string) {
	path := url.Path
	_, ok := m.Links[path]
	if ok {
		return ok, path
	}

	var longestpath string
	for route := range m.Links {
		if strings.HasSuffix(path, "/") {
			if strings.HasPrefix(route, path) {
				if len(route) > len(longestpath) {
					longestpath = route
					ok = true
				}
			}
		}
	}
	return ok, longestpath
}

func (m *Multiplexer) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	ok, dao := pathExists(r.URL, m)
	v, ok := m.Links[dao]
	if !ok {
		log.Print(dao, v)
		http.NotFound(w, r)
		return
	}
	v.ServeHTTP(ctx, w, r)
}
