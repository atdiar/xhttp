// Package session defines a request handler that helps for the instantiation
// of client/server sessions.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	random "math/rand"
	"net/http"
	"time"

	"github.com/atdiar/errcode"

	"github.com/atdiar/errors"
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
	ErrKeyNotFound = errors.New("Key missing or expired.").Code(errcode.KeyNotFound)
	// ErrNoSession is returned when no session has been found for loading
	ErrNoSession = errors.New("No session.").Code(errcode.NoSession)
	// ErrParentInvalid is returned when the parent session is not present or invalid
	ErrParentInvalid = errors.New("Parent session absent or invalid")
)

var (
	sessionValidityKey = "sessionvalid?56dfh468s4hg54gsh"
	KeySID             = "@$ID@"
)

// todo deal with sessions that should not be regen on failure to load

type contextKey struct{}

// ContextKey is used to retrieve a session cookie potentially stored in a context.
var ContextKey contextKey

// Cache defines the interface that a session cache should implement.
// It should be made safe for concurrent use by multiple goroutines as every
// session will most often share only one cache.
type Cache interface {
	Get(ctx context.Context, id string, hkey string) (res []byte, err error)
	Put(ctx context.Context, id string, hkey string, content []byte, maxage time.Duration) error
	Delete(ctx context.Context, id string, hkey string) error
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
	Get(ctx context.Context, id string, hkey string) (res []byte, err error)
	Put(ctx context.Context, id string, hkey string, content []byte, maxage time.Duration) error
	Delete(ctx context.Context, id string, hkey string) error
	TimeToExpiry(ctx context.Context, id string, hkey string) (time.Duration, error)
}

// Interface defines a common interface for objects that are used for session
// management.
type Interface interface {
	ID() (string, error)
	SetID(string)
	Get(context.Context, string) ([]byte, error)
	Put(ctx context.Context, key string, value []byte, maxage time.Duration) error
	Delete(ctx context.Context, key string) error
	Load(res http.ResponseWriter, req *http.Request) error
	Save(res http.ResponseWriter, req *http.Request) error
	Generate(res http.ResponseWriter, req *http.Request)  error
}

