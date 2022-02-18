package xoauth2

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"time"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
	"golang.org/x/oauth2"
)

var (
	// TokenKey is the key under which an oAuth Token is stored in a context
	TokenKey tokenkey
)

type tokenkey struct{}

// Authentifier defines a http request handler that will initiate the oAuth request.
type Authentifier struct {
	Session session.Handler
	*oauth2.Config
	Options []oauth2.AuthCodeOption
	Log     *log.Logger
}

// CallbackHandler defines a http request handler that will deal with the
// finalization of the oAuth request by saving the authorization token in the
// session store and the context object and executing either user Authentication
// (aka user signin) or user Registration (aka user signup).
type CallbackHandler struct {
	authentifier *Authentifier
	next         xhttp.Handler
}

// NewRequest returns a new user Authentifier object that handles a http request
// for user authentication.
func NewRequest(s session.Handler, c *oauth2.Config) (Authentifier, CallbackHandler) {
	auth := Authentifier{s, c, nil, nil}
	return auth, CallbackHandler{&auth, nil}
}

// AuthCodeOptions allows to add some options that will parameterize the login request.
// By default, nothing is passed which means that no refresh token is requested.
func (l Authentifier) AuthCodeOptions(opt ...oauth2.AuthCodeOption) Authentifier {
	l.Options = opt
	return l
}

// ServeHTTP handles the request.
func (l Authentifier) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// !. Check if an authentification session has already been created.

	state, err := generateNonce(32)
	if err != nil {
		if l.Log != nil {
			l.Log.Printf("Error generating oauth state variable: %v", err)
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = l.Session.Put(r.Context(), "oauthstate", ([]byte)(state), 10*time.Minute)
	if err != nil {
		if l.Log != nil {
			l.Log.Printf("Error saving oauth state variable into session: %v", err)
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	url := l.Config.AuthCodeURL(state, l.Options...)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// ServeHTTP handles the request.
func (c CallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx:= r.Context()
	rawstate, err := c.authentifier.Session.Get(ctx,"oauthstate")
	if err != nil {
		if c.authentifier.Log != nil {
			c.authentifier.Log.Printf("Error recovering oauth state variable: %v", err)
		}
		http.Error(w, "XOAUTH2:unable to recover authentication state", http.StatusInternalServerError)
		return
	}
	c.authentifier.Session.Delete(ctx, "oauthstate")
	state := string(rawstate)
	if r.FormValue("state") != state {
		if c.authentifier.Log != nil {
			c.authentifier.Log.Print("Error : state variables are not equal")
		}
		http.Error(w, "XOAUTH2:bad state", http.StatusInternalServerError)
		return
	}

	code := r.FormValue("code")
	tok, err := c.authentifier.Config.Exchange(ctx, code)
	if err != nil {
		if c.authentifier.Log != nil {
			c.authentifier.Log.Printf("Error while retrieving token: %v", err)
		}
		http.Error(w, "XOAUTH2:unable to complete authentication. Token missing.", http.StatusInternalServerError)
		return
	}
	// Put token and http.Client into context object
	ctx = context.WithValue(ctx, TokenKey, tok)
	ctx = context.WithValue(ctx, oauth2.HTTPClient, c.authentifier.Config.Client(ctx, tok))
	r=r.WithContext(ctx)

	if c.next != nil {
		c.next.ServeHTTP(w, r)
	}
}

// Link enables the linking of a xhttp.Handler to the CallbackHandler.
func (c CallbackHandler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	c.next = hn
	return c
}

// generateNonce creates a base64 encoded version of a 32byte Cryptographically
// secure random number to be used as a protection against CSRF attacks.
// It uses Go's implementation of devurandom (which has a backup in case
// devurandom is inaccessible)
func generateNonce(length int) (string, error) {
	bstr := make([]byte, length)
	_, err := rand.Read(bstr)
	if err != nil {
		return "", err
	}
	str := base64.StdEncoding.EncodeToString(bstr)
	return str, nil
}
