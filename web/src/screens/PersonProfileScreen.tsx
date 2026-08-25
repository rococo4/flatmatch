/* Профиль человека — §6. Дорогой лайк живёт только здесь (MainButton).
   Совпадения анкеты: зелёный фон «вы оба», нейтральный — различие с контекстом.
   Красного в сравнении людей не существует.

   Экран один на два входа: из подбора (внизу дорогой лайк) и из «Лайков»
   (внизу стрелка и сердце — их подставляет `bottom`). Разводить их в две
   вёрстки нельзя: анкета одна и та же, и разъехалась бы она мгновенно. */
import type { CSSProperties, ReactNode } from 'react';
import type { SharedAnswer } from '../api/client';
import { fieldLabel } from '../api/format';
import { PhotoPager } from '../ui/PhotoPager';
import { Badge, S } from '../ui/primitives';

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
  /** Зелёная: вы уже в мэтче, кнопка ведёт в него, а не лайкает. */
  match?: boolean;
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
  gradient: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    display: 'flex',
    flexDirection: 'column',
    gap: 2,
    padding: '34px 14px 12px',
    background: 'linear-gradient(to top, rgba(0,0,0,.72), rgba(0,0,0,0))',
    color: '#fff',
  } as CSSProperties,
  matchRow: (both: boolean): CSSProperties => ({
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    padding: '11px 12px',
    borderRadius: 11,
    background: both ? 'var(--verdict-ok-bg)' : 'var(--app-bg)',
  }),
  /** Тихая строка действий: жалоба и блок не соревнуются с дорогим лайком. */
  actions: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    paddingTop: 2,
  } as CSSProperties,
  quiet: {
    border: 'none',
    background: 'none',
    font: 'inherit',
    fontSize: 13,
    color: 'var(--tg-theme-hint-color)',
    cursor: 'pointer',
    padding: 0,
  } as CSSProperties,
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

/**
 * Совпадения анкеты строками — общие для обоих входов на экран.
 *
 * Сервер шлёт только совпавшее, различий в ответе нет: красного в сравнении
 * людей не существует (§6). Подпись строки — «вы оба» и ничего больше: метки
 * ответов не склоняются («не курю», «творческий беспорядок»), и фраза,
 * собранная из них, читалась бы поломанной.
 */
export function sharedRows(shared: SharedAnswer[]): MatchRow[] {
  return shared.map((s) => ({
    tone: 'both',
    title: capitalize(fieldLabel(s.field, s.value)),
    context: 'вы оба',
  }));
}

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
  photoUrls,
  verifiedBadge,
  lastSeen,
  note,
  zone,
  point,
  matches,
  matchesTotal,
  about,
  like,
  bottom,
  onBack,
  onLike,
  onReport,
  onBlock,
}: {
  name: string;
  age: number;
  occupation: string;
  /** Первое — главное; пустой набор = фото не одобрено, и вместо него заглушка (§5). */
  photoUrls: string[];
  /** Бейдж вуза сервер пока не шлёт — строки тогда просто нет. */
  verifiedBadge?: string;
  lastSeen?: string;
  /** Третьим в подзаголовке: «лайкнула тебя» на анкете из «Лайков» (§6). */
  note?: string;
  /** Зона контекста, «Хамовники». Контекста не осталось — строки нет. */
  zone?: string;
  /**
   * Точка контекста готовой строкой: «на комнату Кати · м. Сокол».
   *
   * Без «пересеклись:» — в отличие от зоны, метка точки уже рассказывает о
   * себе целиком, и приставка сделала бы из неё «пересеклись: на комнату».
   */
  point?: string;
  matches: MatchRow[];
  matchesTotal: number; // «6 ИЗ 9»
  /** Только одобренное «о себе»: до аппрува блока нет вовсе (§5). */
  about?: string;
  like?: LikeAction;
  /** Чем заменить нижнюю полосу: стрелка и сердце «Лайков» вместо лайка. */
  bottom?: ReactNode;
  onBack?: () => void;
  onLike?: () => void;
  /** Жалоба и блок (§5) — тихой строкой под контентом, а не кнопками наравне. */
  onReport?: () => void;
  onBlock?: () => void;
}) {
  const bothCount = matches.filter((m) => m.tone === 'both').length;
  const subtitle = [occupation, lastSeen, note].filter(Boolean).join(' · ');

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
        <div style={{ ...L.block, gap: 0, padding: 0, overflow: 'hidden' }}>
          <PhotoPager urls={photoUrls} emptyLabel="фото на проверке" maxHeight="46vh">
            <div style={L.gradient}>
              <span style={{ fontSize: 24, fontWeight: 640, letterSpacing: '-0.5px' }}>
                {name}, {age}
              </span>
              {subtitle && <span style={{ fontSize: 14, opacity: 0.9 }}>{subtitle}</span>}
            </div>
          </PhotoPager>
          {(verifiedBadge || zone || point) && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, padding: 12 }}>
              {verifiedBadge && (
                <div>
                  <Badge tone="accent">{verifiedBadge}</Badge>
                </div>
              )}
              {zone && (
                <ZoneLine>
                  пересеклись: <b style={{ fontWeight: 590 }}>{zone}</b>
                </ZoneLine>
              )}
              {point && (
                <ZoneLine>
                  <b style={{ fontWeight: 590 }}>{point}</b>
                </ZoneLine>
              )}
            </div>
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

        {(onReport || onBlock) && (
          <div style={L.actions}>
            <button type="button" style={L.quiet} onClick={onReport}>
              Пожаловаться
            </button>
            <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>·</span>
            <button type="button" style={L.quiet} onClick={onBlock}>
              Заблокировать
            </button>
          </div>
        )}
      </div>

      {/* Своя нижняя полоса главнее: у анкеты из «Лайков» внизу два жеста, и
          дорогому лайку там места нет. */}
      {bottom && <div style={L.fixed}>{bottom}</div>}

      {/* В Telegram кнопку рисует MainButton, и здесь остаётся только подпись:
          две одинаковые кнопки друг над другом — это два разных обещания. */}
      {!bottom && like && (!like.native || like.caption) && (
        <div style={L.fixed}>
          {!like.native && (
            <button
              type="button"
              style={{
                ...L.mainButton(Boolean(like.disabled)),
                ...(like.match
                  ? { background: 'var(--verdict-ok-bg)', color: 'var(--verdict-ok-fg)' }
                  : {}),
              }}
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

function capitalize(s: string): string {
  return s ? s[0].toUpperCase() + s.slice(1) : s;
}
