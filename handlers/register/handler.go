package register

import (
	"fmt"
	"net/http"

	"github.com/atdiar/errors"

	"github.com/atdiar/goroutine/execution"
)

// Register is a structured type based of the data kept for user registration.
// It corresponds to a basic user schema for registration in the database and
// session cache.
type Register struct {
	Username   string
	Password   string
	Email      string
	Persistent string
	save       func(interface{}) error
}

func (rr Register) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		panic(errors.New(err.Error()))
	}

	user := Register{
		Username:   r.Form.Get("username"),
		Password:   r.Form.Get("password"),
		Email:      r.Form.Get("email"),
		Persistent: r.Form.Get("persistent"),
	}

	// Then we save this user in the database and what not.
	// The funcion is in charge of the sanitization of the data.
	if rr.save != nil {
		err := rr.save(user)
		if err != nil {
			panic(errors.New(err.Error()))
		}
	}

	fmt.Fprint(w, "Registration successful. An email was sent for confirmation.")
}
