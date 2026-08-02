// Package matches turns mutual expensive likes into a match (§2.3).
//
// Contexts need not agree. A match hands both sides contact details plus the
// «почему совпали» summary: where they crossed paths, which search parameters
// overlap, and which questionnaire answers they share — ready-made openers for
// a first message. It is irreversible and private to the pair.
//
// Two simultaneous likes must not produce two matches: creation runs in a
// transaction behind a unique index.
//
// A match is also where the product's success metric starts — the survey 5–7
// days later, and the search closed with «нашёл здесь» (§1, §7).
//
// Empty on purpose — built in §12 step 4.
package matches
