/* Профиль человека — §6. Дорогой лайк живёт только здесь (MainButton).
   Совпадения анкеты: зелёный фон «вы оба», нейтральный — различие с контекстом.
   Красного в сравнении людей не существует. */
import type { CSSProperties, ReactNode } from 'react';
import { Avatar, Badge, S } from '../ui/primitives';

export type MatchRow = {
  /** both = «вы оба» (зелёный), diff = различие с контекстом (нейтральный) */
  tone: 'both' | 'diff';
  title: string;
  context: string; // «вы оба», «у тебя кот — ей ок»
};

/**
 * Состояние дорогого лайка — одно на обе кнопки.
 *
 * В Telegram лайк ставится родной MainButton, в браузере — кнопкой вёрстки, но
 * текст и запрет считает контейнер один раз: две ветки логики разошлись бы.
 */
export type LikeAction = {
  /** «Хочу жить вместе · осталось 3 из 5», «Ответить — мэтч сразу». */
  label: string;
  /** Подпись под кнопкой: «Ответный лайк не тратит лимит» или отказ сервера. */
  caption?: string;
  disabled?: boolean;
  /** Кнопку рисует Telegram — в вёрстке остаётся только подпись. */
  native?: boolean;
};

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
    gap: 6,
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
    gap: 14,
    padding: '14px 12px 20px',
  } as CSSProperties,
  block: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    padding: 12,
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  zoneLine: {
    display: 'flex',
    alignItems: 'center',
    gap: 7,
    padding: '9px 11px',
    borderRadius: 10,
    background: 'var(--app-bg)',
  } as CSSProperties,
  matchRow: (both: boolean): CSSProperties => ({
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    padding: '11px 12px',
    borderRadius: 11,
    background: both ? 'var(--verdict-ok-bg)' : 'var(--app-bg)',
  }),
  fixed: {
    position: 'sticky',
    bottom: 0,
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    padding: '10px 12px',
    paddingBottom: 'max(12px, env(safe-area-inset-bottom))',
    background: 'var(--tg-theme-bg-color)',
    borderTop: '1px solid var(--sep)',
  } as CSSProperties,
  mainButton: (disabled: boolean): CSSProperties => ({
    width: '100%',
    padding: 14,
    border: 'none',
    borderRadius: 11,
    background: disabled ? 'var(--chip-bg)' : 'var(--tg-theme-button-color)',
    font: 'inherit',
    fontSize: 16,
    fontWeight: 590,
    color: disabled ? 'var(--tg-theme-hint-color)' : 'var(--tg-theme-button-text-color)',
    cursor: disabled ? 'default' : 'pointer',
  }),
};

function ZoneLine({ children }: { children: ReactNode }) {
  return (
    <div style={L.zoneLine}>
      <span
        style={{
          width: 7,
          height: 7,
          borderRadius: '50%',
          background: 'var(--type-search-fg)',
          flex: '0 0 auto',
        }}
      />
      <span style={{ fontSize: 13 }}>{children}</span>
    </div>
  );
}

export function PersonProfileScreen({
  name,
  age,
  occupation,
  photoUrl,
  verifiedBadge,
  lastSeen,
  zone,
  matches,
  matchesTotal,
  about,
  like,
  onBack,
  onLike,
}: {
  name: string;
  age: number;
  occupation: string;
  /** Пусто = фото не одобрено: заглушка с инициалами, а не пустой круг (§5). */
  photoUrl?: string;
  /** Бейджи и «была сегодня» сервер пока не шлёт — строк тогда просто нет. */
  verifiedBadge?: string;
  lastSeen?: string;
  /** Зона контекста, «Хамовники». Профиль открыт не из пула — строки нет. */
  zone?: string;
  matches: MatchRow[];
  matchesTotal: number; // «6 ИЗ 9»
  /** Только одобренное «о себе»: до аппрува блока нет вовсе (§5). */
  about?: string;
  like: LikeAction;
  onBack?: () => void;
  onLike?: () => void;
}) {
  const bothCount = matches.filter((m) => m.tone === 'both').length;

  return (
    <div style={L.screen}>
      <div style={L.header}>
        <span
          style={{ fontSize: 22, lineHeight: 1, color: 'var(--tg-theme-accent-text-color)', cursor: 'pointer' }}
          onClick={onBack}
        >
          ‹
        </span>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>{name}</span>
      </div>

      <div style={L.body}>
        <div style={{ ...L.block, gap: 12, padding: '14px 12px' }}>
          <div style={{ display: 'flex', gap: 14, alignItems: 'center' }}>
            <Avatar name={name} size={88} src={photoUrl} />
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0 }}>
              <span style={{ fontSize: 22, fontWeight: 640, letterSpacing: '-0.5px' }}>
                {name}, {age}
              </span>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
                <span style={{ fontSize: 14, color: 'var(--tg-theme-subtitle-text-color)' }}>
                  {occupation}
                </span>
                {verifiedBadge && <Badge tone="accent">{verifiedBadge}</Badge>}
              </div>
              {lastSeen && (
                <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>{lastSeen}</span>
              )}
            </div>
          </div>
          {zone && (
            <ZoneLine>
              пересеклись: <b style={{ fontWeight: 590 }}>{zone}</b>
            </ZoneLine>
          )}
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <span style={S.sectionTitle}>
            СОВПАДЕНИЯ АНКЕТЫ · {bothCount} ИЗ {matchesTotal}
          </span>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {matches.map((m) => {
              const both = m.tone === 'both';
              return (
                <div key={m.title} style={L.matchRow(both)}>
                  <span
                    style={{
                      flex: '0 0 auto',
                      fontSize: 13,
                      fontWeight: 640,
                      color: both ? 'var(--verdict-ok-fg)' : 'var(--tg-theme-hint-color)',
                    }}
                  >
                    {both ? '✓' : '·'}
                  </span>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 1, minWidth: 0 }}>
                    <span
                      style={{
                        fontSize: 14,
                        fontWeight: 590,
                        color: both ? 'var(--verdict-ok-fg)' : 'var(--tg-theme-text-color)',
                      }}
                    >
                      {m.title}
                    </span>
                    <span
                      style={{
                        fontSize: 13,
                        color: both
                          ? 'color-mix(in srgb, var(--verdict-ok-fg) 78%, transparent)'
                          : 'var(--tg-theme-subtitle-text-color)',
                      }}
                    >
                      {m.context}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        {about && (
          <div style={L.block}>
            <span style={S.blockTitle}>О себе</span>
            <span style={{ fontSize: 14, lineHeight: 1.45, textWrap: 'pretty' }}>{about}</span>
          </div>
        )}
      </div>

      {/* В Telegram кнопку рисует MainButton, и здесь остаётся только подпись:
          две одинаковые кнопки друг над другом — это два разных обещания. */}
      {(!like.native || like.caption) && (
        <div style={L.fixed}>
          {!like.native && (
            <button
              type="button"
              style={L.mainButton(Boolean(like.disabled))}
              disabled={like.disabled}
              onClick={onLike}
            >
              {like.label}
            </button>
          )}
          {like.caption && <span style={S.caption}>{like.caption}</span>}
        </div>
      )}
    </div>
  );
}
