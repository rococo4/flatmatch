/* Пул квартиры — §6 «Карточка квартиры».
   Список лайкнувших с совпадениями анкеты и вердикт-чипами (если у смотрящего
   есть поиск) + красный выход из пула с подтверждением (см. EdgeStates). */
import { useState } from 'react';
import type { CSSProperties } from 'react';
import { Avatar, Badge, S, VerdictChip, type Verdict } from '../ui/primitives';

export type FlatPoolPerson = {
  id: string;
  name: string;
  age: number;
  verifiedBadge?: string;
  habits: string;
  matches: number;
  verdicts: { verdict: Verdict; label: string }[];
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
    gap: 16,
    padding: '12px 12px 20px',
  } as CSSProperties,
  personCard: {
    display: 'flex',
    gap: 12,
    padding: 12,
    borderRadius: 12,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
    cursor: 'pointer',
  } as CSSProperties,
  moreButton: {
    width: '100%',
    padding: 11,
    border: 'none',
    borderRadius: 10,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 14,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  /** Красная кнопка выхода стоит прямо на полотне, поэтому своя поверхность
   *  и рамка: --app-bg совпал бы с фоном экрана в обеих темах. */
  leaveButton: {
    width: '100%',
    padding: 13,
    border: '1px solid color-mix(in srgb, var(--tg-theme-destructive-text-color) 30%, transparent)',
    borderRadius: 11,
    background: 'color-mix(in srgb, var(--tg-theme-destructive-text-color) 10%, var(--card-bg))',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-destructive-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
};

export function FlatPoolScreen({
  flat,
  people,
  poolCount,
  shownCount = 3,
  onBack,
  onOpenSource,
  onOpenPerson,
  onLeave,
}: {
  flat: {
    title: string;
    pricePerPerson: string;
    metro: string;
    source: string; // «Авито»
    postedAt: string; // «27 июля»
    addedAt: string; // «добавлено 20 июля»
  };
  people: FlatPoolPerson[];
  poolCount: number;
  shownCount?: number;
  onBack?: () => void;
  onOpenSource?: () => void;
  onOpenPerson?: (id: string) => void;
  onLeave?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const shown = expanded ? people : people.slice(0, shownCount);
  const rest = poolCount - shown.length;

  return (
    <div style={L.screen}>
      <div style={L.header}>
        <span
          style={{ fontSize: 22, lineHeight: 1, color: 'var(--tg-theme-accent-text-color)', cursor: 'pointer' }}
          onClick={onBack}
        >
          ‹
        </span>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Пул квартиры</span>
      </div>

      <div style={L.body}>
        <article style={S.card}>
          <div style={{ ...S.cardHeadBase, background: 'var(--type-flat-bg)', color: 'var(--type-flat-fg)' }}>
            <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>Квартира с рынка</span>
            <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>
              {flat.source} · {flat.postedAt}
            </span>
          </div>
          <div style={{ ...S.cardBody, gap: 10 }}>
            <div style={{ display: 'flex', gap: 10 }}>
              <div
                className="ph-photo"
                style={{ flex: '0 0 84px', height: 63, borderRadius: 8 }}
              >
                <span>скрин</span>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
                <span style={{ fontSize: 16, fontWeight: 590, letterSpacing: '-0.2px' }}>{flat.title}</span>
                <span style={{ fontSize: 15, fontWeight: 590 }}>{flat.pricePerPerson}</span>
                <span style={S.subtitle}>{flat.metro}</span>
              </div>
            </div>
            <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
              <button
                type="button"
                onClick={onOpenSource}
                style={{
                  border: 'none',
                  background: 'none',
                  font: 'inherit',
                  fontSize: 13,
                  fontWeight: 510,
                  color: 'var(--tg-theme-accent-text-color)',
                  cursor: 'pointer',
                }}
              >
                Открыть на {flat.source} ↗
              </button>
              <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>{flat.addedAt}</span>
            </div>
          </div>
        </article>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <span style={S.sectionTitle}>В ПУЛЕ · {poolCount} · ХОТЯТ СНЯТЬ ЭТУ КВАРТИРУ</span>

          {shown.map((p) => (
            <div key={p.id} style={L.personCard} onClick={() => onOpenPerson?.(p.id)}>
              <Avatar name={p.name} size={48} />
              <div style={{ flex: '1 1 auto', display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                  <span style={{ fontSize: 16, fontWeight: 590, letterSpacing: '-0.2px' }}>
                    {p.name}, {p.age}
                  </span>
                  {p.verifiedBadge && <Badge tone="accent">{p.verifiedBadge}</Badge>}
                </div>
                <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>{p.habits}</span>
                <span style={{ fontSize: 13, fontWeight: 510 }}>{p.matches} совпадений анкеты</span>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingTop: 2 }}>
                  {p.verdicts.map((v) => (
                    <VerdictChip key={v.label} verdict={v.verdict}>
                      {v.label}
                    </VerdictChip>
                  ))}
                </div>
              </div>
              <span
                style={{ flex: '0 0 auto', alignSelf: 'center', fontSize: 20, color: 'var(--tg-theme-hint-color)' }}
              >
                ›
              </span>
            </div>
          ))}

          {rest > 0 && (
            <button type="button" style={L.moreButton} onClick={() => setExpanded(true)}>
              ещё {rest}
            </button>
          )}
        </div>

        <div style={{ paddingTop: 4 }}>
          <button type="button" style={L.leaveButton} onClick={onLeave}>
            Выйти из пула
          </button>
        </div>
      </div>
    </div>
  );
}

export const DEMO_FLAT_POOL: FlatPoolPerson[] = [
  {
    id: 'u1',
    name: 'Аня',
    age: 22,
    verifiedBadge: 'Студент ВШЭ',
    habits: 'сова · не курю · без животных',
    matches: 6,
    verdicts: [
      { verdict: 'ok', label: '40–45к' },
      { verdict: 'ok', label: 'сентябрь' },
    ],
  },
  {
    id: 'u3',
    name: 'Иван',
    age: 25,
    habits: 'жаворонок · балкон · без животных',
    matches: 3,
    verdicts: [
      { verdict: 'edge', label: 'от 45к — краем' },
      { verdict: 'no', label: 'октябрь' },
    ],
  },
  {
    // Цена не указана — параметр «неважно», совпадает со всем (§2.1).
    id: 'u4',
    name: 'Настя',
    age: 29,
    habits: 'сова · дома · собака',
    matches: 2,
    verdicts: [
      { verdict: 'no', label: 'цена не указана' },
      { verdict: 'ok', label: 'сентябрь' },
    ],
  },
];
