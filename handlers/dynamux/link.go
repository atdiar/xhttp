// Package dynamux defines a multiplexing helper for url generated at runtime.
package dynamux

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/atdiar/errcode"
	"github.com/atdiar/errors"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

type contextKey struct{}

// Link defines the structure of a url generated at runtime which can collect
// application stats. It is not used to track people across the internet.
// This is an indirection url which enables processing to be done before the
// resource is fetched. Typical use would be url creation for uploaded resources.
type Link struct {
	UID string

	Path       string
	RedirectTo *url.URL
	Active     bool
	// OwnerID string // whom the link was created on behalf of
	// Referer *url.URL
	//ClickerSessionID string
	//ClickCount       int64
	CreatedAt time.Time
	MaxAge    time.Duration

	contextKey *contextKey
}

// NewLink returns an indirection link pointing to a resource (destination URL).
// It is used by a Multiplexer which can then insert custom request handling for
// such dynamically generated links.
// maxage <0 means the link is expired
// maxage = 0 means the link doesn not expire
func NewLink(id string, path string, dest *url.URL, maxage time.Duration) Link {
	return Link{id, path, dest, true, time.Now().UTC(), maxage, new(contextKey)}
}

/* The way it should work:
1. Link is generated (an ID for the link is created and the link itself is  probably
a hash based version (salt+pepper)
2.The link is inserted in the Multiplexer object
3. On request, the Multiplexer calls its request handler ( fo instance to record
statistics such as number of clicks) and then redirects via the
destination url.

*/

// Multiplexer is used to handle dynamically generated URLs.
type Multiplexer struct {
	mu         *sync.RWMutex
	SessionKey string

	Links map[string]Link

	Session session.Interface

	Handler func(Link) xhttp.Handler
	next    xhttp.Handler
}

// NewMultiplexer creates a new dynamic link handler for serving requests to these
// runtime generated links.
func NewMultiplexer(sessionKey string, s session.Interface, LinkHandler func(Link) xhttp.Handler) (*Multiplexer, error) {
	m := &Multiplexer{new(sync.RWMutex), sessionKey, make(map[string]Link), s, LinkHandler, nil}
	return m, m.upload()
}

// AddLink inserts a new Link into the Multiplexer.
func (m *Multiplexer) AddLink(links ...Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	err := m.download()
	if err != nil {
		return err
	}
	for _, lnk := range links {
		m.Links[lnk.Path] = lnk
	}
	return m.upload()
}

// download makes available for usage the links that have been persisted away.
func (m *Multiplexer) download() error {

	Links, err := m.Session.Get(m.SessionKey)
	if err != nil {
		if errors.As(err).Is(errcode.NoID) {
			return m.upload()
		}
		return errors.New("Could not download links from session. \n" + err.Error())
	}
	return json.Unmarshal(Links, &(m.Links))
}

// upload brings back the changes to the persistence layer
func (m *Multiplexer) upload() error {

	b, err := json.Marshal(m.Links)
	if err != nil {
		return err
	}
	return m.Session.Put(m.SessionKey, b, 0)
}

// ActivateLink activates the http request handling for a link.
func (m *Multiplexer) ActivateLink(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	err := m.download()
	if err != nil {
		return err
	}

	link, ok := m.Links[path]
	if !ok {
		return errors.New("TRLINKS: no link found for this url")
	}
	link.Active = true
	m.Links[path] = link

	err = m.upload()
	if err != nil {
		return err
	}

	return nil
}

// DeactivateLink deactivates the http request handling for a given link.
func (m *Multiplexer) DeactivateLink(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	err := m.download()
	if err != nil {
		return err
	}

	link, ok := m.Links[path]
	if !ok {
		return errors.New("TRLINKS: no link found for this url")
	}
	link.Active = false
	m.Links[path] = link

	err = m.upload()
	if err != nil {
		return err
	}

	return nil
}

// ExpireLink does as its name suggests.
func (m *Multiplexer) ExpireLink(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	err := m.download()
	if err != nil {
		return err
	}

	link, ok := m.Links[path]
	if !ok {
		return errors.New("TRLINKS: no link found for this url")
	}
	link.MaxAge = -1
	m.Links[path] = link

	err = m.upload()
	if err != nil {
		return err
	}

	return nil
}

func (m *Multiplexer) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	err := m.download()
	if err != nil {
		http.Error(w, "Error: Could not find the links \n"+err.Error(), http.StatusInternalServerError)
		return
	}
	v, ok := m.Links[path]
	if !ok {
		http.Error(w, "Error: Link broken or missing", http.StatusInternalServerError)
		return
	}
	if !v.Active {
		http.Error(w, "Error: Link is inactive", http.StatusInternalServerError)
		return
	}

	if v.MaxAge < 0 {
		http.Error(w, "Error: Link has expired", http.StatusInternalServerError)
		return
	}

	if time.Now().UTC().Before(v.CreatedAt.Add(v.MaxAge)) {
		http.Error(w, "Error: Link has expired", http.StatusInternalServerError)
		return
	}

	if m.Handler != nil {
		m.Handler(v).ServeHTTP(ctx, w, r)
	}

	if m.next != nil {
		m.next.ServeHTTP(ctx, w, r)
	}
	return
}

/*
The way the library is used, the linkHandler is created before the server starts
and needs to be registered into the multiplexer
The destination url link handlers will have been registered for the corresponding url scheme
The indirect link handler is the same for all links registered in a single Multiplexer.
So for differentiated handling, multiple linkHanfdler need to be instantiated,
or the linkhandler should account for it in the ServeHTTP method code.
*/
