/* Вкладка «Лайки» — плитки входящих (§6).

   Две в ряд, порядок — серверный (по совпадению, §2.3 дизайна): сортировать
   на клиенте нечем, пара считается там же, где колода. Слов над списком нет:
   «ответный лайк не тратит лимит» живёт на анкете лайкнувшего, где сердце и
   нажимают, а не над плитками, где нажать нечего.

   Тап по плитке ведёт в анкету лайкнувшего — там стрелка и сердце. */
import type { CSSProperties } from 'react';

import type { IncomingLikeDTO } from '../api/client';
import { paramName } from '../api/format';
import { Badge, PhotoPlaceholder, VerdictChip } from '../ui/primitives';

const L = {
  screen: {
    display: 'flex',
    flexDirection: 'column',
    minHeight: '100%',
    background: 'var(--app-bg)',
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '0 14px',
    height: 52,
    flex: '0 0 52px',
    background: 'var(--tg-theme-header-bg-color)',
    borderBottom: '1px solid var(--sep)',
  } as CSSProperties,
  body: {
    flex: '1 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
    padding: '12px 12px 20px',
  } as CSSProperties,
  grid: { display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 } as CSSProperties,
  photo: {
    position: 'relative',
    width: '100%',
    aspectRatio: '3/4',
    background: 'var(--chip-bg)',
    overflow: 'hidden',
  } as CSSProperties,
  /** Подложка белёсая в обеих темах: под ней фото, а не поверхность приложения. */
  badge: {
    position: 'absolute',
    left: 8,
    top: 8,
    maxWidth: 'calc(100% - 16px)',
    padding: '3px 8px',
    borderRadius: 7,
    background: 'rgba(255, 255, 255, .92)',
    fontSize: 11,
    fontWeight: 640,
    letterSpacing: '-0.1px',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  } as CSSProperties,
  gradient: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    padding: '26px 10px 8px',
    background: 'linear-gradient(to top, rgba(0,0,0,.72), rgba(0,0,0,0))',
    color: '#fff',
    fontSize: 15,
    fontWeight: 640,
    letterSpacing: '-0.3px',
  } as CSSProperties,
  chips: { display: 'flex', flexWrap: 'wrap', gap: 5, padding: '8px 8px 10px' } as CSSProperties,
  empty: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 10,
    padding: '28px 16px',
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
};

export function LikesScreen({
  likes,
  error,
  onOpen,
}: {
  /** Входящие как их отдал сервер: порядок — часть ответа, не деталь экрана. */
  likes: IncomingLikeDTO[];
  error?: string | null;
  onOpen?: (userId: string) => void;
}) {
  return (
    <div style={L.screen}>
      <div style={L.header}>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Лайки</span>
        {likes.length > 0 && (
          <span style={{ fontSize: 15, color: 'var(--tg-theme-hint-color)' }}>{likes.length}</span>
        )}
      </div>

      <div style={L.body}>
        {error && (
          <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>
            {error}
          </span>
        )}

        {likes.length === 0 ? (
          <div style={L.empty}>
            <span style={{ fontSize: 16, fontWeight: 590, textAlign: 'center', textWrap: 'pretty' }}>
              Пока никто не лайкнул. Появись в подборе — сохрани поиск
            </span>
          </div>
        ) : (
          <div style={L.grid}>
            {likes.map((l) => (
              <LikeTile key={l.user_id} like={l} onOpen={onOpen} />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * Плитка: лицо, бейдж места и ✓-чипы совпавшего.
 *
 * Зелёная рамка и зелёный бейдж — только у отклика на мою комнату: это другой
 * разговор («он хочет ко мне»), и путать его с «ищет там же» нельзя (§2.2).
 */
function LikeTile({
  like,
  onOpen,
}: {
  like: IncomingLikeDTO;
  onOpen?: (userId: string) => void;
}) {
  const room = Boolean(like.pair.room);
  const zone = like.pair.zone?.name;
  // Серые и жёлтые чипы на плитке не рисуются: места мало, а «не совпало» на
  // входящем лайке — не то, ради чего его открывают.
  const green = (like.pair.chips ?? []).filter((c) => c.verdict === 'green');

  return (
    <article
      onClick={() => onOpen?.(like.user_id)}
      style={{
        display: 'flex',
        flexDirection: 'column',
        borderRadius: 14,
        overflow: 'hidden',
        background: 'var(--card-bg)',
        border: room ? '1.5px solid var(--type-home-fg)' : '1px solid var(--sep)',
        cursor: onOpen ? 'pointer' : 'default',
      }}
    >
      <div style={L.photo}>
        {like.photo_urls.length > 0 ? (
          <img
            src={like.photo_urls[0]}
            alt=""
            style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
          />
        ) : (
          <PhotoPlaceholder label="фото на проверке" style={{ width: '100%', height: '100%' }} />
        )}
        {(room || zone) && (
          <span
            style={{
              ...L.badge,
              color: room ? 'var(--type-home-fg)' : 'var(--type-search-fg)',
            }}
          >
            {room ? 'на твою комнату' : `ищет: ${zone}`}
          </span>
        )}
        <div style={L.gradient}>
          {like.name}, {like.age}
        </div>
      </div>

      {(green.length > 0 || like.not_searching) && (
        <div style={L.chips}>
          {green.map((c) => (
            <VerdictChip key={c.param} verdict="ok">
              {paramName(c.param)}
            </VerdictChip>
          ))}
          {/* §2.3: лайк жив, а человек сейчас не ищет — пометка, а не причина
              прятать его от адресата. */}
          {like.not_searching && <Badge>сейчас не в поиске</Badge>}
        </div>
      )}
    </article>
  );
}
