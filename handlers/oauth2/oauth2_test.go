package xoauth2

import (
	"net/http/httptest"
	"testing"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var sess session.Handler
var mux xhttp.ServeMux
var signer Authentifier
var callbackHandler CallbackHandler
var config *oauth2.Config

func testInit() {
	sess = session.New("basic_user_session", "sdgfsqdg56s5gq6ffg3")
	mux = xhttp.NewServeMux()
	config = &oauth2.Config{
		ClientID:     "868368187570-i1s5oqkhta8kqt45s37136jgbjn67nqo.apps.googleusercontent.com",
		ClientSecret: "cPhhAnrz8wRvq9uzAR4vxFz8",
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}

	signer = NewRequest(session, config, Signin)
}

func TestOAuth(t *testing.T) {
	testInit()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
}
