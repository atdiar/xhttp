// Package session defines a request handler that helps the instantiation
// of client/server sessions.
package session

/*
File description

The session package contains two files(session.go and localmemstore.go).

session.go defines a http request handler type which creates a session per
request.

localstorage.go defines a simple implementation of a session store for
development purpose. It should not be used in production.
*/

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/atdiar/context"
	"github.com/satori/go.uuid"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TODO :
// - logging
// - Tests
// - Copyright Disclaimer
// function for parameter initilization.. getter methods

var (
	ERRNOID       = errors.New("No id or Invalid id")
	ERRBADSESSION = errors.New("Session Compromised")
	ERRBADCOOKIE  = errors.New("Bad session cookie. Retry.")
	ERRBADSTORAGE = errors.New("Invalid storage")
	ERREXPIRED    = errors.New("Session has expired")
)

// Cache defines the interface that a session cache should implement.
// Needs to be safe for concurrent use.
type Cache interface {
	Get(id, hkey string) (res []byte, err error)
	Put(id string, hkey string, content []byte) error
	Delete(id, hkey string) error
	Clear()
}

// Store defines the interface that a session store frontend should implement.
// Needs to be safe for concurrent use.
type Store interface {
	Get(id, hkey string) (res []byte, err error)
	Put(id string, hkey string, content []byte) error
	Delete(id, hkey string) error

	// Expire sets the time from Now in seconds after which the session
	// should be considered stale.
	// if t = 0, the session should expire immediately.
	// if t < 0, the session does not expire.
	Expire(id string, t int64) error
}

// Handler defines a type for request handling objects in charge of
// session instantiation and validation.
type Handler struct {
	Params http.Cookie
	Secret string
	Store  Store
	Cache  Cache

	Transience int // Max time in store for a browser session (maxAge = 0)

	metadata metadata
}

// New returns a session handler initialized with defaults.
func New(secret string, store Store) Handler {
	h := Handler{}

	// Defaults (session cookie by default)
	h.Params.Name = "GSID"
	h.Params.Path = "/"
	h.Params.HttpOnly = true
	h.Params.Secure = true
	h.Params.MaxAge = 0 // session is invalidated after browser is closed.
	h.Transience = 86400

	if store == nil {
		panic("The provided session store is nil.")
	}
	h.Store = store

	h.Secret = "Alibaba loves Gombo soup ! Please change that bad secret !"
	if len(secret) != 0 {
		h.Secret = secret
	}

	h.metadata = newSessionData()

	return h
}

// Session handler API

// Get will retrieve the value corresponding to a given store key from
// the session store.
// Safe for concurrent use
func (h Handler) Get(key string) ([]byte, error) {

	if h.Cache == nil {
		return h.Store.Get(h.metadata.ID(), key)
	}

	res, err := h.Cache.Get(h.metadata.ID(), key)
	if err == nil {
		return res, nil
	}

	// On cache miss, we fetch from store and then try to update the cache
	// with the result before returning it.
	res, err = h.Store.Get(h.metadata.id, key)
	if err != nil {
		return nil, err
	}

	err = h.Cache.Put(h.metadata.id, key, res)
	if err != nil {
		log.Print(err) // log that caching failed.. TODO: build/plug-in a more powerful error logging system/service interface.
	}

	return res, nil
}

// Put will save a key/value pair in the session store.
func (h Handler) Put(key string, value []byte) error {

	err := h.Store.Put(h.metadata.ID(), key, value)
	if err != nil {
		return err
	}

	if h.Cache == nil {
		return nil
	}

	err = h.Cache.Put(h.metadata.ID(), key, value)
	if err != nil {
		log.Print(err) // Putting a value into the cache may not succeed. It's OK. Just log it.
	}
	return nil
}

// Delete will erase a session store item.
func (h Handler) Delete(key string) error {

	if h.Cache == nil {
		return h.Store.Delete(h.metadata.ID(), key)
	}

	err := h.Cache.Delete(h.metadata.ID(), key) // Attempt to delete a value from cache MUST succeed.
	if err != nil {
		log.Print(err)
		return err
	}

	err = h.Store.Delete(h.metadata.ID(), key)
	if err != nil {
		return err
	}
	return nil
}

// Expire allows to set the duration we should wait before considering
// the session stale.
// if t < 0, the session expires immediately.
// if t = 0, the session expires when the browser is closed.
// if t > 0, the session expires after t seconds.
func (h Handler) Expire(t int) error {
	err := h.Store.Expire(h.metadata.ID(), int64(t))
	if err != nil {
		return err
	}
	h.Params.MaxAge = t
	h.metadata.Update(true) // sentinel notifying that we should use generate to update session cookie.
	return nil
}

// ID returns the session id of a user.
func (h Handler) ID() string {
	return h.metadata.ID()
}

// Expiry retrieves the session Expiration date.
func (h Handler) Expiry() time.Time {
	return h.metadata.Expiry()
}

