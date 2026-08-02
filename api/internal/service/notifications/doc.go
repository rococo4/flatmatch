// Package notifications delivers pushes through the bot (§7).
//
// MVP types: a new searcher in your zone, somebody entered a flat's pool,
// «тебя лайкнул {имя}», «мэтч с {имя}», plus service messages — a flat that
// looks taken, a changed point, moderation verdicts, the TTL «ещё ищешь?» and
// the post-match survey.
//
// The constraints are the point of this package: at most one push per zone per
// day, at most 2–3 per user per day overall, and every send is logged in
// notifications with a unique index so duplicates are impossible at the
// database level rather than by careful coding.
//
// Sending goes through tgbot; who may be told about whom goes through
// visible_people (§4).
//
// Empty on purpose — built in §12 step 6.
package notifications
