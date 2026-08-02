// Package moderation is the manual review queue in the founder's Telegram chat
// (§5).
//
// Two queues: questionnaires (photo and «о себе» — ok / photo not ok / text not
// ok) and user-created points (ok / reject, and for «к себе» additionally: the
// room photo is not stock, the share price is realistic). Searches are never
// moderated — they are structural and contain no text.
//
// Distinctions that matter: a rejected photo is not a ban, it means status
// «без фото» and lower ranking. The ban button is the outcome of an abuse
// report, not a content tool. Before a human sees anything there is automatic
// validation: links only from avito.ru/cian.ru, price 20–300k, a station from
// the directory.
//
// Also owns optional email verification: a university domain from the config
// whitelist earns a badge. It is never a gate and always free.
//
// Empty on purpose — built in §12 step 5.
package moderation
