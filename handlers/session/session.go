// Package session defines a request handler that helps for the instantiation
// of client/server sessions.
package session

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"log"
	"net/http"
	"time"

	"context"

	"github.com/atdiar/errcode"
	"github.com/atdiar/errors"
	"github.com/atdiar/flag"
	"github.com/atdiar/xhttp"
	"github.com/satori/go.uuid"
)

var (
	// ErrNoID is returned when no session ID was found or the value was invalid.
	ErrNoID = errors.New("No id or Invalid id.").Code(errcode.NoID)
	// ErrBadSession is returned when the session is in an invalid state.
	ErrBadSession = errors.New("Session may have been compromised or does not exist.").Code(errcode.BadSession)
	// ErrBadCookie is returned when the session cookie is invalid.
	ErrBadCookie = errors.New("Bad session cookie. Retry.").Code(errcode.BadCookie)
	// ErrNoCookie is returned when the session cookie is absent
	ErrNoCookie = errors.New("Session Cookie absent.").Code(errcode.BadCookie)
	// ErrBadStorage is returned when session storage is faulty.
	ErrBadStorage = errors.New("Invalid storage.").Code(errcode.BadStorage)
	// ErrExpired is returned when the session has expired.
	ErrExpired = errors.New("Session has expired.").Code(errcode.Expired)
	// ErrKeyNotFound is returned when getting the value for a given key from the cookie
	// store failed.
	ErrKeyNotFound = errors.New("Key missing or expired").Code(errcode.KeyNotFound)
)

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
	Save(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error)
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

	h.Cookie = NewCookie(name, secret, 0, "")
	h.uuidgen = func() (string, error) {
		return uuid.NewV4().String(), nil
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

func SetUUIDgenerator(f func() (string, error)) func(Handler) Handler {
	return func(h Handler) Handler {
		h.uuidgen = f
		return h
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
			return nil, ErrNoID
		}
		return []byte(v), nil
	}
	if h.CachingEnabled.IsTrue() {
		res, err := h.Cache.Get(id, key)
		if err != nil {
			err2 := h.Cache.Clear()
			if err2 != nil {
				h.CachingEnabled.Flip()
				if h.Log != nil {
					h.Log.Println(err, err2)
				}
			}
		} else {
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

// Load will try to recover the session handler state if it was previously
// handled. Otherwise, it will try loading the metadata directly from the request
// object if it exists. If none works, an error is returned.
// Not safe for concurrent use by multiple goroutines.
func (h Handler) Load(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	dt := ctx.Value(h.ContextKey)

	if dt == nil {
		// in this case, there is no session already loaded and saved.
		// we try to retrieve a session cookie.
		reqc, err := req.Cookie(h.Cookie.Config.Name)
		if err != nil {
			// We should generate a new session since there is no cookie at the next step
			return ctx, ErrNoCookie
		}
		err = h.Cookie.Decode(*reqc)
		if err != nil {
			if h.Log != nil {
				h.Log.Println(err)
			}
			return ctx, ErrBadCookie
		}
		return h.Save(ctx, res, req)
	}
	c := dt.(http.Cookie)
	h.Cookie.Config = &c
	if h.Cookie.UpdateFlag.IsTrue() {
		return h.Save(ctx, res, req)
	}
	return ctx, nil
}

// Save will update and keep the session data in the per-request context store.
// It needs to be called to apply session data changes.
// These changes entail a modification in the value of the session cookie.
// Not safe for concurrent use by multiple goroutines.
func (h Handler) Save(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	hc, err := h.Cookie.Encode()
	if err != nil {
		return ctx, err
	}
	res.Header().Add("Set-Cookie", hc.String())
	h.Cookie.UpdateFlag.Set(false)
	return context.WithValue(ctx, h.ContextKey, hc), nil
}

// Generate creates a completely new session.
func (h Handler) Generate(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {

	// 1. Create UUID
	id, err := h.uuidgen()
	if err != nil {
		return ctx, err
	}

	// 3. Update session cookie
	for k := range h.Cookie.Data {
		delete(h.Cookie.Data, k)
	}
	h.Cookie.SetID(id)
	h.Cookie.UpdateFlag.Set(true)

	return h.Save(ctx, res, req)
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

	if h.next != nil {
		h.next.ServeHTTP(c, res, req)
	}
}

// Link enables the linking of a xhttp.Handler to the session Handler.
func (h Handler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	h.next = hn
	return h
}

// OrderedGroup groups sessions by increasing priority order. It is useful When
// a user has several sessions still valid (unsigned, signed, admin etc) with
// different settings.
// For example, on authentication and user signing, we can switch from using an
// unsigned user session handler to the one for signed-in user.
// Typically, these sessions are not mutually exclusive meaning that using one
// session does not expire the other ones.
type OrderedGroup struct {
	Handlers []Handler
	next     xhttp.Handler
}

func NewOrderedGroup(sessions ...Handler) OrderedGroup {
	return OrderedGroup{sessions, nil}
}

// Get will retrieve the value corresponding to a given store key from
// the relevant session store.
// It finds out the relevant session by checking existence of the session
// ContextKey inside.
func (o OrderedGroup) Get(ctx context.Context, key string) (res []byte, err error) {
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
func (o OrderedGroup) Put(ctx context.Context, key string, value []byte, maxage time.Duration) error {
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
func (o OrderedGroup) Delete(ctx context.Context, key string) error {
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
func (o OrderedGroup) Load(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
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

// Save will update and keep the session data in the per-request context store.
// It needs to be called to apply session data changes.
// These changes entail a modification in the value of the  relevant session cookie.
// Not safe for concurrent use by multiple goroutines.
func (o OrderedGroup) Save(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	if o.Handlers == nil {
		return ctx, nil
	}
	for i := len(o.Handlers) - 1; i >= 0; i++ {
		if v := ctx.Value(o.Handlers[i].ContextKey); v != nil {
			return o.Handlers[i].Save(ctx, res, req)
		}
		continue
	}
	return ctx, nil
}

// Generate creates a completely new session corresponding to a given session ContextKey.
func (o OrderedGroup) Generate(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
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
func (o OrderedGroup) ServeHTTP(ctx context.Context, res http.ResponseWriter, req *http.Request) {
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
func (o OrderedGroup) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	o.next = hn
	return o
}

// Group defines an ensemble of session handlers that can be used for a specific
// http route. only one sesssion per group can be used to process a http request.
// Hence, the sessions in a group are mutually exclusive.
type Group struct {
	Handlers map[*contextKey]Handler
	next     xhttp.Handler
}

// Get will retrieve the value corresponding to a given store key from
// the relevant session store.
// It finds out the relevant session by checking existence of the session
// ContextKey inside.
func (g Group) Get(ctx context.Context, key string) (res []byte, err error) {
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
func (o Group) Put(ctx context.Context, key string, value []byte, maxage time.Duration) error {
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
func (o Group) Delete(ctx context.Context, key string) error {
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
func (o Group) Load(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
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

// Save will update and keep the session data in the per-request context store.
// It needs to be called to apply session data changes.
// These changes entail a modification in the value of the  relevant session cookie.
// Not safe for concurrent use by multiple goroutines.
func (o Group) Save(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	if o.Handlers == nil {
		return ctx, nil
	}
	for k, v := range o.Handlers {
		if ctx.Value(k) != nil {
			return v.Save(ctx, res, req)
		}
		return ctx, nil
	}
	return ctx, nil
}

// Generate creates a completely new session corresponding to a given session ContextKey.
func (o Group) Generate(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
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
func (g Group) ServeHTTP(ctx context.Context, res http.ResponseWriter, req *http.Request) {
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
func (g Group) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	g.next = hn
	return g
}

func NewGroup(sessions ...Handler) Group {
	m := make(map[*contextKey]Handler)
	for _, session := range sessions {
		m[session.ContextKey] = session
	}
	return Group{m, nil}
}

// ComputeHmac256 returns a base64 Encoded MAC.
func ComputeHmac256(message, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(message)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// VerifySignature checks the integrity of the metadata whose MAC was computed.
func VerifySignature(messageb64, messageMAC, secret string) (bool, error) {
	message, err := base64.StdEncoding.DecodeString(messageb64)
	if err != nil {
		return false, err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	expectedMAC := mac.Sum(nil)
	return hmac.Equal([]byte(messageMAC), expectedMAC), nil
}
