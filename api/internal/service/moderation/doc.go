// Package moderation is the manual review queue in the founder's Telegram chat
// (§5).
//
// Two queues: questionnaires (photo and «о себе» — ok / photo not ok / text not
// ok) and user-created points (ok / reject, and for «к себе» additionally: the
// room photo is not stock, the share price is realistic). Searches are never
// moderated — they are structural and contain no text.
//
// The verdicts that matter are the two branches of a rejected photo. A photo is
// mandatory (§4), so there is no «без фото» status to fall back on: a
// questionnaire that never passed goes back to draft and out of every list
// until another photo is uploaded, while an approved one keeps its status —
// re-checking a photo must not throw somebody out of the pools they are in.
// The ban button is the outcome of an abuse report, not a content tool.
//
// Before a human sees anything there is automatic validation, and it lives in
// service/points: links only from avito.ru/cian.ru, price 20–300k, a station
// from the directory.
//
// Built in §12 step 5; the transport underneath it — sendPhoto, the callback
// polling and /queue — is step 6.
package moderation
