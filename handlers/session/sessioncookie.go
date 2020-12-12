package session

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/atdiar/errcode"
	"github.com/atdiar/errors"
	"github.com/atdiar/flag"
)

// CookieValue defines the structure of the data stored in cookie based sessions.
type CookieValue struct {
	Value  string     `json:"V"`
	Expiry *time.Time `json:"X,omitempty"`
}

// NewCookieValue formats a new value ready for storage in the session cookie.
func NewCookieValue(val string, maxage time.Duration, options ...func(CookieValue) CookieValue) CookieValue {
	n := time.Now().UTC()
	var c CookieValue
	if maxage == 0 {
		c = CookieValue{val, nil}
	} else {
		n = n.Add(maxage)
		c = CookieValue{val, &n}
	}
	if options != nil {
		for _, opt := range options {
			c = opt(c)
		}
	}
	return c
}

// Expired returns the expiration status of  a given value.
func (c CookieValue) Expired() bool {
	if c.Expiry == nil {
		return false
	}
	return time.Now().After(*(c.Expiry))
}

func (c CookieValue) tryRetrieve() (string, bool) {
	if !c.Expired() {
		return c.Value, true
	}
	return "", false
}

// AddTimeLimit allows to set an additional time limit to a cookie value.
// An example of such use case is when we want the value to exist only for the
// remaining duration of a session.
func AddTimeLimit(t time.Time) func(CookieValue) CookieValue {
	nt := t.UTC()
	return func(c CookieValue) CookieValue {
		if c.Expiry != nil {
			if nt.Before(*(c.Expiry)) {
				c.Expiry = &nt
			}
			return c
		}
		c.Expiry = &nt
		return c
	}
}

// Cookie defines the structure of a cookie based session object that can be
// used to persist session data between a client and the server.
type Cookie struct {
	HttpCookie *http.Cookie
	Data       map[string]CookieValue
	ApplyMods  *flag.Flag

	Secret string
	// the delimiter should be sendable via cookie.
	// It can't belong to the base64 list of accepted sigils.
	// It is used to separate the session cookie secret from the payload.
	Delimiter string
}

// NewCookie creates a new cookie based session object.
func NewCookie(name string, secret string, maxage int, options ...func(Cookie) Cookie) Cookie {
	if name == "" {
		panic("Session cookie name cannpt be the empty string.")
	}

	if secret == "" {
		panic("Session cookie secret cannpt be the empty string.")
	}

	s := Cookie{
		HttpCookie: &http.Cookie{},
		Data:       make(map[string]CookieValue),
		ApplyMods:  &flag.Flag{},
		Secret:     secret,
		Delimiter:  ":",
	}
	s.HttpCookie.Name = name
	s.HttpCookie.MaxAge = maxage

	s = DefaultCookieConfig(s)

	if options != nil {
		for _, opt := range options {
			s = opt(s)
		}
	}

	_, ok := s.ID()
	if !ok {
		panic("ERR: id is a reserved key for the storage of the session id. Do not erase it.")
	}
	s.Touch()
	return s
}

// DefaultCookieConfig is used to configure a session Cookie underlying
// http.Cookie with sane default values.
// The cookie parameters are set to ;
// * HttpOnly: true
// * Path:"/"
// * Secure: true
func DefaultCookieConfig(s Cookie) Cookie {
	s.HttpCookie.HttpOnly = true
	s.HttpCookie.Secure = true
	s.HttpCookie.Path = "/"
	return s
}

// ID returns the session id if it has not expired.
func (c Cookie) ID() (string, bool) {
	return c.Data["id"].Value, true
}

// SetID is a setter for the session id in the cookie based session.
func (c Cookie) SetID(id string) {
	c.Data["id"] = NewCookieValue(id, 0)
	c.ApplyMods.Set(true)
}

// Get retrieves the value stored in the cookie session corresponding to the
// given key, if it exists/has not expired.
func (c Cookie) Get(key string) (string, bool) {
	cval, ok := c.Data[key]
	if !ok {
		return "", false
	}
	if cval.Expired() {
		delete(c.Data, key)
		c.ApplyMods.Set(true)
		return "", false
	}
	return c.Data[key].tryRetrieve()
}

