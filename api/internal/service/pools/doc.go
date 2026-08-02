// Package pools computes pools and the visibility rule they all go through.
//
// A zone pool is not a stored entity (§2.1): it is the set of everyone whose
// active search intersects the zone, derived on read from searches × zones.
// Nobody owns it, nobody joins it — creating a search puts you in, closing it
// takes you out. Inside, ranking is personal to the viewer's own search and
// splits into three sections: «Совпали по всем параметрам» / «Пересекаются
// частично» / «Не совпали». A match is an INTERSECTION of ranges, not equality,
// and a NULL parameter means «неважно» and intersects everything.
//
// This package also owns visible_people(viewer_id, ...) from §4: the single
// function every consumer — zone pools, flat pools, counters, pushes, like
// validation — must go through, so the mutual gender_pref rule and blocks can
// never be forgotten in one query. Counters are therefore personal.
//
// Empty on purpose — built in §12 step 3.
package pools
