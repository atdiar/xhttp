// Package session defines a request handler that helps for the instantiation
// of client/server sessions.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"log"
	"net/http"
	"time"

	"github.com/atdiar/errcode"

	"github.com/atdiar/errors"
	"github.com/atdiar/flag"
	"github.com/atdiar/xhttp"
)

var (
	// ErrNoID is returned when no session ID was found or the value was invalid.
	ErrNoID = errors.New("No id or Invalid id.").Code(errcode.NoID)
	// ErrBadSession is returned when the session is in an invalid state.
	ErrBadSession = errors.New("Session may have been compromised or does not exist.").Code(errcode.BadSession)
	// ErrBadCookie is returned when the session cookie is invalid.
	ErrBadCookie = errors.New("Bad session cookie. Retry.").Code(errcode.BadCookie)
	// ErrNoCookie is returned when the session cookie is absent
	ErrNoCookie = errors.New("Session cookie absent.").Code(errcode.BadCookie)
	// ErrBadStorage is returned when session storage is faulty.
	ErrBadStorage = errors.New("Invalid storage.").Code(errcode.BadStorage)
	// ErrExpired is returned when the session has expired.
	ErrExpired = errors.New("Session has expired.").Code(errcode.Expired)
	// ErrKeyNotFound is returned when getting the value for a given key from the cookie
	// store failed.
	ErrKeyNotFound = errors.New("Key missing or expired").Code(errcode.KeyNotFound)
	// ErrNoSession is returned when no session has been found for loading
	ErrNoSession = errors.New("No session").Code(errcode.NoSession)
)

// todo mitigate session fixation

type contextKey struct{}

// ContextKey is used to retrieve a session cookie potentially stored in a context.
var ContextKey contextKey

// Cache defines the interface that a session cache should implement.
// It should be made safe for concurrent use by multiple goroutines as every
// session will most often share only one cache.
type Cache interface {
	Get(id, hkey string) (res []byte, err error)
	Put(id string, hkey string, content []byte, maxage time.Duration) error
	Delete(id, hkey string) error
	Clear() error
	ClearAfter(t time.Duration) error
}

// Store defines the interface that a session store should implement.
// It should be made safe for concurrent use by multiple goroutines as the
// server-side session store is very likely to be shared across sessions.
//
// N.B. When maxage is set for the validity of a key or the whole session:
// if t < 0, the key/session should expire immediately.
// if t = 0, the key/session has no set expiry.
type Store interface {
	Get(id, hkey string) (res []byte, err error)
	Put(id string, hkey string, content []byte, maxage time.Duration) error
	Delete(id, hkey string) error
	TimeToExpiry(id string, hkey string) (time.Duration, error)
}

// Interface defines a common interface for objects that are used for session
// management.
type Interface interface {
	Get(string) ([]byte, error)
	Put(key string, value []byte, maxage time.Duration) error
	Delete(key string) error
	Load(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error)
	SetSessionCookie(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error)
	Generate(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error)
}

// Handler defines a type for request handling objects in charge of
// session instantiation and validation.
//
// The duration of a session server-side is not necessarily the same as the
// duration of the session credentials stored by the client.
// The latter is controlled by the MaxAge field of the session cookie.
type Handler struct {
	Name   string
	Secret string

	// Cookie is the field that holds client side stored user session data
	// via a session cookie sent with every requests.
	Cookie Cookie

	// Handler specific context key under which  the session cookie is saved
	ContextKey *contextKey

	// Store is the interface implemented by server-side session stores.
	Store Store

	Cache          Cache
	CachingEnabled *flag.CcFlag

	uuidgen func() (string, error)

	Log *log.Logger

	next xhttp.Handler
}

