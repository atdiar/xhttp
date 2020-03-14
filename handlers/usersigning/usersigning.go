package usersigning

import (
	"context"
	"database/sql"
	"log"
	"net/http"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

// Handler defines a generic request Handler that can be configured with specific
// implementations in order to deal with signing-up or signing-in a user.
// The user information can be stored in a SQL database after being sourced from
// oAuth providers or a traditional email sign up form.
type Handler struct {
	Session session.Interface
	Handler xhttp.Handler
	next    xhttp.Handler

	Log *log.Logger
}

// PreparedStmt defines an alias for a  constructor of functions that execute
// a Prepared Statement to store user info into the database.
type PreparedStmt = func(*sql.Stmt) func(userinfo interface{}) (sql.Result, error)

// DBSQLCreateUserFunc is an alias for functions functions that execute a Prepared
// Statement to store user info into the database.
type DBSQLCreateUserFunc = func(userinfo interface{}) (sql.Result, error)

// New returns a request handler used for user signup. It is generic
// and as suc, ought to be configured according to each service provider via the
// second argument.
func New(s session.Interface, Configure func(s Handler) Handler) Handler {
	n := Handler{
		Session: s,
		Handler: nil,
		next:    nil,
		Log:     nil,
	}
	if Configure != nil {
		return Configure(n)
	}
	return n
}

// Configure is a method that accepts Configuration functions for the signup
// Handler.
func (s Handler) Configure(cs ...func(s Handler) Handler) Handler {
	for _, c := range cs {
		if c != nil {
			s = c(s)
		}
	}
	return sfr
}

func (s Handler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if s.Handler == nil {
		panic("Signup handler was not registered correctly.")
	}
	s.Handler.ServeHTTP(ctx, w, r)
	if s.next != nil {
		s.next.ServeHTTP(ctx, w, r)
	}
}

// Link enables the linking of a xhttp.Handler to the Signup Handler.
func (s Handler) Link(h xhttp.Handler) xhttp.HandlerLinker {
	s.next = h
	return s
}
