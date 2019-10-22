package googleoauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp/handlers/session"
	"golang.org/x/oauth2"
)

var (
	TokenSessionKey = "oauthToken"
)

type LoginHandler struct {
	Session session.Handler
	*oauth2.Config
	AccessType    oauth2.AuthCodeOption
	ApprovalForce oauth2.AuthCodeOption
}

type CallbackHandler struct {
	Session session.Handler
	*oauth2.Config
	Context context.Context
}

func NewHandlers(s session.Handler, c *oauth2.Config, ctx context.Context) (LoginHandler, CallbackHandler) {
	return LoginHandler{s, c, nil, nil}, CallbackHandler{s, c, ctx}
}

func (l LoginHandler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	state, err := generateNonce(32)
	if err != nil {
		log.Printf("Error generating oauth state variable: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = l.Session.Put("oauthstate", ([]byte)(state))
	if err != nil {
		log.Printf("Error saving oauth state variable into session: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	url := l.Config.AuthCodeURL(state, l.AccessType, l.ApprovalForce)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func (c CallbackHandler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	rawstate, err := c.Session.Get("oauthstate")
	if err != nil {
		log.Printf("Error recovering oauth state variable: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	state := string(rawstate)
	if r.FormValue("state") != state {
		log.Print("Error : state variables are not equal")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	code := r.FormValue("code")
	tok, err := c.Config.Exchange(c.Context, code)
	if err != nil {
		log.Printf("Error while retrieving token: %v", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
	jtok, err := json.Marshal(*tok)
	if err != nil {
		log.Printf("Error marshalling oauth token: %v", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
	c.Session.Put(TokenSessionKey, jtok)
}

func TokenFromSession(s session.Handler) (oauth2.Token, error) {
	rawtoken, err := s.Get(TokenSessionKey)
	if err != nil {
		return oauth2.Token{}, errors.New("Could not retrieve token from Session.")
	}
	tok := &oauth2.Token{}
	err = json.Unmarshal(rawtoken, tok)
	return *tok, err
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

// LoginHandler creates a new object that deals with user authentication for a
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
func (r LoginRequester) ServeHTTP(ctx execution.Context, w http.ResponseWriter, req *http.Request) {
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

func (h CallbackHandler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
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