// metadata is a type for the information stored in a sessioncookie.
type metadata struct {
	ID        string
	Expiry    time.Time
	Value     string
	delimiter string
	update    bool
}

// Key is the value that is used to retrieve a saved session handler from
// the per-request context datastore.
var Key metadata

func newSessionData() metadata {
	return metadata{
		delimiter: ":", // should be sendable via cookie and not in base64 sigil list
		update:    true,
	}
}

func (session *metadata) ID() string {
	return session.ID
}

func (session *metadata) SetID(s string) {
	session.ID = s
	update = true
}

func (session *metadata) Expiry() time.Time {
	return session.Expiry
}

func (session *metadata) Expire(t time.Time) {
	session.Expiry = t
	update = true
}

func (session *metadata) Updated() bool {
	return session.update
}

func (session *metadata) Update(b bool) {
	session.update = b
}

func (token *metadata) Encode(secret string) string {
	j, err := json.Marshal(token)
	if err != nil {
		log.Panic("JSON encoding internal failure. Exceptional behaviour while encoding session metadata.")
	}
	return computeHmac256(j, []byte(secret)) + token.delimiter + base64.StdEncoding.EncodeToString(j)
}

func (token *metadata) Decode(metadata string, secret string) error {
	// let's split the two components on the string-marshalled metadata (raw + Encoded)
	s := strings.Split(secret, token.delimiter)
	if len(s) <= 1 || len(s) > 4096 {
		panic("A surprising error occured. Bad session token.")
	}

	ok, err := VerifySignature(s[1], s[0], secret)
	if !ok {
		log.Println("Sessioncookie seems to have been tampered with.")
		return ERRBADSESSION
	}
	str, err := base64.StdEncoding.DecodeString(s[1])
	if err != nil {
		return err
	}

	err = json.Unmarshal(str, token)
	if err != nil {
		log.Println("While Decoding metadata, JSON unmarshalling failed.")
		return err
	}
	return nil
}

// Load will try to recover the session handler state if it was previously
// handled. Otherwise, it will try loading the metadata directly from the request
// object if it exists. If none works, an error is returned.
func (h *Handler) Load(res http.ResponseWriter, req *http.Request, ctx context.Object) error {
	v, err := ctx.Get(Key)
	if err != nil {
		reqc, err := req.Cookie(h.Params.Name)
		if err != nil {
			return ERRNOID
		}
		h.Params = *reqc
		err = h.metadata.Decode(reqc.Value, h.Secret)
		if err != nil {
			// TODO session is invalid. Maybe it has been tamperedw with
			// log error and return invalid session error
			return ERRNOID
		}
		h.Save(res, req, ctx)
		return nil
	}
	oldsess, ok := v.(Handler)
	if !ok {
		panic("Something very odd happened. Session handler is of wrong type ?!.")
	}

	*h = oldsess
	h.Save(res, req, ctx)

	return nil
}

// Save will keep the session handler state in the per-request context store.
// It needs to be called to have changes to the way sessions are handled take effect.
func (h *Handler) Save(res http.ResponseWriter, req *http.Request, ctx context.Object) {

	if h.metadata.Updated() {
		h.generate(res, req, ctx)
	}
	ctx.Put(Key, h)
}

// Renew will regenerate the session with a brand new session id. todo review: needs reload
func (h *Handler) Renew(res http.ResponseWriter, req *http.Request, ctx context.Object) {
	h.generate(res, req, ctx)
	ctx.Put(Key, *h)
}

// generate creates a new session (new session cookie with renewed session metadata)
func (h *Handler) generate(res http.ResponseWriter, req *http.Request, ctx context.Object) {

	// 1. Create UUID
	var uUID string
	id := uuid.NewV4()
	uUID = id.String()

	// 2. Generate expiry date (in UTC) //TODO cookie.Expire for old browsers ?
	var expdate time.Time
	if h.Params.MaxAge > 0 {
		expdate = time.Now().Add(time.Duration(h.Params.MaxAge) * time.Second).UTC()
	}

	// 3. Update internal metadata object
	h.metadata.SetID(uUID)
	h.metadata.Expire(expdate)

	// 4. Sets new cookie and save new session.
	h.Params.Value = h.metadata.Encode(h.Secret)
	http.SetCookie(res, &(h.Params))
	h.metadata.Update(false)
	ctx.Put(Key, h)
}

// ServeHTTP effectively makes the session an http request handler middleware.
func (h Handler) ServeHTTP(res http.ResponseWriter, req *http.Request, ctx context.Object) (http.ResponseWriter, bool) {
	err := h.Load(res, req, ctx)

	if err != nil {
		http.Error(res, "Failed to load session.", 500)
		h.generate(res, req, ctx)
		return res, true
	}

	return res, false
}

// computeHmac256 returns a base64 Encoded MAC.
func computeHmac256(message, secret []byte) string {
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
