// Package zones is the directory of Moscow districts, metro stations and metro
// lines (§8).
//
// A zone resolves to a set of stations, and two zones intersect when their
// station sets do — that is what makes a search for «Сокол» show up in the
// «оранжевая ветка» pool (§2.1). The directory is finite and versioned in the
// repository, not user-editable.
package zones
