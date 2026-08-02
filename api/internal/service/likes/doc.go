// Package likes owns both gestures (§2.2, §2.3).
//
// The cheap like — «Интересно — войти в пул» — is unlimited, one tap, and
// exists ONLY for market flats. Zone pools and «к себе» have no join gesture.
//
// The expensive like is «хочу жить вместе»: 5 per day, placed only from a full
// profile or from a «к себе» card, and it requires the liker to have some
// active search — skin in the game, not a geography check. It records its
// context (zone_id OR point_id). Replying to an incoming like costs nothing
// from the budget and requires no active search.
//
// The rule that shapes this package: an expensive like NEVER expires — not when
// someone leaves a pool, not when a point changes, not when a search closes
// (§9 cancelled every form of expiry). Its only terminal states are a match or
// waiting forever. Instead of expiry there is transparency: a liker with no
// active searches is shown with a grey «сейчас не в поиске» note.
//
// Empty on purpose — built in §12 step 4.
package likes
