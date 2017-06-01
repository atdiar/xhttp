package register

import (
	"net/http"

	"github.com/atdiar/errors"

	"github.com/atdiar/goroutine/execution"
)

// User is a structured type based of the data kept for user registration.
// It corresponds to a basic user schema for registration in the database and
// session cache.
type User struct {
	Username   string
	Password   string
	Email      string
	Persistent string
	save       func(interface{}, execution.Context, http.ResponseWriter, *http.Request) error
}

// New creates a user data hoding object with a user registration hook.
func New(save func(interface{}, execution.Context, http.ResponseWriter, *http.Request) error) User {
	u := User{}
	u.save = save
	return u
}

// Save registers a user, can write whether the operation succeeded to w.
func (u User) Save(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	if u.save != nil {
		err := u.save(u, ctx, w, r)
		if err != nil {
			panic(errors.New(err.Error()))
		}
	}
}

func (u User) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		panic(errors.New(err.Error()))
	}

	u.Username = r.Form.Get("username")
	u.Password = r.Form.Get("password")
	u.Email = r.Form.Get("email")
	u.Persistent = r.Form.Get("persistent")

	// Then we save this user in the database and what not.
	// The funcion is in charge of the sanitization of the data.
	// That function may panic but that will be caught up by a panic handler.
	u.Save(ctx, w, r)
}
