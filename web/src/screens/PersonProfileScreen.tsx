/* Профиль человека — §6. Дорогой лайк живёт только здесь (MainButton).
   Совпадения анкеты: зелёный фон «вы оба», нейтральный — различие с контекстом.
   Красного в сравнении людей не существует. */
import type { CSSProperties, ReactNode } from 'react';
import { Avatar, Badge, LimitChip, S, type Fact } from '../ui/primitives';

export type MatchRow = {
  /** both = «вы оба» (зелёный), diff = различие с контекстом (нейтральный) */
  tone: 'both' | 'diff';
  title: string;
  context: string; // «вы оба поздно ложитесь», «у тебя кот — ей ок»
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
    alignItems: 'center',
    gap: 10,
    padding: '10px 12px',
    paddingBottom: 'max(12px, env(safe-area-inset-bottom))',
    background: 'var(--tg-theme-bg-color)',
    borderTop: '1px solid var(--sep)',
  } as CSSProperties,
  mainButton: {
    flex: '1 1 auto',
    padding: 14,
    border: 'none',
    borderRadius: 11,
    background: 'var(--tg-theme-button-color)',
    font: 'inherit',
    fontSize: 16,
    fontWeight: 590,
    color: 'var(--tg-theme-button-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
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
  verifiedBadge,
  lastSeen,
  zone,
  matches,
  matchesTotal,
  about,
  searchFacts,
  likesLeft,
  likesTotal,
  onBack,
  onLike,
}: {
  name: string;
  age: number;
  occupation: string;
  verifiedBadge?: string;
  lastSeen: string; // «была сегодня»
  zone: string; // «Хамовники»
  matches: MatchRow[];
  matchesTotal: number; // «6 ИЗ 9»
  about: string;
  searchFacts: Fact[];
  likesLeft: number; // «3 из 5»
  likesTotal: number;
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
            <Avatar name={name} size={88} />
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
              <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>{lastSeen}</span>
            </div>
          </div>
          <ZoneLine>
            пересеклись: <b style={{ fontWeight: 590 }}>{zone}</b>
          </ZoneLine>
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

        <div style={L.block}>
          <span style={S.blockTitle}>О себе</span>
          <span style={{ fontSize: 14, lineHeight: 1.45, textWrap: 'pretty' }}>{about}</span>
        </div>

        <div style={L.block}>
          <span style={S.blockTitle}>Её поиск</span>
          {searchFacts.map((f) => (
            <div key={f.label} style={S.row}>
              <span style={S.rowLabel}>{f.label}</span>
              <span style={{ fontWeight: 510, textAlign: 'right' }}>{f.value}</span>
            </div>
          ))}
        </div>
      </div>

      {/* В реализации — нативная MainButton; здесь та же зона визуально. */}
      <div style={L.fixed}>
        <button type="button" style={L.mainButton} onClick={onLike}>
          Хочу жить вместе
        </button>
        <LimitChip>
          {likesLeft} из {likesTotal}
        </LimitChip>
      </div>
    </div>
  );
}

/* Демо-данные для вёрстки. */
export const DEMO_MATCHES: MatchRow[] = [
  { tone: 'both', title: 'Сова', context: 'вы оба поздно ложитесь' },
  { tone: 'both', title: 'Не курите', context: 'вы оба не курите' },
  { tone: 'both', title: 'Гости редко', context: 'вы оба зовёте гостей пару раз в месяц' },
  { tone: 'both', title: 'Тихо после 23', context: 'вы оба за тишину ночью' },
  { tone: 'both', title: 'Готовите дома', context: 'вы оба готовите почти каждый день' },
  { tone: 'both', title: 'Делите счета пополам', context: 'вы оба за равные платежи' },
  { tone: 'diff', title: 'Животные', context: 'у тебя кот — ей ок' },
  { tone: 'diff', title: 'Уборка', context: 'ты за график, она — по ситуации' },
];

export const DEMO_ABOUT =
  'Второй год в Москве, учусь на матфаке и работаю part-time. Дома тихо: сериалы в наушниках, готовлю почти каждый день. Ищу человека, с которым можно делить квартиру и не делить круг общения.';
