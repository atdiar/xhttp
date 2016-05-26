// Package session defines a request handler that helps for the instantiation
// of client/server sessions.
package session

/*
Package description
===================

The session package contains three files:

* session.go
* metadata.go
* localmemstore.go

session.go defines a xhttp.Handler type which creates a session per
request.

metadata.go defines the format of session data that is marshalled/unmarshalled
to/from the session cookie.

localstorage.go defines a simple implementation of a session store for
development purpose. It should not be used in production.
*/

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/localmemstore"
	"github.com/atdiar/xhttp"
	"github.com/satori/go.uuid"
)

// TODO :
// - logging
// - Tests
// build tag for localmemstore
const (
	maxInt32 = 1<<31 - 1
	maxInt64 = 1<<63 - 1
)

var (
	// ErrNoID is returned when no session ID was found or the value was invalid.
	ErrNoID = errors.New("No id or Invalid id.")
	// ErrBadSession is returned when the session is in an invalid state.
	ErrBadSession = errors.New("Session may have been compromised or does not exist.")
	// ErrBadCookie is returned when the session cookie is invalid.
	ErrBadCookie = errors.New("Bad session cookie. Retry.")
	// ErrBadStorage is returned when session storage is faulty.
	ErrBadStorage = errors.New("Invalid storage.")
	// ErrExpired is returned when the session has expired.
	ErrExpired = errors.New("Session has expired.")
)

// Cache defines the interface that a session cache should implement.
// It should be made safe for concurrent use by multiple goroutines.
type Cache interface {
	Get(id, hkey string) (res []byte, err error)
	Put(id string, hkey string, content []byte) error
	Delete(id, hkey string) error
	Clear()
}

// Store defines the interface that a session store should implement.
// It should be made safe for concurrent use by multiple goroutines.
//
// NOTE: SetExpiry sets a timeout for the validity of a session.
// if t = 0, the session should expire immediately.
// if t < 0, the session does not expire.
type Store interface {
	Get(id, hkey string) (res []byte, err error)
	Put(id string, hkey string, content []byte) error
	Delete(id, hkey string) error
	SetExpiry(id string, t time.Duration) error
}

// Key is a type that allows to define a specific Key to be used in Key/Value
// stores to save/retrieve a session.
type Key struct{}

// Handler defines a type for request handling objects in charge of
// session instantiation and validation.
//
// The duration of a session server-side is not necessarily the same as the
// duration of the session credentials stored by the client.
// The latter is controlled by the MaxAge field of the session cookie.
type Handler struct {
	// Cookie is a template that stores the configuration for the storage of the
	// session object client-side.
	Cookie http.Cookie
	Secret string

	// Store is the interface implemented by server-side session stores.
	// duration represents the default length of a session server-side.
	// it may and probably is different from the cookie.Expire value that defines
	// the duration of client side session storage.
	Store    Store
	duration time.Duration

	Cache Cache

	uuidgen func() string

	Data data
	next xhttp.Handler
}

// Link enables the linking of a xhttp.Handler to the session Handler.
func (h Handler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	h.next = hn
	return h
}

// New returns a request handler which implements a server session
// initialized with defaults.
func New(secret string, s Store) Handler {
	h := Handler{}

	// Defaults (session cookie by default)
	h.Cookie.Name = "GSID"
	h.Cookie.Path = "/"
	h.Cookie.HttpOnly = true
	h.Cookie.Secure = true
	h.Cookie.MaxAge = 0 // session cookie is invalidated after browser is closed.
	h.duration = 86400

	if s == nil {
		panic("The provided session store is nil.")
	}
	h.Store = s

	if len(secret) == 0 {
		panic("The secret cannot be an empty string.")
	}
	h.Secret = secret

	h.Data = newToken()

	h.uuidgen = func() string {
		return uuid.NewV4().String()
	}

	return h
}

// EnableCaching allows to specify cache storage for the session data.
func (h Handler) EnableCaching(c Cache) Handler {
	h.Cache = c
	return h
}

// DisableCaching returns a new Handler with the caching disabled.
func (h Handler) DisableCaching() Handler {
	h.Cache = nil
	return h
}

// SetDuration defines the default duration limit for the server-side
// storage of session data.
func (h Handler) SetDuration(t time.Duration) Handler {
	h.duration = t
	return h
}

// ChangeUUIDgenerator allows to change the unique session ID generator used.
func (h Handler) ChangeUUIDgenerator(f func() string) Handler {
	h.uuidgen = f
	return h
}

// Key returns a value that is used to retrieve a saved session handler from
// the per-request context datastore.
func (h Handler) Key() Key {
	return Key{}
}

// *****************************************************************************
// Session handler UI
// *****************************************************************************

// Get will retrieve the value corresponding to a given store key from
// the session store.
func (h Handler) Get(key string) ([]byte, error) {

	if h.Cache == nil {
		return h.Store.Get(h.Data.GetID(), key)
	}

	res, err := h.Cache.Get(h.Data.GetID(), key)
	if err == nil {
		return res, nil
	}

	// On cache miss, we fetch from store and then try to update the cache
	// with the result before returning it.
	res, err = h.Store.Get(h.Data.GetID(), key)
	if err != nil {
		return nil, err
	}

	err = h.Cache.Put(h.Data.GetID(), key, res)
	if err != nil {
		log.Print(err) // log that caching failed.. TODO: build/plug-in a more powerful error logging system/service interface.
	}

	return res, nil
}

