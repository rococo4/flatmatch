package entity

import (
	"errors"
	"fmt"
)

// ErrNotFound is what a repository answers when the row is not there. Declared
// here so a service can recognise it without importing the package that talks
// to Postgres.
var ErrNotFound = errors.New("not found")

// FieldsError carries the names InvalidFields and MissingFields return, so a
// caller can say which fields to fix without deriving the list a second time.
type FieldsError struct {
	Err    error
	Fields []string
}

func (e *FieldsError) Error() string {
	return fmt.Sprintf("%v: %v", e.Err, e.Fields)
}

func (e *FieldsError) Unwrap() error { return e.Err }
