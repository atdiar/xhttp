package csrf

import (
	"github.com/iambase/gombo/middleware/session"
	"testing"
)

func TestConfigure(t *testing.T) {
	Session, err := session.New()
	if err != nil {
		t.Fatal(err.Error())
	}
	CSRF, err := New(Session)
	if err != nil {
		t.Fatal(err.Error())
	}

	err = CSRF.Configure(Options.Cookie(
		Options.ChangeCookie.Name("test")))

	if err != nil {
		t.Fatal(err.Error())
	}

	if CSRF.csrf.Cookie.Name != "test" {
		t.Errorf("Got %s but wanted %s", CSRF.csrf.Cookie.Name, "test")
	}
}