// Put will save a key/value pair in the session store.
func (h Handler) Put(key string, value []byte) error {

	err := h.Store.Put(h.Data.GetID(), key, value)
	if err != nil {
		return err
	}

	if h.Cache == nil {
		return nil
	}

	err = h.Cache.Put(h.Data.GetID(), key, value)
	if err != nil {
		log.Print(err) // Putting a value into the cache may not succeed. It's OK. Just log it as weird behaviour.
	}
	return nil
}

// Delete will erase a session store item.
func (h Handler) Delete(key string) error {

	if h.Cache == nil {
		return h.Store.Delete(h.Data.GetID(), key)
	}

	err := h.Cache.Delete(h.Data.GetID(), key) // Attempt to delete a value from cache MUST succeed.
	if err != nil {
		// the receiver of this error should be able to deal with this.
		// TODO add a method to return a handler with caching disabled.
		return err
	}

	err = h.Store.Delete(h.Data.GetID(), key)
	if err != nil {
		return err
	}
	return nil
}

// SetExpiry sets the duration of a single user session.
// On the server-side, the duration is set on the session storage.
// On the client-side, the duration is set on the client cookie.
//
// Hence, this function mutates session data in a way that should eventually
// be visible to the client.
//
// The rules are the following:
// * if t <0, the session expires immediately.
// * if t = 0, the session expires when the browser is closed. (browser session)
// * if t > 0, the session expires after t seconds.
func (h Handler) SetExpiry(t time.Duration) (Handler, error) {
	// server-side
	err := h.Store.SetExpiry(h.Data.GetID(), t)
	if err != nil {
		return h, err
	}
	// since cookie.MaxAge is an int type, we need to do a little gymnastic here
	// to avoid overflow.
	if t > maxInt32 {
		t = maxInt32
	}

	// client side
	h.Cookie.MaxAge = int(t)
	d := time.Now().UTC().Add(t)
	h.Cookie.Expires = d
	h.Data.SetExpiry(d)

	return h, nil
}

// SessionData returns the session data.
func (h Handler) SessionData() data {
	return h.Data.Retrieve()
}

// Load will try to recover the session handler state if it was previously
// handled. Otherwise, it will try loading the metadata directly from the request
// object if it exists. If none works, an error is returned.
// Not safe for concurrent use by multiple goroutines. (Would not make sense)
func (h *Handler) Load(ctx execution.Context, res http.ResponseWriter, req *http.Request) error {
	dt, err := ctx.Get(h.Key())

	if err != nil {
		// in this case, there is no session already laoded and saved.
		// we try to retrieve a session cookie.
		reqc, err := req.Cookie(h.Cookie.Name)
		if err != nil {
			return ErrBadSession
		}
		h.Cookie = *reqc
		err = h.Data.Decode(reqc.Value, h.Secret)
		if err != nil {
			// TODO session is invalid. Maybe it has been tampered with
			// log error and return invalid session error
			return ErrBadCookie
		}
		h.Save(ctx, res, req)
		return nil
	}
	sessiondata := dt.(data)
	h.Data = sessiondata
	h.Save(ctx, res, req)

	return nil
}

// Save will keep the session handler state in the per-request context store.
// It needs to be called to apply changes due to a session reconfiguration.
// Not safe for concurrent use by multiple goroutines.
func (h *Handler) Save(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
	if h.Data.IsUpdated() {
		h.generate(ctx, res, req)
	}
	ctx.Put(h.Key(), h.SessionData())
}

// Renew will regenerate the session with a brand new session id.
// This is the method to call when loading the session failed, for instance.
func (h *Handler) Renew(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
	h.generate(ctx, res, req)
	ctx.Put(h.Key(), h.SessionData())
}

// generate rebuilds a completely new session
// (clean-slate session cookie with clean-slate session data token)
func (h *Handler) generate(ctx execution.Context, res http.ResponseWriter, req *http.Request) {

	// 1. Create UUID
	var uUID string
	id := uuid.NewV4()
	uUID = id.String()

	// 2. Generate expiry date
	var expdate time.Time
	expdate = h.Cookie.Expires.UTC()
	if h.Cookie.MaxAge > 0 {
		expdate = time.Now().Add(time.Duration(h.Cookie.MaxAge) * time.Second).UTC()
		h.Cookie.Expires = expdate
	}

	// 3. Update Data token
	h.Data.SetID(uUID)
	h.Data.SetExpiry(expdate)

	// 4. Sets new cookie and save new session.
	h.Cookie.Value = h.Data.Encode(h.Secret)
	http.SetCookie(res, &(h.Cookie))
	h.Data.Update(false)
	ctx.Put(h.Key(), h)
}

// ServeHTTP effectively makes the session a xhttp request handler.
func (h Handler) ServeHTTP(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
	err := h.Load(ctx, res, req)

	if err != nil {
		http.Error(res, "Failed to load session.", 500)
		h.generate(ctx, res, req)
	}

	if h.next != nil {
		h.next.ServeHTTP(ctx, res, req)
	}
}

// computeHmac256 returns a base64 Encoded MAC.
func computeHmac256(message, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(message)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// verifySignature checks the integrity of the metadata whose MAC was computed.
func verifySignature(messageb64, messageMAC, secret string) (bool, error) {
	message, err := base64.StdEncoding.DecodeString(messageb64)
	if err != nil {
		return false, err
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(message)
	expectedMAC := mac.Sum(nil)
	return hmac.Equal([]byte(messageMAC), expectedMAC), nil
}

// DevStore returns a Key/Value datastructure implementing the Store
// interface for convenience during development.
// This is unsuitable for use in production.
func DevStore() localmemstore.Store {
	return localmemstore.New()
}