// New creates a http request handler that deals with session management.
func New(name string, secret string, options ...func(Handler) Handler) Handler {
	h := Handler{}
	h.Name = name
	h.Secret = secret
	h.ContextKey = &contextKey{}
	h.CachingEnabled = flag.NewCC()
	h.CachingEnabled.Set(false) // by default

	h.Cookie = NewCookie(name, secret, 0)
	h.uuidgen = func() (string, error) {
		b := make([]byte, 70)
		_, err := rand.Read(b)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	if options != nil {
		for _, opt := range options {
			if opt != nil {
				h = opt(h)
			}
		}
	}
	return h
}

// Configure allows for further parametrization of the session handler.
func (h Handler) Configure(options ...func(Handler) Handler) Handler {
	if options != nil {
		for _, opt := range options {
			if opt != nil {
				h = opt(h)
			}
		}
	}
	return h
}

func SetCookie(c Cookie) func(Handler) Handler {
	return func(h Handler) Handler {
		h.Cookie = c
		return h
	}
}

// SetStore is a configuration option for the session that adds server-side storage.
// The presence of a store automatically transforms the session in a server-side
// one.Only the session id is stored in the session cookie.
func SetStore(s Store) func(Handler) Handler {
	return func(h Handler) Handler {
		h.Store = s
		return h
	}
}

func SetCache(c Cache) func(Handler) Handler {
	return func(h Handler) Handler {
		h.Cache = c
		h.CachingEnabled.Set(true)
		return h
	}
}

func FixedUUID(id string) func(Handler) Handler {
	return func(s Handler) Handler {
		s.uuidgen = func() (string, error) {
			return id, nil
		}
		s.Cookie.SetID(id)
		return s
	}
}

// *****************************************************************************
// Session handler UI
// *****************************************************************************

// Get will retrieve the value corresponding to a given store key from
// the session store.
func (h Handler) Get(key string) ([]byte, error) {
	id, ok := h.Cookie.ID()
	if !ok {
		return nil, ErrNoID
	}
	if h.Cache == nil {
		if h.Store != nil {
			return h.Store.Get(id, key)
		}
		v, ok := h.Cookie.Get(key)
		if !ok {
			return nil, ErrKeyNotFound
		}
		return []byte(v), nil
	}
	if h.CachingEnabled.IsTrue() {
		res, err := h.Cache.Get(id, key)
		if err != nil {
			err2 := h.Cache.Clear()
			if err2 != nil {
				// there must be a problem with the cache, let's turn it off.
				h.CachingEnabled.Flip()
				if h.Log != nil {
					h.Log.Println(err, err2)
				}
			}
		} else {
			// if we hit the cache, let's return the result.
			return res, nil
		}
	}
	// On cache miss, we fetch from store/cookiestore and then try to update the cache
	// with the result before returning it.
	if h.Store != nil {
		res, err := h.Store.Get(id, key)
		if err != nil {
			return nil, err
		}
		maxage, err := h.Store.TimeToExpiry(id, key)
		if err != nil {
			if h.Log != nil {
				h.Log.Print(err)
			}
			return res, nil
		}
		if h.CachingEnabled.IsTrue() {
			err = h.Cache.Put(id, key, res, maxage)
			if err != nil {
				err2 := h.Cache.Clear()
				if err2 != nil {
					h.CachingEnabled.Flip()
					if h.Log != nil {
						h.Log.Println(err, err2)
					}
				}
			}
		}

		return res, nil
	}
	v, ok := h.Cookie.Get(key)
	if !ok {
		return nil, ErrKeyNotFound
	}
	res := []byte(v)
	maxage, err := h.Store.TimeToExpiry(id, key)
	if err != nil {
		if h.Log != nil {
			h.Log.Print(err)
		}
		return res, nil
	}
	if h.CachingEnabled.IsTrue() {
		err = h.Cache.Put(id, key, res, maxage)
		if err != nil {
			err2 := h.Cache.Clear()
			if err2 != nil {
				h.CachingEnabled.Flip()
				if h.Log != nil {
					h.Log.Println(err, err2)
				}
			}
		}
	}
	return res, nil
}

// Put will save a key/value pair in the session store.
// if maxage < 0, the key/session should expire immediately.
// if maxage = 0, the key/session has no set expiry.
func (h Handler) Put(key string, value []byte, maxage time.Duration) error {
	id, ok := h.Cookie.ID()
	if !ok {
		return ErrNoID
	}
	if h.Store != nil {
		err := h.Store.Put(id, key, value, maxage)
		if err != nil {
			return err
		}
		if h.Cache == nil {
			return nil
		}
		if h.CachingEnabled.IsTrue() {
			err = h.Cache.Put(id, key, value, maxage)
			if err != nil {
				err2 := h.Cache.Clear()
				if err2 != nil {
					h.CachingEnabled.Flip()
					if h.Log != nil {
						h.Log.Println(err, err2)
					}
				}
			}
		}
		return nil
	}

	h.Cookie.Set(key, string(value), maxage)

	if h.Cache == nil {
		return nil
	}

	if h.CachingEnabled.IsTrue() {
		err := h.Cache.Put(id, key, value, maxage)
		if err != nil {
			err2 := h.Cache.Clear()
			if err2 != nil {
				h.CachingEnabled.Flip()
				if h.Log != nil {
					h.Log.Println(err, err2)
				}
			}
		}
	}

	return nil
}

// Delete will erase a session store item.
func (h Handler) Delete(key string) error {
	id, ok := h.Cookie.ID()
	if !ok {
		return ErrNoID
	}
	if h.Cache == nil {
		if h.Store != nil {
			return h.Store.Delete(id, key)
		}
		h.Cookie.Delete(key)
		return nil
	}
	if h.CachingEnabled.IsTrue() {
		err := h.Cache.Delete(id, key) // Attempt to delete a value from cache MUST succeed.
		if err != nil {
			err2 := h.Cache.Clear()
			if err2 != nil {
				h.CachingEnabled.Flip()
				if h.Log != nil {
					h.Log.Println(err, err2)
				}
			}
		}
	}

	err := h.Store.Delete(id, key)
	if err != nil {
		return err
	}
	return nil
}

// Load attempts to find the latest version of the session cookie that will be set
// in the response.
func (h *Handler) Load(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	c, ok := ctx.Value(h.ContextKey).(http.Cookie)
	if !ok {
		// in this case, there is no session cookie already set;
		// perhaps the session got modified in flight but the cookie was never set (let's log for this)
		// We try to retrieve a session cookie from the request.

		// in case the session is reloaded during requets handling but the session cookies has not been set
		if h.Cookie.ApplyMods.IsTrue() {
			if h.Log != nil {
				h.Log.Print("session cookie got modifications that have not been persisted by setting a http cookie")
			}
		}

		// Let's try to load a session cookie value from the request
		reqc, err := req.Cookie(h.Name)
		if err != nil {
			// at this point, should generate a new session since there is no session cookie
			// sent by the client.
			return context.WithValue(ctx, h.ContextKey, ErrBadSession), err
		}
		err = h.Cookie.Decode(*reqc)
		if err != nil {
			if h.Log != nil {
				h.Log.Println(errors.New("Bad cookie").Wraps(err))
			}
			return context.WithValue(ctx, h.ContextKey, ErrBadCookie), ErrBadCookie.Wraps(err)
		}
		h.Cookie.ApplyMods.Set(false)
		return context.WithValue(ctx, h.ContextKey, *(h.Cookie.HttpCookie)), nil
	}
	err := h.Cookie.Decode(c)
	if err != nil {
		return ctx, errors.New("couldn't load session cookie").Wraps(err)
	}
	return ctx, nil
}

// SetSessionCookie will modify and keep the session data in the per-request context store.
// It needs to be called to apply session data changes.
// These changes entail a modification in the value of the session cookie.
// The session cookie is stored in the context.Context non-encoded.
// Not safe for concurrent use by multiple goroutines.
func (h *Handler) SetSessionCookie(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	hc, err := h.Cookie.Encode()
	if err != nil {
		return ctx, err
	}
	http.SetCookie(res, &hc)
	h.Cookie.ApplyMods.Set(false)
	return context.WithValue(ctx, h.ContextKey, hc), nil
}

// Generate creates a completely new session.
func (h *Handler) Generate(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	// 1. Create UUID
	id, err := h.uuidgen()
	if err != nil {
		return ctx, err
	}

	// 2. Update session cookie
	for k := range h.Cookie.Data {
		delete(h.Cookie.Data, k)
	}
	h.Cookie.SetID(id)
	h.Cookie.ApplyMods.Set(true)

	return h.SetSessionCookie(ctx, res, req)
}

// ServeHTTP effectively makes the session a xhttp request handler.
func (h Handler) ServeHTTP(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	// We want any potential caching system to remain aware of changes to the
	// cookie header. As such, we have to add a Vary header.
	res.Header().Add("Vary", "Cookie")

	c, err := h.Load(ctx, res, req)
	if err != nil {
		c, err = h.Generate(c, res, req)
		if err != nil {
			http.Error(res, "Unable to generate session", http.StatusInternalServerError)
			return
		}
	}
	c, err = h.SetSessionCookie(c, res, req)
	if err != nil {
		http.Error(res, "Unable to set session cookie", http.StatusInternalServerError)
		return
	}

	if h.next != nil {
		h.next.ServeHTTP(c, res, req)
	}
}

// Link enables the linking of a xhttp.Handler to the session Handler.
func (h Handler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	h.next = hn
	return h
}

// Ordered groups sessions by increasing priority order. It is useful When
// a user has several sessions still valid (unsigned, signed, admin etc) with
// different settings.
// For example, on authentication and user signing, we can switch from using an
// unsigned user session handler to the one for signed-in user.
// Typically, these sessions are not mutually exclusive meaning that using one
// session does not expire the other ones.
type Ordered struct {
	Handlers []Handler
	next     xhttp.Handler
}

// SelectHighest returns a session management http request handler with sessions
// inserted from lowest priority to highest.
func SelectHighest(sessions ...Handler) Ordered {
	return Ordered{sessions, nil}
}

// Get will retrieve the value corresponding to a given store key from
// the relevant session store.
// It finds out the relevant session by checking existence of the session
// ContextKey inside.
func (o Ordered) Get(ctx context.Context, key string) (res []byte, err error) {
	if o.Handlers == nil {
		return nil, errors.New("No handler registered")
	}
	for i := len(o.Handlers) - 1; i >= 0; i++ {
		if v := ctx.Value(o.Handlers[i].ContextKey); v != nil {
			return o.Handlers[i].Get(key)
		}
		continue
	}
	return res, err
}

// Put will save a key/value pair in the relevant session store.
// It finds out the relevant session by checking existence of the session
// ContextKey inside.
func (o Ordered) Put(ctx context.Context, key string, value []byte, maxage time.Duration) error {
	if o.Handlers == nil {
		return errors.New("No handler registered")
	}
	for i := len(o.Handlers) - 1; i >= 0; i++ {
		if v := ctx.Value(o.Handlers[i].ContextKey); v != nil {
			return o.Handlers[i].Put(key, value, maxage)
		}
		continue
	}
	return nil
}

// Delete will erase a session store item from the relevant session.
// It finds out the relevant session by checking existence of the session
// ContextKey inside.
func (o Ordered) Delete(ctx context.Context, key string) error {
	if o.Handlers == nil {
		return errors.New("No handler registered")
	}
	for i := len(o.Handlers) - 1; i >= 0; i++ {
		if v := ctx.Value(o.Handlers[i].ContextKey); v != nil {
			return o.Handlers[i].Delete(key)
		}
		continue
	}
	return nil
}

// Load will try to recover the session handler state if it was previously
// handled. Otherwise, it will try loading the metadata directly from the request
// object if it exists. If none works, an error is returned.
// Not safe for concurrent use by multiple goroutines.
func (o Ordered) Load(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	if o.Handlers == nil {
		return ctx, errors.New("No handler registered")
	}
	for i := len(o.Handlers) - 1; i >= 0; i++ {
		if v := ctx.Value(o.Handlers[i].ContextKey); v != nil {
			return o.Handlers[i].Load(ctx, res, req)
		}
		continue
	}
	return ctx, nil
}

// SetSessionCookie will update and keep the session data in the per-request context store.
// It needs to be called to apply session data changes.
// These changes entail a modification in the value of the  relevant session cookie.
// Not safe for concurrent use by multiple goroutines.
func (o Ordered) SetSessionCookie(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	if o.Handlers == nil {
		return ctx, nil
	}
	for i := len(o.Handlers) - 1; i >= 0; i++ {
		if v := ctx.Value(o.Handlers[i].ContextKey); v != nil {
			return o.Handlers[i].SetSessionCookie(ctx, res, req)
		}
		continue
	}
	return ctx, nil
}

// Generate creates a completely new session corresponding to a given session ContextKey.
func (o Ordered) Generate(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	if o.Handlers == nil {
		return ctx, nil
	}
	for i := len(o.Handlers) - 1; i >= 0; i++ {
		if v := ctx.Value(o.Handlers[i].ContextKey); v != nil {
			return o.Handlers[i].Generate(ctx, res, req)
		}
		continue
	}
	return ctx, nil
}

// ServeHTTP effectively makes the session a xhttp request handler.
func (o Ordered) ServeHTTP(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	// We want any potential caching system to remain aware of changes to the
	// cookie header. As such, we have to add a Vary header.
	res.Header().Add("Vary", "Cookie")

	c, err := o.Load(ctx, res, req)

	if err != nil {
		c, err = o.Generate(c, res, req)
		if err != nil {
			http.Error(res, "Unable to generate session", http.StatusInternalServerError)
			return
		}
	}

	if o.next != nil {
		o.next.ServeHTTP(c, res, req)
	}
}

// Link enables the linking of a xhttp.Handler to the session Handler.
func (o Ordered) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	o.next = hn
	return o
}

