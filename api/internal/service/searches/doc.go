// Package searches owns the search — a user's subscription to one zone plus a
// per-person budget range, and optionally room count, move-in month, term and
// "what's already there" chips (§2.1).
//
// A search is not a card and is never published: it puts its author into every
// zone pool whose zone intersects theirs, and it feeds matching and pushes.
// At most 3 active per person — the only limit. One person's zones MAY overlap
// or even coincide (§2.1): «Хамовники» and «красная ветка» are two honest
// different intents; the pool shows such a person once, by their best verdict.
//
// Also owns the lifecycle from §3: TTL «ещё ищешь?», pause on silence,
// close-with-a-reason where «нашёл здесь» is the product's headline metric, and
// reactivation from the archive.
//
// The filter set is FIXED (§2.1) and must not grow: zone, per-person budget,
// rooms, move-in month, term. Gender is a server-side visibility rule, not a
// filter. Habits are sorting signals and verdict chips, never cut-offs.
//
// Empty on purpose — built in §12 step 3.
package searches
