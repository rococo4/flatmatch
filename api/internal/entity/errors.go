package entity

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("not found")

var ErrSearchLimit = errors.New("active search limit reached")

var ErrSearchClosed = errors.New("search is closed")

var ErrLikeLimit = errors.New("daily like limit reached")

var ErrNeedSearch = errors.New("an active search is required to like")

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
