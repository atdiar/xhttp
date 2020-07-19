package rbac

import (
	"context"
	"encoding/json"
	//"log"
	"net/http"
	"net/http/httptest"
	//	"strconv"
	"testing"
	"time"

	//"github.com/atdiar/errcode"
	"github.com/atdiar/errors"
	_ "github.com/atdiar/init/debug"
	"github.com/atdiar/localmemstore"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

// todo: think about using a session Group for the roleList.

const (
	id0         = "id0"
	id1         = "id1"
	id2         = "id2"
	id3         = "id3"
	sessionid   = "sessionid"
	roleTableId = "roletableuid"
)

// RoleDB is a mock Role Database for test purposes whose implementation is merely
// an in-memory key-value store.
var RoleDB = localmemstore.New() //session.New("ROLEDB", "somesecret", session.FixedUUID(sessionid))

func saveRoleInDB(r Role) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}

	b2, err := RoleDB.Get(roleTableId, r.UID)
	if err == nil {
		storedRole := new(Role)
		err = json.Unmarshal(b2, storedRole)
		if err != nil {
			return err
		}
		if SameRoleDefinitions(r, *storedRole) {
			return nil
		}
		return errors.New("UNABLE TO ASSIGN ROLE. ROLE ID ALREADY IN USE ")
	}
	err = RoleDB.Put(roleTableId, r.UID, b, 0)
	return err
}

// AssignRoleToUserFn is an example of a afunction used to grant user roles.
// Here, a user is idenetified by its session id.
// The session storage is used to hold the list of roles a user has been assigned.
func AssignRoleToUserFn(s session.Handler) func(context.Context, http.ResponseWriter, *http.Request, Role) (context.Context, error) {
	return func(ctx context.Context, w http.ResponseWriter, req *http.Request, r Role) (context.Context, error) {
		err := saveRoleInDB(r)
		if err != nil {
			return ctx, err
		}
		ctx, err = s.Load(ctx, w, req)
		if err != nil {
			return ctx, err
		}

		b, err := json.Marshal(r)
		if err != nil {
			return ctx, err
		}

		b2, err := s.Get(r.UID)
		if err == nil {
			storedRole := new(Role)
			err = json.Unmarshal(b2, storedRole)
			if err != nil {
				return ctx, err
			}
			if SameAssignedRoles(r, *storedRole) {
				return ctx, nil
			}
			return ctx, errors.New("UNABLE TO ASSIGN ROLE. ROLE ID ALREADY EXISTS FOR THIS USER ")
		}
		err = s.Put(r.UID, b, r.Duration)
		if err != nil {
			return ctx, err
		}
		ctx, err = s.SetSessionCookie(ctx, w, req)
		return ctx, err
	}
}

// note: no need to check role expiry here since it is encoded in the way the role
// is being saved as a session object.
func AssertUserHasRoleFn(s session.Handler) func(context.Context, http.ResponseWriter, *http.Request, Role) (context.Context, error) {
	return func(ctx context.Context, w http.ResponseWriter, req *http.Request, r Role) (context.Context, error) {
		// first, we try to retrieve the session
		ctx, err := s.Load(ctx, w, req)
		if err != nil {
			return ctx, errors.New("unable to retrieve session in order to check user roles.").Wraps(err)
		}

		b2, err := s.Get(r.UID)
		if err != nil {
			return ctx, err
		}

		storedRole := new(Role)
		err = json.Unmarshal(b2, storedRole)
		if err != nil {
			return ctx, err
		}
		if SameRoleDefinitions(r, *storedRole) {
			return ctx, nil
		}
		return ctx, errors.New("DB ERROR: ROLE MISMATCH FOR SAME ROLE ID")

	}
}

func Multiplexer(t *testing.T) (xhttp.ServeMux, session.Handler) {
	mux := xhttp.NewServeMux()
	// setup a session handler that uses a fixed session id for testing purposes
	s := session.New("SID", "secretissecret", session.FixedUUID(sessionid))

	mux.USE(s)

	role0 := NewRole(id0, "role0", 0)
	role1 := NewRole(id1, "role1", 1*time.Hour)
	role2 := NewRole(id2, "role2", 1*time.Hour)
	role3 := NewRole(id3, "role3", 1*time.Hour)

	roles0 := NewRoleList(AssignRoleToUserFn(s), role0)
	roles123 := NewRoleList(AssignRoleToUserFn(s), role1, role2, role3)

	roleEnforcer0 := Enforce(roles0, AssertUserHasRoleFn(s))
	roleEnforcer123 := Enforce(roles123, AssertUserHasRoleFn(s))

	// this route is used to set user roles which should be persisted somewhere
	// for check on the role protected routes.
	mux.GET("/setroles", roles123.Link(xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		ctx, err := s.Load(ctx, w, r)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		id, _ := s.Cookie.ID()
		if id != sessionid {
			http.Error(w, "ID MISMATCH", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(id))
	})))

	mux.GET("/protected/zero", roleEnforcer0.Link(xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		w.Write(([]byte)("user was allowed. role0"))
	})))
	mux.GET("/protected/123", roleEnforcer123.Link(xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		w.Write(([]byte)("user was allowed. role123"))
	})))
	mux.GET("/protected/0123", xhttp.Chain(roleEnforcer0, roleEnforcer123).Link(xhttp.HandlerFunc(func(ctx context.Context, w http.ResponseWriter, r *http.Request) {
		w.Write(([]byte)("user was allowed. role0 and role123"))
	})))

	return mux, s
}

func TestRBAC(t *testing.T) {
	mux, _ := Multiplexer(t)

	// Make a request on /setroles in order to get assigned role1, role2, and role3
	req, err := http.NewRequest("GET", "/setroles", nil)
	if err != nil {
		t.Fatal(err)
	}

	// issue http request
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should capture the cookie
	wcookie := w.Result().Cookies()
	if wcookie == nil {
		t.Fatal("No cookie has been set, including session coookie.")
	}

	// Let's verify that the user session id is effectively being written route
	// in the response body.
	if b := w.Body; b.String() != sessionid {
		t.Fatalf("Expected: %v but got: %v \n", sessionid, b)
	}

	// Then let's make a request on /protected/123 . We should have all the authorizations
	// required.
	req, err = http.NewRequest("GET", "/protected/123", nil)
	if err != nil {
		t.Fatal(err)
	}
	// should add the cookies generated when roles were assigned
	for _, c := range wcookie {
		req.AddCookie(c)
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if b := w.Body.String(); b != "user was allowed. role123" {
		t.Fatalf("Expected: %v but got: %v \n", "user was allowed. role123", b)
	}

	// Let's now make a request on a route we do not have sufficient roles for
	// /protected/0 for instance
	req, err = http.NewRequest("GET", "/protected/0", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range wcookie {
		req.AddCookie(c)
	}
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if b := w.Body.String(); b == "user was allowed. role0" {
		t.Fatalf("Expected different but got %v \n", b)
	}

}
