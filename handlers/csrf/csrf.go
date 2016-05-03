// Package Instance implements a stateless Cross-Site request forgery protection middleware
// It does not store the cookie anywhere into the DOM so it should not be susceptible to any BREACH attack
package Instance

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"github.com/iambase/gombo/middleware/session"
	"github.com/iambase/gombo/middleware/session/cookie"
	"github.com/iambase/gombo/router/context"
	"net/http"
)

var (
	INVALIDSESSION = errors.New("Session pointer is nil. Session does not exist ?")
)

type Instance struct {
	Session *session.Instance
	Name    string
	Header  string
	Cookie  cookie.Instance
	// Note : the CSRF cookie is not dynamically bound to the session Instance.
	// It should be updated by a call to the `Update()` method by the session Instance
	// on session change.
}

// CSRF Instance configuration logic

func New(s *session.Instance) *Instance {
	if s == nil {
		panic(INVALIDSESSION)
	}
	c := new(Instance)
	//Defaults
	c.Name = "XSRF-TOKEN"
	c.Header = "X-" + c.Name
	c.Session = s
	c.Cookie = s.Cookie

	return c
}

// Option represents the type of functions used to configure a Instance object.
type Option func(*Instance) error

// Configurator is an empty struct type whose methods return Instance configuration functions.
// Note that Instance.Session, Instance.Cookie and Instance.Header are never directly modified.
// Changes to Instance.Name propagate to the rest of the Instance object.

type Configurator struct {
	ChangeCookie cookie.Configurator
}

func (o Configurator) Name(str string) func(*Instance) error {
	return func(c *Instance) error {
		c.Name = str
		c.Header = "X-" + c.Name
		if c.Cookie == nil {
			c.Cookie, _ = cookie.New(cookie.Options.Name(c.Name))
		}
		c.Cookie.Configure(cookie.Options.Name(c.Name))
		return nil
	}
}

// Cookie is likely to be used very seldomly. In some edge cases such as changing the behaviour of CSRF Expiry
func (o Configurator) Cookie(options ...cookie.Option) func(*Instance) error {
	return func(cf *Instance) error {
		if cf.Cookie != nil {
			cf.Cookie.Configure(options...)
		} else {
			c, _ := cookie.New()
			c.Configure(options...)
			cf.Cookie = c
		}
		return nil
	}
}

// Instance API (it is exported for the cases when granularity is required/convenient)

func (i *Instance) generateToken(res http.ResponseWriter, req *http.Request, data *context.Instance) (err error) {
	tok, err := GenerateToken(32)
	if err != nil {
		http.Error(res, "Generating CSRF Token failed", 503)
		return err
	}
	i.Cookie.Configure(cookie.Options.Value(tok), cookie.Options.MaxAge(0))
	rcookie := i.Session.GetRaw().Cookie.GetRaw()

	d, err := data.Store.Get(rcookie.Name)
	if err != nil {
		http.Error(res, "Generating CSRF Token failed", 503)
		return err
	}
	sessioncookiename := d.(string)

	if err = i.Session.Put(sessioncookiename, i.Cookie.GetRaw().Name, i.Cookie.GetRaw().Value); err != nil {
		http.Error(res, "Storing new CSRF Token in session failed", 503)
		return err
	}

	http.SetCookie(res, i.Cookie.GetRaw())
	return err
}

// Update is used to refresh Instance cookie information that should follow the information of the session cookie.
func (i *Instance) Update() {
	// Refreshing Cookie in case Session cookie changed - hopefully this verification does not consume too many cycles.
	sckie := i.Session.GetRaw().Cookie.GetRaw()
	i.Cookie.Configure(cookie.Options.Path(sckie.Path), cookie.Options.MaxAge(sckie.MaxAge), cookie.Options.Domain(sckie.Domain), cookie.Options.Secure(sckie.Secure))
}

func (i *Instance) ServeHTTP(res http.ResponseWriter, req *http.Request, data *context.Instance) (http.ResponseWriter, bool) {
	// Case 1: no CSRF Header set on the request
	//
	// If CSRF cookie does NOT exist, then we should create one (All methods)
	// If CSRF cookie DOES exist, invalidate for PUT/POST/DELETE and reset value.
	// For GET, look if CSRF token value in store is still the same : if it is not in store, delete cookie
	// Don't do anything if value is the same / reset token if it is in store but the value is different
	if Header, ok := req.Header[i.Header]; !ok {
		c, err := req.Cookie(i.Name)
		if c == nil || err != nil {
			err = i.generateToken(res, req, data)
			if err != nil {
				// Server was unable to generate CSRF token
				return res, true
			}
			if req.Method == "GET" {
				return res, false
			}
			return res, true
		}
		if req.Method == "GET" {
			var sessionid string
			if v, err := data.Store.Get(i.Session.GetRaw().Cookie.GetRaw().Name); err == nil {
				sessionid = v.(string)
			} else {
				http.Error(res, "Forbidden. CSRF Token missing or invalid", 403) //needed here
				return res, true
			}

			storedCSRF, err := i.Session.Get(sessionid, i.Name)
			switch t := storedCSRF.(type) { // TODO Once looking over the types set in the session and back, will need to clear things up here
			case []byte:
				storedCSRF = string(t)
			case string:
				storedCSRF = string(t)
			default:
				break
			}
			if err != nil { // session deleted || not persisted || session id changed (session reset) || xsrf cookie Name changed
				delCook := i.Cookie.GetRaw()
				delCook.MaxAge = -1
				http.SetCookie(res, delCook)
			}
			if storedCSRF != c.Value {
				err = i.generateToken(res, req, data)
				if err != nil {
					return res, true
				}
			}
			return res, false
		}
		err = i.generateToken(res, req, data)
		if err != nil {
			return res, true
		}
		http.Error(res, "Forbidden. CSRF Token missing or invalid", 403) //needed here
		return res, true

	} else {

		// Case 2: CSRF Header has been set for the request
		//
		// If GET method : Ignore even though it is a bit unexpected
		// Other Methods: match with value in Session. Invalidate if not equal and reset. If matching, reset and continue to next handler.
		if req.Method == "GET" {
			http.Error(res, "Warning. CSRF token sent with GET request. Undesirable behaviour", 403)
			return res, false
		}

		rawsessioncookiename, err := data.Store.Get(i.Session.GetRaw().Cookie.GetRaw().Name)
		if err != nil {
			http.Error(res, "Session ID invalid", 403)
			return res, true
		}
		sessioncookiename := rawsessioncookiename.(string)

		tokenraw, err := i.Session.Get(sessioncookiename, i.Name)
		if err != nil {
			err = i.generateToken(res, req, data)
			if err != nil {
				return res, true
			}
			return res, true
		}

		token := tokenraw.(string) //TODO might not need this assertion once I finish the session middleware so that it takes only string (w/JSON string)
		if Header[0] == token {
			err = i.generateToken(res, req, data)
			if err != nil {
				return res, true
			}
			return res, false
		} else {
			err = i.generateToken(res, req, data)
			if err != nil {
				return res, true
			}
			http.Error(res, "CSRF token missing or invalid", 403)
			return res, true
		}
	}
}

// GenerateToken creates a base64 encoded version of a 32byte Cryptographically secure random number to be used as a protection against CSRF attacks.
// It uses Go's implementation of devurandom (which has a backup in case devurandom is inaccessible)
func GenerateToken(length int) (string, error) {
	bstr := make([]byte, length)
	_, err := rand.Read(bstr)
	if err != nil {
		return "", err
	} else {
		str := base64.StdEncoding.EncodeToString(bstr)
		return str, nil
	}
}
