package rbac

import (
	"context"
	"log"
	//	"log"
	"net/http"
	"time"

	"github.com/atdiar/xhttp"
)

// todo: secure roles, cookie based persistence is unsecure
// todo: implementing role early expiry/removal.

type contextKey string

// Role defines a user role. User roles can be used to grant access to parts of an
// application to a selection of credited clients.
// If permissions are added, permissions will have to be checked.
type Role struct {
	UID         string
	Name        string
	Permissions map[string]interface{}
	Duration    time.Duration
	CreatedAt   time.Time
	AssignedOn  time.Time
	ContextKey  *contextKey `json:"-"`
}

// NewRole creates a Role and persists it was not already persisted.
func NewRole(uid string, name string, duration time.Duration, perms ...string) Role {
	perm := make(map[string]interface{}, len(perms))
	for _, p := range perms {
		perm[p] = nil
	}
	contextEntry := contextKey("")
	return Role{
		UID:         uid,
		Name:        name,
		Permissions: perm,
		Duration:    duration,
		CreatedAt:   time.Now().UTC(),
		AssignedOn:  time.Time{},
		ContextKey:  &contextEntry,
	}
}

// SameRoleDefinitions is an equality test for Roles.
func SameRoleDefinitions(r, t Role) bool {
	if r.UID == t.UID && r.Name == t.Name && r.Duration == t.Duration && r.CreatedAt.Equal(t.CreatedAt) && len(r.Permissions) == len(t.Permissions) {
		for k, _ := range t.Permissions {
			_, ok := r.Permissions[k]
			if !ok {
				return false
			}
		}
		return true
	}
	return false
}

// SameAssignedRoles is an equality test for roles that have been assigned.
func SameAssignedRoles(r, t Role) bool {
	if r.UID == t.UID && r.Name == t.Name && r.Duration == t.Duration && r.CreatedAt.Equal(t.CreatedAt) && r.AssignedOn.Equal(t.AssignedOn) && len(r.Permissions) == len(t.Permissions) {
		for k, _ := range t.Permissions {
			_, ok := r.Permissions[k]
			if !ok {
				return false
			}
		}
		return true
	}
	return false
}

// RoleList defines a list of roles that may be enforced simultaneously.
type RoleList struct {
	Roles      map[*contextKey]Role
	AssignRole func(http.ResponseWriter, *http.Request, Role) error
	next       xhttp.Handler
}

// NewRoleList creates a RoleList.
// The first argument is the function used to assign roles in response to a http request
// to be granted said roles.
func NewRoleList(AssignFunc func(http.ResponseWriter, *http.Request, Role) error, roles ...Role) RoleList {
	m := make(map[*contextKey]Role)
	for _, role := range roles {
		m[role.ContextKey] = role
	}
	return RoleList{m, AssignFunc, nil}
}

func (rl RoleList) ServeHTTP( w http.ResponseWriter, req *http.Request) {
	ctx:= req.Context()
	if rl.AssignRole == nil {
		http.Error(w, "", http.StatusInternalServerError)
	}
	var err error
	for _, r := range rl.Roles {
		r.AssignedOn = time.Now().UTC()
		err = rl.AssignRole(w, req, r)
		if err != nil {
			http.Error(w, "unable to grant authorization", http.StatusInternalServerError)
			return
		}
		ctx = context.WithValue(ctx, r.ContextKey, r)
	}
	req = req.WithContext(ctx)
	if rl.next != nil {
		rl.next.ServeHTTP(w, req)
	}
}

func (rl RoleList) Link(h xhttp.Handler) xhttp.HandlerLinker {
	rl.next = h
	return rl
}

// Enforcer is a xhttp handler that is used to make sure that access to a server endpoint
// is made with the proper roles and/or permissions.
type Enforcer struct {
	Roles                RoleList
	AuthorizationChecker func(http.ResponseWriter, *http.Request, Role) error
	next                 xhttp.Handler
}

// Enforce returns a role-based access checking xhttp.Handler.
// As in the Rolelist AccessGranted method, it takes as argument a function that
// checks if a user has the proper roles.
func Enforce(r RoleList, AuthorizationChecker func(http.ResponseWriter, *http.Request, Role) error) Enforcer {
	return Enforcer{r, AuthorizationChecker, nil}
}

func (e Enforcer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx:= r.Context()
	var err error
	for _, role := range e.Roles.Roles {
		err = e.AuthorizationChecker(w, r, role)
		if err != nil {
			log.Print("Err: \n", err, "\n", role)
			http.Error(w, "Access Denied, Role or permission missing.", http.StatusUnauthorized)
			return
		}
		ctx = context.WithValue(ctx, role.ContextKey, role)
	}
	r = r.WithContext(ctx)
	if e.next != nil {
		e.next.ServeHTTP(w, r)
		return
	}
}

func (e Enforcer) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	e.next = hn
	return e
}
