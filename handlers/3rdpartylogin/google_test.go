package login

import(
  "net/http"
  "testing"
  "context"
  "github.com/atdiar/xhttp/handlers/oauth2"
  "net/http/httptest"
  "github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func TestOAuth(t *testing.T) {
  sess := session.New("basic_user_session", "sdgfsqdg56s5gq6ffg3")
  // multiplexer creation
  mux := xhttp.NewServeMux()

  login:= WithGoogle(sess,"/","/signup",func(ctx context.Context, provideruserinfo map[string]interface{}) (dbuserinfo map[string]string, err error){
    return nil,nil // this is an example, the importantthing here is that err is nil.
  })

  config := &oauth2.Config{
		ClientID:     "868368187570-i1s5oqkhta8kqt45s37136jgbjn67nqo.apps.googleusercontent.com",
		ClientSecret: "cPhhAnrz8wRvq9uzAR4vxFz8",
		Scopes:       []string{"openid", "profile","email"},
		Endpoint:     google.Endpoint,
	}

  auth,cb:= xoauth2.NewRequest(sess,config)

  mux.GET("/auth/google",auth)
  mux.GET("/callback",cb.Link(login))

  req, err := http.NewRequest("GET", "http://example.com/auth/google", nil)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
}
