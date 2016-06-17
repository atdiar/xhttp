// Package csrf implements a request handler which generates a token to protect
// against Cross-Site Request Forgery.
package csrf

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

const (
	methodGET     = "GET"
	methodHEAD    = "HEAD"
	methodOPTIONS = "OPTIONS"
	invalidToken  = "Forbidden. anti-CSRF Token missing or invalid"
)

var (
	errInvalidSession = errors.New("Session pointer is nil. Session does not exist ?")
)

// Handler is a special type of request handler that creates a token value used
// to protect against Cross-Site Request Forgery vulnerabilities.
type Handler struct {
	Cookie  http.Cookie // anti-csrf cookie sent to client.
	Session session.Handler
	strict  bool // if true, a request is only valid if the xsrf Header is present.
	next    xhttp.Handler
}

// New builds a new anti-CSRF request handler.
// Because this token should be saved as a session value and matched against in
// order to  check the validity of a request, a fully parameterized session
// Handler object should be passed as argument.
// By default, the session cookie, holding the anti-CSRF value, is named
// "ANTICSRF"
func New(s session.Handler) Handler {
	h := Handler{}
	s = s.DisableCaching()
	h.Session = s
	h.Cookie = h.Session.Cookie
	h.strict = true

	h.Cookie.Name = "ANTICSRF"
	h.Cookie.HttpOnly = false
	h.Cookie.MaxAge = 0

	return h
}

// Link enables the linking of a xhttp.Handler to the anti-CSRF request Handler.
func (h Handler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	h.next = hn
	return h
}

// LaxMode disables the hard requirement for an anti-CSRF specific Header
// to be present in the client request.
// By default, it is true, meaning that if no header is found, the request
// handling cannot proceed further.
func (h Handler) LaxMode() Handler {
	h.strict = true
	return h
}

func (h Handler) generateToken(ctx execution.Context, res http.ResponseWriter, req *http.Request) (err error) {
	tok, err := generateToken(32)
	if err != nil {
		http.Error(res, "Generating anti-CSRF Token failed", 503)
		return err
	}
	// First we replace the session cookie by the anti-CSRF cookie
	// That will ensure that on Session Save, the anti-CSRF is registered in the
	// http response header.
	h.Session.Cookie = h.Cookie
	h.Session.Data.StoreValue(tok)

	if err = h.Session.Put(h.Cookie.Name, ([]byte)(tok)); err != nil {
		http.Error(res, "Storing new CSRF Token in session failed", 503)
		return err
	}
	h.Session.Save(ctx, res, req)
	return err
}

// TokenFromCtx tries to retrieve the anti-CSRF token value from the context
// datastore.
func (h Handler) TokenFromCtx(ctx execution.Context) (string, error) {
	h.Session.Cookie = h.Cookie
	v, err := h.Session.DataFromCtx(ctx)
	if err != nil {
		return "", err
	}
	return v.RetrieveValue(), nil
}

// ServeHTTP handles the servicing of incoming http requests.
func (h Handler) ServeHTTP(ctx execution.Context, res http.ResponseWriter, req *http.Request) {
	// First we have to load the session data.
	// Indeed, we want to register the CSRF token as a session value.
	// For this, we need to use the most recently generated session id.
	err := h.Session.Load(ctx, res, req)
	if err != nil {
		// the order in which the session and anti-csrf request handlers have been
		// called is probably incorrect.
		panic("Session not found: Cannot register CSRF token as a session value.")
	}
	// We replace the session cookie by the anticsrf cookie.
	// That will ensure that on Session Save, the anti-csrf is added to the
	// http response header.
	h.Session.Cookie = h.Cookie

	switch req.Method {
	case methodGET, methodHEAD, methodOPTIONS:
		// No CSRF check is performed for these methods.
		// However, an anti-CSRF token is generated and sent with the response
		// iff none has been generated yet.
		err = h.Session.Load(ctx, res, req)
		if err != nil {
			err = h.generateToken(ctx, res, req)
			if err != nil {
				http.Error(res, "Internal Server Error", 500)
				return
			}
		}
		if h.next != nil {
			h.next.ServeHTTP(ctx, res, req)
		}
		return

	default:
		Header, ok := req.Header[h.Cookie.Name]
		if !ok {
			if h.strict {
				err = h.generateToken(ctx, res, req)
				if err != nil {
					http.Error(res, "Internal Server Error", 500)
					return
				}
				http.Error(res, invalidToken, 403)
				return
			}
			err = h.Session.Load(ctx, res, req)
			if err != nil {
				err = h.generateToken(ctx, res, req)
				if err != nil {
					http.Error(res, "Internal Server Error", 500)
					return
				}
				http.Error(res, invalidToken, 403)
				return
			}
			// Validation
			tokenReceived := h.Session.Data.RetrieveValue()
			rawTokenInSession, err := h.Session.Get(h.Session.Cookie.Name)
			if err != nil {
				err = h.generateToken(ctx, res, req)
				if err != nil {
					http.Error(res, "Internal Server Error", 500)
					return
				}
				http.Error(res, invalidToken, 403)
				return
			}
			if tokenReceived != string(rawTokenInSession) {
				err = h.generateToken(ctx, res, req)
				if err != nil {
					http.Error(res, "Internal Server Error", 500)
					return
				}
				http.Error(res, invalidToken, 403)
				return
			}
			if h.next != nil {
				h.next.ServeHTTP(ctx, res, req)
			}
			return
		}
		// Header exists. The anti-csrf cookie must be present too.
		err = h.Session.Load(ctx, res, req)
		if err != nil {
			err = h.generateToken(ctx, res, req)
			if err != nil {
				http.Error(res, "Internal Server Error", 500)
				return
			}
			http.Error(res, invalidToken, 403)
			return
		}

		tokenReceived := Header[0]
		tokenFromCookie := h.Session.Data.RetrieveValue()
		rawTokenInSession, err := h.Session.Get(h.Session.Cookie.Name)
		if err != nil {
			err = h.generateToken(ctx, res, req)
			if err != nil {
				http.Error(res, "Internal Server Error", 500)
				return
			}
			http.Error(res, invalidToken, 403)
			return
		}
		if t := string(rawTokenInSession); tokenReceived != t || tokenFromCookie != t {
			err = h.generateToken(ctx, res, req)
			if err != nil {
				http.Error(res, "Internal Server Error", 500)
				return
			}
			http.Error(res, invalidToken, 403)
			return
		}
		if h.next != nil {
			h.next.ServeHTTP(ctx, res, req)
		}
		return
	}
}

// generateToken creates a base64 encoded version of a 32byte Cryptographically
// secure random number to be used as a protection against CSRF attacks.
// It uses Go's implementation of devurandom (which has a backup in case
// devurandom is inaccessible)
func generateToken(length int) (string, error) {
	bstr := make([]byte, length)
	_, err := rand.Read(bstr)
	if err != nil {
		return "", err
	}
	str := base64.StdEncoding.EncodeToString(bstr)
	return str, nil
}