//  Grouped defines an ensemble of session handlers that can be used for a specific
// http route. only one sesssion per group can be used to process a http request.
// Hence, the sessions are mutually exclusive.
type Grouped struct {
	Handlers map[*contextKey]Handler
	next     xhttp.Handler
}

func SelectFrom(sessions ...Handler) Grouped {
	m := make(map[*contextKey]Handler)
	for _, session := range sessions {
		m[session.ContextKey] = session
	}
	return Grouped{m, nil}
}

// Get will retrieve the value corresponding to a given store key from
// the relevant session store.
// It finds out the relevant session by checking existence of the session
// ContextKey inside.
func (g Grouped) Get(ctx context.Context, key string) (res []byte, err error) {
	if g.Handlers == nil {
		return nil, errors.New("No handler registered")
	}
	for k, v := range g.Handlers {
		if ctx.Value(k) != nil {
			return v.Get(key)
		}
		return res, errors.New("Session: handler nil")
	}
	return res, err
}

// Put will save a key/value pair in the relevant session store.
// It finds out the relevant session by checking existence of the session
// ContextKey inside.
func (o Grouped) Put(ctx context.Context, key string, value []byte, maxage time.Duration) error {
	if o.Handlers == nil {
		return errors.New("No handler registered")
	}

	for k, v := range o.Handlers {

		if ctx.Value(k) != nil {
			return v.Put(key, value, maxage)
		}
		return errors.New("Session: handler nil")
	}
	return nil
}

