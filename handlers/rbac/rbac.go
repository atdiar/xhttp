package rbac

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

type contextKey struct{}

type Role struct {
	UID         string
	Name        string
	Permissions []string
	Duration    time.Duration
	CreatedAt   time.Time
	ContextKey  *contextKey `json:"-"`
}

func NewRole(uid string, name string, duration time.Duration, perms ...string) Role {
	return Role{uid, name, perms, duration, time.Now(), new(contextKey)}
}

func newRole() Role {
	return Role{
		UID:         "",
		Name:        "",
		Permissions: make([]string, 0),
		Duration:    900000 * time.Hour,
		CreatedAt:   time.Now().UTC(),
		ContextKey:  new(contextKey),
	}
}

// Persist allows for the storage of a role into a sql database.
// The storing function is using an implicit prepared statement that can be
// seen in the following constructor:
//
// PreparedStmt defines an alias for a  constructor of functions that execute
// a Prepared Statement to store user info into the database.
// type PreparedStmt = func(*sql.Stmt) func(userinfo interface{}) (sql.Result, error)
func (r *Role) Persist(SaveRole func(v interface{}) (*sql.Result, error)) error {
	_, err := SaveRole(r)
	return err
}

type RoleList struct {
	Roles map[*contextKey]Role
	next  xhttp.Handler
}

func NewRoleList(roles ...Role) RoleList {
	m := make(map[*contextKey]Role)
	for _, r := range roles {
		m[r.ContextKey] = r
	}
	return RoleList{m, nil}
}

// Unroll returns the list of Role in slice form.
func (rl RoleList) Unroll() []Role {
	s := make([]Role, 0)
	for _, r := range rl.Roles {
		s = append(s, r)
	}
	return s
}

func (rl RoleList) Persist(SaveRoles func(v interface{}) (*sql.Result, error)) error {
	roles := rl.Unroll()
	var err error
	for _, r := range roles {
		err = r.Persist(SaveRoles)
		if err != nil {
			return err
		}
	}
	return nil
}

// AccessGranted is used to make sure that a user has the access rights granted by a
// given list of Roles.
func (rl RoleList) AccessGranted(s session.Interface) bool {
	for _, r := range rl.Unroll() {
		_, err := s.Get(r.UID)
		if err != nil {
			return false
		}
		continue
	}
	return true
}

// ServeHTTP is used for the request handling logic, that is, defining the
// role-based access or the permission-based access
func (rl RoleList) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	for _, role := range rl.Unroll() {
		ctx = context.WithValue(ctx, role.ContextKey, role)
	}
	if rl.next != nil {
		rl.next.ServeHTTP(ctx, w, r)
	}
}

type Enforcer struct {
	Roles   RoleList
	Session session.Interface
	next    xhttp.Handler
}

func Enforce(r RoleList, s session.Interface) Enforcer {
	return Enforcer{r, s, nil}

}

func (e Enforcer) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if e.Roles.AccessGranted(e.Session) {
		if e.next != nil {
			e.next.ServeHTTP(ctx, w, r)
			return
		}
	}
	http.Error(w, "Access Denied, Role or permission missing.", http.StatusUnauthorized)
}

func (e Enforcer) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	e.next = hn
	return e
}
