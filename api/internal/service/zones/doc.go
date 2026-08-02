// Package zones is the directory of Moscow districts, metro stations and metro
// lines (§8).
//
// A zone resolves to a set of stations, and two zones intersect when their
// station sets do — that is what makes a search for «Сокол» show up in the
// «оранжевая ветка» pool (§2.1). The directory is finite and versioned in the
// repository, not user-editable.
//
// Empty on purpose — built in §12 step 3.
package zones