// Delete will erase a session store item from the relevant session.
// It finds out the relevant session by checking existence of the session
// ContextKey inside.
func (o Grouped) Delete(ctx context.Context, key string) error {
	if o.Handlers == nil {
		return errors.New("No handler registered")
	}

	for k, v := range o.Handlers {
		if ctx.Value(k) != nil {
			return v.Delete(key)
		}
		return errors.New("Session: handler nil")
	}
	return nil
}

// Load will try to recover the session handler state if it was previously
// handled. Otherwise, it will try loading the metadata directly from the request
// object if it exists. If none works, an error is returned.
// Not safe for concurrent use by multiple goroutines.
func (o Grouped) Load(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	if o.Handlers == nil {
		return ctx, errors.New("No handler registered")
	}
	for k, v := range o.Handlers {
		if ctx.Value(k) != nil {
			return v.Load(ctx, res, req)
		}
		return ctx, errors.New("Session: handler nil")
	}
	return ctx, nil
}

// SetSessionCookie will update and keep the session data in the per-request context store.
// It needs to be called to apply session data changes.
// These changes entail a modification in the value of the  relevant session cookie.
// Not safe for concurrent use by multiple goroutines.
func (o Grouped) SetSessionCookie(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	if o.Handlers == nil {
		return ctx, nil
	}
	for k, v := range o.Handlers {
		if ctx.Value(k) != nil {
			return v.SetSessionCookie(ctx, res, req)
		}
		return ctx, nil
	}
	return ctx, nil
}

