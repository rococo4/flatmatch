// Package entity holds the domain types of the product: what a user, a search,
// a point, a like and a match are, independent of how they are stored or
// transported.
//
// The rule that makes this package worth having: it imports nothing from the
// rest of the codebase and nothing that knows about Postgres, HTTP or Telegram.
// Every other package may depend on entity; entity depends on none of them.
// So the shape of a domain type is decided by the product, never by a column
// type or a JSON field.
//
// Concretely, that means no `db:` or `json:` tags live here. Repositories keep
// their own row structs and map to and from these types; API packages keep
// their own DTOs. That duplication is the point — it is what lets the storage
// schema and the wire format change without dragging the domain along.
package entity