// Handler defines a type for request handling objects in charge of
// session instantiation and validation.
//
// The duration of a session server-side is not necessarily the same as the
// duration of the session credentials stored by the client.
// The latter is controlled by the MaxAge field of the session cookie.
type Handler struct {
	parent *Handler
	Name   string
	Secret string

	// Cookie is the field that holds client side stored user session data
	// via a session cookie sent with every requests.
	Cookie     Cookie
	ServerOnly bool

	// Handler specific context key under which  the session cookie is saved
	ContextKey *contextKey

	// Store is the interface implemented by server-side session stores.
	Store Store
	Cache Cache

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

	h.Cookie = NewCookie(name, secret, 0)
	h.uuidgen = func() (string, error) {
		bstr := make([]byte, 32)
		_, err := rand.Read(bstr)
		if err != nil {
			random.Seed(time.Now().UnixNano())
			_, _ = random.Read(bstr)
		}
		return string(bstr), nil
	}

	if options != nil {
		for _, opt := range options {
			if opt != nil {
				h = opt(h)
			}
		}
	}
	if h.ServerOnly && h.Store == nil {
		panic(errors.New("error: serveronly session with no server storage").Error())
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

func SetMaxage(maxage int) func(Handler) Handler {
	return func(h Handler) Handler {
		h.Cookie.HttpCookie.MaxAge = maxage
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
		return h
	}
}

func SetUUIDgenerator(f func() (string, error)) func(Handler) Handler {
	return func(h Handler) Handler {
		h.uuidgen = f
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

func ServerOnly() func(Handler) Handler {
	return func(h Handler) Handler {
		h.ServerOnly = true
		return h
	}
}

// *****************************************************************************
// Session handler UI
// *****************************************************************************

// ID will return the client session ID if it has not expired. Otherwise it return an error.
func (h Handler) ID() (string, error) {
	id, ok := h.Cookie.ID()
	if !ok {
		return "", ErrNoID
	}
	return id, nil
}

// SetID will set a new id for a client navigation session.
func (h Handler) SetID(id string) {
	h.Cookie.SetID(id)
	h.Cookie.ApplyMods.Set(true)
}

// TODOD set client and server session id in context object?

// Get will retrieve the value corresponding to a given store key from
// the session.
func (h Handler) Get(ctx context.Context, key string) ([]byte, error) {
	id, ok := h.Cookie.ID()
	if !ok {
		return nil, ErrNoID
	}

	if h.Cache != nil {
		res, err := h.Cache.Get(ctx, id, h.Name+"/"+key)
		if err == nil {
			return res, err
		}
	}

	if h.Store != nil {
		_, err := h.Store.Get(ctx, id, h.Name+"/"+sessionValidityKey)
		if err != nil {
			return nil, ErrBadSession.Wraps(err)
		}
		// let's touch the session
		err = h.Touch(ctx)
		if err != nil {
			if h.Log != nil {
				h.Log.Print(err)
			}
		}

		res, err := h.Store.Get(ctx, id, h.Name+"/"+key)
		if err != nil {
			return nil, err
		}
		if h.Cache != nil {
			maxage, err := h.Store.TimeToExpiry(ctx, id, h.Name+"/"+key)
			if err != nil {
				if h.Log != nil {
					h.Log.Print(err)
				}
				return res, nil
			}
			err = h.Cache.Put(ctx, id, h.Name+"/"+key, res, maxage)
			if err != nil {
				if h.Log != nil {
					h.Log.Print(err)
				}
			}
		}
		return res, err
	}

	if h.ServerOnly {
		panic(errors.New("error: serveronly session with no server storage").Error())
	}

	v, ok := h.Cookie.Get(key)
	if !ok {
		return nil, ErrKeyNotFound
	}
	err := h.Touch(ctx)
	if err != nil {
		if h.Log != nil {
			h.Log.Print(err)
		}
	}
	res := []byte(v)
	if h.Cache != nil {
		maxage, err := h.Cookie.TimeToExpiry(key)
		if err != nil {
			if h.Log != nil {
				h.Log.Print(err)
			}
			return res, nil
		}
		err = h.Cache.Put(ctx, id, h.Name+"/"+key, res, maxage)
		if err != nil {
			if h.Log != nil {
				h.Log.Print(err)
			}
		}
	}
	return res, nil
}

// Put will save a key/value pair in the session store (preferentially).
// If no store is present, cookie storage will be used.
// if maxage < 0, the key/session should expire immediately.
// if maxage = 0, the key/session has no set expiry.
func (h Handler) Put(ctx context.Context, key string, value []byte, maxage time.Duration) error {
	id, ok := h.Cookie.ID()
	if !ok {
		return ErrNoID
	}

	if h.Store != nil {
		_, err := h.Store.Get(ctx, id, h.Name+"/"+sessionValidityKey)
		if err != nil {
			return ErrBadSession.Wraps(err)
		}

		err = h.Store.Put(ctx, id, h.Name+"/"+key, value, maxage)
		if err != nil {
			return err
		}
		// let's touch the session
		h.Cookie.Touch()
		if h.Cookie.HttpCookie.MaxAge > 0 {
			err = h.Store.Put(ctx, id, h.Name+"/"+sessionValidityKey, []byte("true"), time.Duration(h.Cookie.HttpCookie.MaxAge))
			if err != nil {
				if h.Log != nil {
					h.Log.Print(err)
				}
			}
		}

		if h.Cache == nil {
			return nil
		}
		err = h.Cache.Put(ctx, id, h.Name+"/"+key, value, maxage)
		if err != nil {
			if h.Log != nil {
				h.Log.Println(err)
			}
		}
		return nil
	}

	if h.ServerOnly {
		panic(errors.New("error: serveronly session with no server storage").Error())
	}

	h.Cookie.Set(key, string(value), maxage)

	// Let's touch the session
	if key != sessionValidityKey {
		h.Cookie.Touch()
	}

	if h.Cache == nil {
		return nil
	}

	err := h.Cache.Put(ctx, id, h.Name+"/"+key, value, maxage)
	if err != nil {
		if h.Log != nil {
			h.Log.Println(err)
		}
	}

	return nil
}

// Delete will erase a session store item.
func (h Handler) Delete(ctx context.Context, key string) error {
	id, ok := h.Cookie.ID()
	if !ok {
		return ErrNoID
	}

	if h.Cache == nil {
		err := h.Cache.Delete(ctx, id, h.Name+"/"+key) // Attempt to delete a value from cache MUST succeed.
		if err != nil {
			if h.Log != nil {
				h.Log.Println(err)
			}
		}
	}
	if h.Store != nil {
		_, err := h.Store.Get(ctx, id, h.Name+"/"+sessionValidityKey)
		if err != nil {
			return nil // the session is invalid anyway.
		}

		err = h.Store.Delete(ctx, id, h.Name+"/"+key)
		if err != nil {
			return err
		}

		err = h.Touch(ctx)
		if err != nil {
			if h.Log != nil {
				h.Log.Print(err)
			}
		}
		// attempt to touch the session
		if h.Cookie.HttpCookie.MaxAge > 0 {
			err = h.Store.Put(ctx, id, h.Name+"/"+sessionValidityKey, []byte("true"), time.Duration(h.Cookie.HttpCookie.MaxAge))
			if err != nil {
				if h.Log != nil {
					h.Log.Print(err)
				}
			}
		}
		return nil
	}
	if h.ServerOnly {
		panic(errors.New("error: serveronly session with no server storage").Error())
	}

	h.Cookie.Delete(key)

	err := h.Touch(ctx)
	if err != nil {
		if h.Log != nil {
			h.Log.Print(err)
		}
	}

	return nil
}

func (h Handler) Loaded(ctx context.Context) bool {
	_, ok := ctx.Value(h.ContextKey).(http.Cookie)
	return ok
}

// loadFromCookie recovers the session data from the session cookie sent by the client or,
// if already called before, attempts to find the latest version of the session
// cookie that will have been saved by using the Save method.
func (h Handler) loadCookie(res http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()
	// Let's try to load a session cookie value from the request
	reqc, err := req.Cookie(h.Name)
	if err != nil {
		// at this point, should generate a new session since there is no session cookie
		// sent by the client.
		req = req.WithContext(context.WithValue(ctx, h.ContextKey, ErrBadSession))
		return ErrBadSession.Wraps(err)
	}

	err = h.Cookie.Decode(*reqc)
	if err != nil {
		if h.Log != nil {
			h.Log.Println(errors.New("Bad cookie").Wraps(err))
		}
		req = req.WithContext(context.WithValue(ctx, h.ContextKey, ErrBadCookie))
		return ErrBadCookie.Wraps(err)
	}
	h.Cookie.ApplyMods.Set(false)

	if h.Store != nil {
		_, err = h.Get(ctx, sessionValidityKey)
		if err != nil {
			req = req.WithContext(context.WithValue(ctx, h.ContextKey, ErrBadSession))
			return ErrBadSession.Wraps(err)
		}
	}
	req = req.WithContext(context.WithValue(ctx, h.ContextKey, *(h.Cookie.HttpCookie)))
	return  nil
}

func (h *Handler) Load(res http.ResponseWriter, req *http.Request) error {
	ctx:= req.Context()
	if h.Loaded(ctx) {
		return nil
	}

	p, err := h.Parent()
	if err == nil {

		if !p.Loaded(ctx) {
			return ErrParentInvalid
		}

		pid, err := p.ID()
		if err != nil {
			return ErrParentInvalid.Wraps(err)
		}

		if !h.ServerOnly {
			err = h.loadCookie(res, req)
			if err != nil {
				return err
			}
		}

		id, err := h.ID()
		if err != nil {
			return ErrNoID
		}
		_, err = h.Get(ctx, sessionValidityKey)
		if err != nil {
			return ErrBadSession.Wraps(err)
		}

		psid, err := h.Get(ctx, p.Name+"/id")
		if err != nil {
			return ErrBadSession.Wraps(errors.New("Could not retrieve parent session id").Wraps(err))
		}
		if pid != string(psid) {
			return ErrParentInvalid.Wraps(errors.New("session parent was loaded but session parent id is not matching with id stored in its spawn. "))
		}
		_, err = p.Get(ctx, h.Name+"/"+id)
		if err != nil {
			return ErrBadSession.Wraps(errors.New("The session does not appear on its parent"))
		}

		return h.Save(res, req)
	}
	// if session has no parent
	if !h.ServerOnly {
		return h.loadCookie(res, req)
	}
	_, err = h.ID()
	if err != nil {
		return ErrNoID
	}
	_, err = h.Get(ctx, sessionValidityKey)
	if err != nil {
		return ErrBadSession.Wraps(err)
	}
	return h.Save(res, req)
}

// Save will modify and keep the session data in the per-request context store.
// It needs to be called to apply session data changes.
// These changes entail a modification in the value of the session cookie.
// The session cookie is stored in the context.Context non-encoded.
// Not safe for concurrent use by multiple goroutines.
func (h *Handler) Save(res http.ResponseWriter, req *http.Request) error {
	ctx:= req.Context()
	hc, err := h.Cookie.Encode()
	if err != nil {
		return err
	}
	if !h.ServerOnly {
		http.SetCookie(res, &hc)
	}
	h.Cookie.ApplyMods.Set(false)
	req = req.WithContext(context.WithValue(ctx, h.ContextKey, hc))
	return nil
}

// Generate creates a completely new session. with a new generated id.
func (h *Handler) Generate(res http.ResponseWriter, req *http.Request) error {
	ctx:=req.Context()
	// 1. Create UUID
	id, err := h.uuidgen()
	if err != nil {
		return  err
	}

	// 2. Update session cookie
	for k := range h.Cookie.Data {
		delete(h.Cookie.Data, k)
	}
	h.Cookie.SetID(id)
	h.Cookie.ApplyMods.Set(true)

	// 3.  Establish the session on the server if server storage is available
	err = h.Put(ctx, sessionValidityKey, []byte("true"), time.Duration(h.Cookie.HttpCookie.MaxAge))
	if err != nil {
		return errors.New("Failed to generate new session.").Wraps(err)
	}

	p, err := h.Parent()
	if err == nil {
		if !p.Loaded(ctx) {
			return ErrParentInvalid
		}
		err = h.Put(ctx, p.Name+"/id", []byte(id), 0)
		if err != nil {
			return err
		}
		err = p.Put(ctx, h.Name+"/"+id, Info(req).ToJSON(), 0)
	}

	return h.Save(res, req)
}

// Load is used to load a session which is only known server-side. (serve-only)
func LoadServerOnly(r *http.Request, id string, h *Handler) error {
	ctx:= r.Context()
	if !h.ServerOnly || h.Store == nil {
		return errors.New("Unable to load server session. Session Handler parameters are incorrect")
	}

	if h.Loaded(ctx) {
		sid, err := h.ID()
		if err != nil {
			goto load
		}
		if sid != id {
			goto load
		}
		return nil
	}

load:
	h.SetID(id)

	p, err := h.Parent()
	if err == nil {

		if !p.Loaded(ctx) {
			return ErrParentInvalid
		}

		pid, err := p.ID()
		if err != nil {
			return ErrParentInvalid.Wraps(err)
		}

		_, err = h.Get(ctx, sessionValidityKey)
		if err != nil {
			return ErrBadSession.Wraps(err)
		}

		psid, err := h.Get(ctx, p.Name+"/id")
		if err != nil {
			return ErrParentInvalid.Wraps(err)
		}
		if pid != string(psid) {
			return ErrParentInvalid.Wraps(errors.New("session parent was loaded but session parent id is not matching with id stored in its spawn. "))
		}
		_, err = p.Get(ctx, h.Name+"/"+id)
		if err != nil {
			return ErrBadSession.Wraps(errors.New("The session does not appear on its parent"))
		}
		hc, err := h.Cookie.Encode()
		if err != nil {
			return err
		}
		h.Cookie.ApplyMods.Set(false)
		r = r.WithContext(context.WithValue(ctx, h.ContextKey, hc))
		return nil
	}
	// if session has no parent
	_, err = h.Get(ctx, sessionValidityKey)
	if err != nil {
		return ErrBadSession.Wraps(err)
	}
	hc, err := h.Cookie.Encode()
	if err != nil {
		return err
	}
	h.Cookie.ApplyMods.Set(false)
	r = r.WithContext(context.WithValue(ctx, h.ContextKey, hc))
	return nil
}

// Generate will create and load in context.Context a new server-only session
// for a provided id if it does not already exist
func GenerateServerOnly(r *http.Request, id string, h *Handler)  error {
	ctx:= r.Context()
	h.SetID(id)
	_, err := h.Get(r.Context(), sessionValidityKey)
	if err == nil {
		err = LoadServerOnly(r, id, h)
		if err != nil {
			return errors.New("Session does already exist but could not be loaded").Wraps(err)
		}
		return err
	}
	err = h.Put(ctx, sessionValidityKey, []byte("true"), time.Duration(h.Cookie.HttpCookie.MaxAge))
	if err != nil {
		return err
	}

	p, err := h.Parent()
	if err == nil {
		if !p.Loaded(ctx) {
			return ErrParentInvalid
		}
		err = h.Put(ctx, p.Name+"/id", []byte(id), 0)
		if err != nil {
			return err
		}
		err = p.Put(ctx, h.Name+"/"+id, Info(r).ToJSON(), 0)
	}

	hc, err := h.Cookie.Encode()
	if err != nil {
		return err
	}
	h.Cookie.ApplyMods.Set(false)
	r = r.WithContext(context.WithValue(ctx, h.ContextKey, hc))
	return nil
}

// Spawn returns a handler for a subsession, that is, a dependent session.
func (h Handler) Spawn(name string, options ...func(Handler) Handler) Handler {
	sh := New(name, h.Secret, options...)
	sh.parent = &h
	return sh
}

// Spawned links to session into a Parent-Spawn dependent relationship.
// A session cannot spawn itself (i.e. session names have to be different).
func (h Handler) Spawned(s Handler) Handler {
	if h.Name != s.Name {
		s.parent = &h
	}
	return s
}

// Parent returns an unitialized copy of the handler of a Parent session if
// the aforementionned exists.
// To use a Parent session,the Load method should be called first.
func (h Handler) Parent() (Handler, error) {
	if h.parent != nil {
		res := *h.parent
		return res, nil
	}
	return h, ErrParentInvalid
}

// Revoke revokes the current session.
func (h Handler) Revoke(ctx context.Context) error {
	id, err := h.ID()
	if err != nil {
		return errors.New("Unable to revoke session. Could not retrieve session ID").Wraps(err)
	}
	h.Cookie.Expire()
	err = h.Delete(ctx, sessionValidityKey)
	if err != nil {
		return err
	}
	p, err := h.Parent()
	if err != nil {
		return nil
	}
	pid, err := h.Get(ctx, p.Name+"/id")
	if err != nil {
		if h.Log != nil {
			h.Log.Print(errors.New("Unable to recover parent session id for revocation.").Wraps(err))
		}
		return errors.New("Unable to recover parent session id for revocation.").Wraps(err)
	}
	p.SetID(string(pid))
	err = p.Delete(ctx, h.Name+"/"+id)
	if h.Log != nil {
		h.Log.Print(err)
	}
	return nil // we could return the error but it's not mandatory... we'll cleanup the parent session later.
}

func (h Handler) Touch(ctx context.Context) error {
	// sends the signal to send a session cookie back to the client to renew
	if !h.ServerOnly {
		h.Cookie.Touch()
		return nil
	}

	if h.Cookie.HttpCookie.MaxAge > 0 {
		return h.Put(ctx, sessionValidityKey, []byte("true"), time.Duration(h.Cookie.HttpCookie.MaxAge))
	}
	return nil
}

// ServeHTTP effectively makes the session a xhttp request handler.
func (h Handler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	// We want any potential caching system to remain aware of changes to the
	// cookie header. As such, we have to add a Vary header.
	res.Header().Add("Vary", "Cookie")

	err := h.Load(res, req)
	if err != nil {
	 	err = h.Generate(res, req)
		if err != nil {
			http.Error(res, "Unable to generate session", http.StatusInternalServerError)
			return
		}
	}
	err = h.Save(res, req)
	if err != nil {
		http.Error(res, "Unable to set session cookie", http.StatusInternalServerError)
		return
	}

	if h.next != nil {
		h.next.ServeHTTP(res, req)
	}
}

// Link enables the linking of a xhttp.Handler to the session Handler.
func (h Handler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	h.next = hn
	return h
}

type Metadata struct {
	Start     time.Time `json:"start"`
	UserAgent string    `json:"useragent"`
	IPAddress string    `json:"ipaddress"`
}

func (m Metadata) ToJSON() []byte {
	b, err := json.Marshal(m)
	if err != nil {
		panic(err) // should never happen unless we change the Metadata type  definition
	}
	return b
}

func Info(r *http.Request) Metadata {
	m := Metadata{}
	m.Start = time.Now().UTC()
	m.UserAgent = r.UserAgent()
	m.IPAddress = r.RemoteAddr
	return m
}

// Enforce return a handler whose purpose is tom make sure that the sessions are
// present before continuing with request handling.
func Enforcer(sessions ...Handler) xhttp.HandlerLinker {
	return xhttp.LinkableHandler(xhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := r.Context()
		var err error
		if len(sessions) != 0 {
			for _, s := range sessions { // TODO cancel context
				err = s.Load(w, r)
				if err != nil {
					http.Error(w, "Some session credentials are missing", http.StatusUnauthorized) // TODO perhaps create an enforcer that does not write the response but return a bool or something
					return
				}
				continue
			}
		}
		r=r.WithContext(c)
	}))
}

/*
// todo EnforceHighest

// Ordered groups sessions by decreasing priority order (index 0 is the highest priority).
// It is useful Wwen a user has several sessions still valid (unsigned, signed, admin etc)
// with different settings.
// For example, on authentication and user signing, we can switch from using an
// unsigned user session handler to the session handler for signed-in user.
// Typically, these sessions are not mutually exclusive meaning that using one
// session does not expire the other ones.
type Ordered struct {
	Handlers []Handler
	next     xhttp.Handler
}

// SelectHighestPriority returns a session management http request handler with sessions
// inserted from highest priority (index 0) to lowest.
func SelectHighestPriority(sessions ...Handler) Ordered {
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
	return ctx, errors.New("No session to load")
}

// todo create a Save method for Ordered sessions

// ServeHTTP effectively makes the session a xhttp request handler.
func (o Ordered) ServeHTTP(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	// We want any potential caching system to remain aware of changes to the
	// cookie header. As such, we have to add a Vary header.
	res.Header().Add("Vary", "Cookie")

	c, err := o.Load(ctx, res, req)

	if err != nil {
		http.Error(res, "Unable to load session", http.StatusInternalServerError)
		return
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

// Save will update and keep the session data in the per-request context store.
// It needs to be called to apply session data changes.
// These changes entail a modification in the value of the  relevant session cookie.
// Not safe for concurrent use by multiple goroutines.
func (o Grouped) Save(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
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
*/

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
