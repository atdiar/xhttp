// Package oauth2 is a wrapping package that derives a context.Context from
// an executiopn.Context
package oauth2

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/atdiar/errors"
	"github.com/atdiar/goroutine/execution"
	"golang.org/x/oauth2"
)

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
	State string // used to mitigate csrf attacks. Verified in callback handling.
}

// LoginHandler creates a new object that deals with user authentication for a
// given endpoint
func LoginHandler(c *oauth2.Config) LoginRequester {
	return LoginRequester{
		c, "",
	}
}
func (r LoginRequester) ServeHTTP(ctx execution.Context, w http.ResponseWriter, req *http.Request) {
	URL, err := url.Parse(r.Config.Endpoint.AuthURL)
	if err != nil {
		log.Fatal(errors.New(err.Error())) // TODO: see if it is the right thing to do
	}
	parameters := url.Values{}
	parameters.Add("client_id", r.Config.ClientID)
	parameters.Add("scope", strings.Join(r.Config.Scopes, " "))
	parameters.Add("redirect_uri", r.Config.RedirectURL)
	parameters.Add("response_type", "code")
	parameters.Add("state", r.State)
	URL.RawQuery = parameters.Encode()
	url := URL.String()
	http.Redirect(w, req, url, http.StatusTemporaryRedirect)
}

// Handler defines the type of objects that will apply the logic used to
// handle the response dispatched to the callback address after a authentication
// request.
type Handler struct {
	*oauth2.Config
	PrefixURL string                  //prefix of the URL that enables to retrieve scoped user data
	State     string                  // anti csrf
	Apply     func(interface{}) error // used to handle the token
}

func (h Handler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	state := r.FormValue("state")
	if state != h.State {
		fmt.Printf("invalid oauth state, expected '%s', got '%s'\n", h.State, state)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	code := r.FormValue("code")

	token, err := h.Config.Exchange(oauth2.NoContext, code)
	if err != nil {
		fmt.Printf("oauthConf.Exchange() failed with '%s'\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	resp, err := http.Get(h.PrefixURL +
		url.QueryEscape(token.AccessToken))
	if err != nil {
		fmt.Printf("Get: %s\n", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	defer resp.Body.Close()

	response, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("ReadAll: %s\n", err)
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

/*
package main

import (
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "net/url"
  "strings"

  "golang.org/x/oauth2"
  "golang.org/x/oauth2/facebook"
)

var (
  oauthConf = &oauth2.Config{
    ClientID:     "YOUR_CLIENT_ID",
    ClientSecret: "YOUR_CLIENT_SECRET",
    RedirectURL:  "YOUR_REDIRECT_URL_CALLBACK",
    Scopes:       []string{"public_profile"},
    Endpoint:     facebook.Endpoint,
  }
  oauthStateString = "thisshouldberandom"
)

const htmlIndex = `<html><body>
Logged in with <a href="/login">facebook</a>
</body></html>
`

func handleMain(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Content-Type", "text/html; charset=utf-8")
  w.WriteHeader(http.StatusOK)
  w.Write([]byte(htmlIndex))
}

func handleFacebookLogin(w http.ResponseWriter, r *http.Request) {
  Url, err := url.Parse(oauthConf.Endpoint.AuthURL)
  if err != nil {
    log.Fatal("Parse: ", err)
  }
  parameters := url.Values{}
  parameters.Add("client_id", oauthConf.ClientID)
  parameters.Add("scope", strings.Join(oauthConf.Scopes, " "))
  parameters.Add("redirect_uri", oauthConf.RedirectURL)
  parameters.Add("response_type", "code")
  parameters.Add("state", oauthStateString)
  Url.RawQuery = parameters.Encode()
  url := Url.String()
  http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleFacebookCallback(w http.ResponseWriter, r *http.Request) {
  state := r.FormValue("state")
  if state != oauthStateString {
    fmt.Printf("invalid oauth state, expected '%s', got '%s'\n", oauthStateString, state)
    http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
    return
  }

  code := r.FormValue("code")

  token, err := oauthConf.Exchange(oauth2.NoContext, code)
  if err != nil {
    fmt.Printf("oauthConf.Exchange() failed with '%s'\n", err)
    http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
    return
  }

  resp, err := http.Get("https://graph.facebook.com/me?access_token=" +
    url.QueryEscape(token.AccessToken))
  if err != nil {
    fmt.Printf("Get: %s\n", err)
    http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
    return
  }
  defer resp.Body.Close()

  response, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    fmt.Printf("ReadAll: %s\n", err)
    http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
    return
  }

  log.Printf("parseResponseBody: %s\n", string(response))

  http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func main() {
  http.HandleFunc("/", handleMain)
  http.HandleFunc("/login", handleFacebookLogin)
  http.HandleFunc("/oauth2callback", handleFacebookCallback)
  fmt.Print("Started running on http://localhost:9090\n")
  log.Fatal(http.ListenAndServe(":9090", nil))
}
*/