// Set inserts a value in the cookie session for a given key.
// Do not use "id" as a key. It has been reserved by the library.
func (c Cookie) Set(key string, val string, maxage time.Duration) {
	if key == "id" {
		panic("ERR: cannot used 'id' as key.")
	}
	switch {
	case maxage > 0:
		c.Data[key] = NewCookieValue(val, time.Duration(c.HttpCookie.MaxAge), AddTimeLimit(time.Now().UTC().Add(maxage)))
		c.ApplyMods.Set(true)
		return
	case maxage == 0:
		c.Data[key] = NewCookieValue(val, 0)
		c.ApplyMods.Set(true)
		return
	case maxage < 0:
		if _, ok := c.Data[key]; ok {
			delete(c.Data, key)
			c.ApplyMods.Set(true)
			return
		}
	}
}

// Delete will remove the value stored in the cookie session for the given key
// if it exsts.
func (c Cookie) Delete(key string) {
	delete(c.Data, key)
	c.ApplyMods.Set(true)
}

func (c Cookie) TimeToExpiry(key string) (time.Duration, error) {
	val, ok := c.Data[key]
	if !ok {
		return 0, errors.New("no value stored for key: " + key)
	}
	if val.Expired() {
		delete(c.Data, key)
		c.ApplyMods.Set(true)
		return 0, errors.New("no value stored for key: " + key)
	}
	return val.Expiry.Sub(time.Now().UTC()), nil
}

// Erase deletes the session cookies sharing the session name
func (c Cookie) Erase(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	cookieslice := r.Cookies()
	for _, cookie := range cookieslice {
		if cookie.Name == c.HttpCookie.Name {
			cookie.MaxAge = -1
			http.SetCookie(w, cookie)
		}
	}
}

// Expire will allow to send a signal to the client browser to delete the
// session cookie as the session is now expired.
// At the next request, the client may be issued a new session id.
func (c Cookie) Expire() {
	c.Data["id"] = NewCookieValue("", time.Duration(c.HttpCookie.MaxAge), AddTimeLimit(time.Now()))
	c.HttpCookie.MaxAge = -1
	c.Set(sessionValidityKey, "false", time.Duration(c.HttpCookie.MaxAge))
}

// Touch sets a new maxage for the session cookie and updates the expiry date of
// every non-expired items stored in the session cookie (if provided)
// Otherwise, it just resets the session duration using the previous session
// cookie maxage value.
func (c Cookie) Touch() {
	c.Set(sessionValidityKey, "true", time.Duration(c.HttpCookie.MaxAge))
}

// Encode will return a session cookie holding the json serialized session data.
func (c Cookie) Encode() (http.Cookie, error) {
	jval, err := json.Marshal(c.Data)
	if err != nil {
		return http.Cookie{}, errors.New("Encoding failure for session cookie.").Wraps(err)
	}
	v := ComputeHmac256(jval, []byte(c.Secret)) + c.Delimiter + base64.StdEncoding.EncodeToString(jval)

	c.HttpCookie.Value = v
	if len(c.HttpCookie.String()) > 4096 {
		return http.Cookie{}, errors.New("ERR: JSON encoded value too big for cookie. Max allowed length is 4kB i.e. 4096 bytes")
	}
	c.ApplyMods.Set(true)

	return *(c.HttpCookie), nil
}

// Decode is used to deserialize the session cookie in order to make the stored
// session data accessible.
// If we detect that the client has tampered with the session cookie somehow,
// an error is returned.
func (c Cookie) Decode(h http.Cookie) error {
	// let's split the two components on the string-marshalled metadata (raw + Encoded)
	s := strings.Split(h.Value, c.Delimiter)
	if len(s) <= 1 || len(s) > 4000 {
		return ErrBadCookie.Wraps(errors.New("Cookie seems to have been tampered with. Size too large"))
	}
	b64Message := s[1]
	b64MAC := s[0]
	ok, err := VerifySignature(b64Message, b64MAC, c.Secret)
	if !ok {
		e := errors.New("Signature verification failure of session cookie")
		if err != nil {
			return e.Wraps(err)
		}
		return e
	}
	str, err := base64.StdEncoding.DecodeString(b64Message)
	if err != nil {
		log.Print("Decoding error")
		return errors.New("Decoding failure").Wraps(err).Code(errcode.BadCookie)
	}
	err = json.Unmarshal(str, &(c.Data))
	if err != nil {
		return errors.New("Unmarshalling failure of session value").Wraps(err).Code(errcode.BadCookie)
	}
	return nil
}
