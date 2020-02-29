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
	"github.com/atdiar/xhttp/handlers/usersigning"
	"golang.org/x/oauth2"
)

var (
	// Signin is the parameter value used to start an Authentication session for
	// user authentication.
	Signin = func() AuthReason { return "signin" }()

	// Signup is the parameter value used to start an Authentification session for
	// user registration.
	Signup = func() AuthReason { return "signup" }()

	// TokenKey is the key under which an oAuth Token is stored in a context
	TokenKey tokenkey
)

// AuthReason defines an option type used to declare whether the authorization
// request is for user registration (signup) or user authentication (signin).
type AuthReason string

type tokenkey struct{}

// Authentifier defines a http request handler that will initiate the oAuth request.
type Authentifier struct {
	Session session.Handler
	*oauth2.Config
	Options []oauth2.AuthCodeOption
	Log     *log.Logger
	whatfor AuthReason
}

// CallbackHandler defines a http request handler that will deal with the
// finalization of the oAuth request by saving the authorization token in the
// session store and the context object and executing either user Authentication
// (aka user signin) or user Registration (aka user signup).
type CallbackHandler struct {
	authentifier *Authentifier
	signin       usersigning.Handler
	signup       usersigning.Handler
	next         xhttp.Handler
}

// NewRequest returns a new user Authentifier object that handles a http request
// for user authentication.
func NewRequest(s session.Handler, c *oauth2.Config, reason AuthReason) Authentifier {
	return Authentifier{s, c, nil, nil, reason}
}

// WithOptions allows to add some options for the handling of Login.
// For further information about these options, please refer to the oAuth2 package.
func (l Authentifier) WithOptions(opt ...oauth2.AuthCodeOption) Authentifier {
	l.Options = opt
	return l
}

// ServeHTTP handles the request.
func (l Authentifier) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// !. Check if an authentification session has already been created.

	state, err := generateNonce(32)
	if err != nil {
		if l.Log != nil {
			l.Log.Printf("Error generating oauth state variable: %v", err)
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = l.Session.Put("oauthstate", ([]byte)(state), 10*time.Minute)
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

// Callback is a method of the Authentifier that returns a handler for the
// callback route where a user is reirected once the oAuth login phase is done.
func (l Authentifier) Callback(signin usersigning.Handler, signup usersigning.Handler) CallbackHandler {
	return CallbackHandler{&l, signin, signup, nil}
}

// ServeHTTP handles the request.
func (c CallbackHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	rawstate, err := c.authentifier.Session.Get("oauthstate")
	if err != nil {
		if c.authentifier.Log != nil {
			c.authentifier.Log.Printf("Error recovering oauth state variable: %v", err)
		}
		http.Error(w, "XOAUTH2:unable to recover authentication state", http.StatusInternalServerError)
		return
	}
	c.authentifier.Session.Delete("oauthstate")
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

	// TODO:  --------------------------------

	// 2. INSERT SIGNUP/SIGNING LOGIC
	switch c.authentifier.whatfor {
	case Signin:
		c.signin.ServeHTTP(ctx, w, r)
	case Signup:
		c.signup.ServeHTTP(ctx, w, r)
	}
	// TODO:  --------------------------------

	if c.next != nil {
		c.next.ServeHTTP(ctx, w, r)
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

/*
// What is needed: a random password to protect against csrf attacks on the
// authentication server and a oauth2.Config object that holds the necessary
// information to be sent to the login server of choice. (endpoint)
//
// The random csrf password will be verified during the callback handling.
// The callback address is registered in the app configuration.

// LoginRequester defines the type of oauth2 authentication-enabling objects.
// These objects holds the configuration options that describes the oauth2
// endpoint and the data that can be retrieved from a successful authentication
// (scopes such as email, public profile etc.).
type LoginRequester struct {
	*oauth2.Config
	AccessType    oauth2.AuthCodeOption
	ApprovalForce oauth2.AuthCodeOption
	State         string // used to mitigate csrf attacks. Verified in callback handling.
}

// Authentifier creates a new object that deals with user authentication for a
// given endpoint
// If the http client argument is nil, the default http client will be used.
func Login(c *oauth2.Config, client *http.Client, AccessType oauth2.AuthCodeOption, ApprovalForce oauth2.AuthCodeOption) (LoginRequester, CallbackHandler) {
	t, err := generateNonce(32)
	if err != nil {
		panic(err)
	}
	l := LoginRequester{
		c, AccessType, ApprovalForce, t,
	}
	ctx := context.Background()
	if client != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, client)
	}
	h := CallbackHandler{c, ctx, t, "", nil}
	return l, h
}
func (r LoginRequester) ServeHTTP(ctx context.Context, w http.ResponseWriter, req *http.Request) {
	url := r.Config.AuthCodeURL(r.State, r.AccessType, r.ApprovalForce)
	http.Redirect(w, req, url, http.StatusTemporaryRedirect)
}

// Handler defines the callback handler to an authentication request.
type CallbackHandler struct {
	*oauth2.Config
	Context   context.Context
	State     string             // anti csrf
	PrefixURL string             //prefix of the URL that enables to retrieve scoped user data
	Apply     func([]byte) error // used to handle the response
}

func (h CallbackHandler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")
	if state != h.State {
		fmt.Printf("invalid oauth state, expected '%s', got '%s'\n", h.State, state)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	code := r.FormValue("code")

	token, err := h.Config.Exchange(h.Context, code)
	if err != nil {
		fmt.Printf("oauthConf.Exchange() failed with '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	resp, err := http.Get(h.PrefixURL +
		url.QueryEscape(token.AccessToken))
	if err != nil {
		log.Printf("Get: %s\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	defer resp.Body.Close()

	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("ReadAll: %s\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	if h.Apply != nil { // do something with the response
		err := h.Apply(response)
		if err != nil {
			log.Panic(errors.New(err.Error()))
		}
	}
	log.Printf("parseResponseBody: %s\n", string(response))

	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
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
*/
