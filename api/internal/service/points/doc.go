// Package points owns points — the two kinds of object with a flat behind them
// (§2.2).
//
// `flat` is a listing from the market: a link, a parsed Telegram channel post
// or a manual entry. It is depersonalised — created_by exists for provenance,
// limits and moderation only, is never shown, and confers no ownership. Its
// pool is the set of cheap likes: «Интересно — войти в пул», unlimited, one tap.
//
// `intent_home` is «сосед к себе»: someone with a specific flat looking for
// somebody to move in. Two stages — `live` (already living there, at most 1)
// and `moving_in` (agreed with the landlord and moving in, at most 2), never
// more than 2 active in total, and they do not consume search slots. It HAS NO
// POOL: every applicant wants the same one room, so the response is an
// expensive like straight to the author («Хочу эту комнату»).
//
// Also owns freshness (§8): the crowdsourced «Объявление ещё живое?» vote after
// following a link out, hiding at max(2, 30% of the pool) «сдана» votes, the
// 14-day TTL, and last_seen_alive.
//
// Empty on purpose — built in §12 step 5.
package points
