package database

import "context"

/*
type (
	Interface = session.Store
)
*/

// Interface is a generic interface for database objects.
type Interface interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) ([]byte, error)
}
