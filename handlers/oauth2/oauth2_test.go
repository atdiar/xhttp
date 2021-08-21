package xoauth2

import (
	"net/http/httptest"
	"testing"
  "database/sql"
  _ "github.com/go-sql-driver/mysql"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	oauth "google.golang.org/api/oauth2/v2"
)
var ctx context.Context
var sess session.Handler
var mux xhttp.ServeMux
var signer Authentifier
var callbackHandler CallbackHandler
var config *oauth2.Config
var db *sql.DB

func testInitMain() (deferred func()) {
	// context initialization
	ctx = context.Background()
  // session creation
	sess = session.New("basic_user_session", "sdgfsqdg56s5gq6ffg3")

  // multiplexer creation
	mux = xhttp.NewServeMux()

  // database creation
  db, err = sql.Open("mysql", "user:password@/dbname")
  if err != nil {
	   panic(err)
  }
  db.SetConnMaxLifetime(time.Minute * 3)
  db.SetMaxOpenConns(10)
  db.SetMaxIdleConns(10)


  // google authentication and signup configuration
  config = &oauth2.Config{
		ClientID:     "868368187570-i1s5oqkhta8kqt45s37136jgbjn67nqo.apps.googleusercontent.com",
		ClientSecret: "cPhhAnrz8wRvq9uzAR4vxFz8",
		Scopes:       []string{"openid", "profile","email"},
		Endpoint:     google.Endpoint,
	}
  stmt,err := db.PrepareContext(ctx, "INSERT INTO ")
  if err != nil{
    panic(err)
  }
  DBCreateUserFromGoogleInfo := func(userinfo interface{}) (sql.Result, error){
    iu,ok:= userinfo.(*oauth.Userinfoplus)
    if !ok{
      return nil, errors.New("could not retrieve information from google OAuth")
    }
    return stmt.ExecContext(ctx)
  }

  googleSignup:= usersigning.New(sess, signup.WithGoogle())

	signer = NewRequest(sess, config, Signup)

  return func(){
    Database.Close()
  }
}

func TestOAuth(t *testing.T) {
	deffered :=testInitMain()
  defer deffered()

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
}