// Generate creates a completely new session corresponding to a given session ContextKey.
func (o Grouped) Generate(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	if o.Handlers == nil {
		return ctx, nil
	}
	for k, v := range o.Handlers {
		if ctx.Value(k) != nil {
			return v.Generate(ctx, res, req)
		}
		return ctx, nil
	}
	return ctx, nil
}

// ServeHTTP effectively makes the session a xhttp request handler.
func (g Grouped) ServeHTTP(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	// We want any potential caching system to remain aware of changes to the
	// cookie header. As such, we have to add a Vary header.
	res.Header().Add("Vary", "Cookie")

	c, err := g.Load(ctx, res, req)

	if err != nil {
		c, err = g.Generate(c, res, req)
		if err != nil {
			http.Error(res, "", http.StatusInternalServerError)
		}
	}

	if g.next != nil {
		g.next.ServeHTTP(c, res, req)
	}
}

// Link enables the linking of a xhttp.Handler to the session Handler.
func (g Grouped) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	g.next = hn
	return g
}

// ComputeHmac256 returns a base64 Encoded MAC.
func ComputeHmac256(message, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(message)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// VerifySignature checks the integrity of the base64 encoded data whose MAC of its base64 decoding was computed.
func VerifySignature(messageb64, messageMAC, secret string) (bool, error) {
	message, err := base64.StdEncoding.DecodeString(messageb64)
	if err != nil {
		return false, err
	}
	mMAC, err := base64.StdEncoding.DecodeString(messageMAC)
	if err != nil {
		return false, err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	expectedMAC := mac.Sum(nil)
	return hmac.Equal([]byte(mMAC), expectedMAC), nil
}
