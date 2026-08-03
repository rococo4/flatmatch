/* Зонный пул — §6. Открывается из промо-блока ленты или из «Моих поисков».
   Пул виртуален: жёсткая граница — только зона; цена/комнатность/даты —
   вердикт-чипы и три секции ранжирования, а не отсечка. */
import { useState } from 'react';
import type { CSSProperties } from 'react';
import { Avatar, Badge, S, VerdictChip, type Verdict } from '../ui/primitives';
import { TabBar } from '../ui/TabBar';

export type PoolPerson = {
  id: string;
  name: string;
  age: number;
  verifiedBadge?: string;
  habits: string; // «сова · не курю · без животных»
  matches: number; // «N совпадений анкеты»
  verdicts: { verdict: Verdict; label: string }[];
};

export type PoolSection = {
  id: 'all' | 'partial' | 'none';
  title: string; // «СОВПАЛИ ПО ВСЕМ ПАРАМЕТРАМ · 2»
  people: PoolPerson[];
  dimmed?: boolean;
  collapsedAfter?: number; // свёрнуто после N карточек, остальное под «ещё K»
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
  savedBanner: {
    display: 'flex',
    flexDirection: 'column',
    gap: 3,
    padding: '11px 12px',
    borderRadius: 12,
    background: 'var(--saved-card-bg)',
    border: '1px solid var(--saved-card-border)',
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
};

function PersonMiniCard({ p, onOpen }: { p: PoolPerson; onOpen?: (id: string) => void }) {
  return (
    <div style={L.personCard} onClick={() => onOpen?.(p.id)}>
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
  );
}

function Section({ s, onOpen }: { s: PoolSection; onOpen?: (id: string) => void }) {
  const [expanded, setExpanded] = useState(false);
  const limit = s.collapsedAfter && !expanded ? s.collapsedAfter : s.people.length;
  const shown = s.people.slice(0, limit);
  const rest = s.people.length - shown.length;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8, opacity: s.dimmed ? 0.6 : 1 }}>
      <span style={S.sectionTitle}>{s.title}</span>
      {shown.map((p) => (
        <PersonMiniCard key={p.id} p={p} onOpen={onOpen} />
      ))}
      {rest > 0 && (
        <button type="button" style={L.moreButton} onClick={() => setExpanded(true)}>
          ещё {rest}
        </button>
      )}
    </div>
  );
}

export function ZonePoolScreen({
  zone,
  zoneGen,
  sections,
  pointsCount,
  unreadLikes,
  onBack,
  onOpenPerson,
  onOpenPoints,
}: {
  zone: string; // «Хамовники»
  zoneGen: string; // «Хамовников»
  sections: PoolSection[];
  /** Сколько точек подходит под те же параметры. */
  pointsCount?: number;
  unreadLikes: number;
  onBack?: () => void;
  onOpenPerson?: (id: string) => void;
  onOpenPoints?: () => void;
}) {
  return (
    <div style={L.screen}>
      <div style={L.header}>
        <span
          style={{ fontSize: 22, lineHeight: 1, color: 'var(--tg-theme-accent-text-color)', cursor: 'pointer' }}
          onClick={onBack}
        >
          ‹
        </span>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>{zone}</span>
      </div>

      <div style={L.body}>
        <div style={L.savedBanner}>
          <span style={{ fontSize: 14, fontWeight: 590, color: 'var(--type-home-fg)', lineHeight: 1.35 }}>
            Ты в пуле {zoneGen} · поиск сохранён
          </span>
          <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
            пуш о каждом новом совпавшем
          </span>
        </div>

        {sections.map((s) => (
          <Section key={s.id} s={s} onOpen={onOpenPerson} />
        ))}

        {/* Тихая ссылка в выдачу точек под те же параметры — не кнопка. */}
        {typeof pointsCount === 'number' && (
          <div
            onClick={onOpenPoints}
            style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '4px 2px', cursor: 'pointer' }}
          >
            <span
              style={{
                flex: '1 1 auto',
                fontSize: 14,
                fontWeight: 510,
                color: 'var(--tg-theme-accent-text-color)',
                minWidth: 0,
              }}
            >
              Квартиры под этот поиск · {pointsCount}
            </span>
            <span style={{ flex: '0 0 auto', fontSize: 17, lineHeight: 1, color: 'var(--tg-theme-hint-color)' }}>
              ›
            </span>
          </div>
        )}
      </div>

      <TabBar active="feed" unreadLikes={unreadLikes} hasNewInSearches />
    </div>
  );
}

/* Демо-данные для вёрстки. */
export const DEMO_POOL: PoolSection[] = [
  {
    id: 'all',
    title: 'СОВПАЛИ ПО ВСЕМ ПАРАМЕТРАМ · 2',
    people: [
      {
        id: 'u1',
        name: 'Аня',
        age: 22,
        verifiedBadge: 'Студент ВШЭ',
        habits: 'сова · не курю · без животных',
        matches: 6,
        verdicts: [
          { verdict: 'ok', label: '40–45к' },
          { verdict: 'ok', label: '2 комн.' },
          { verdict: 'ok', label: 'сентябрь' },
        ],
      },
      {
        id: 'u2',
        name: 'Даша',
        age: 24,
        verifiedBadge: 'Студент МГУ',
        habits: 'плавает · не курю · кот',
        matches: 5,
        verdicts: [
          { verdict: 'ok', label: '38–44к' },
          { verdict: 'ok', label: '2 комн.' },
          { verdict: 'ok', label: 'год+' },
        ],
      },
    ],
  },
  {
    id: 'partial',
    title: 'ПЕРЕСЕКАЮТСЯ ЧАСТИЧНО · 2',
    people: [
      {
        id: 'u3',
        name: 'Иван',
        age: 25,
        habits: 'жаворонок · балкон · без животных',
        matches: 3,
        verdicts: [
          { verdict: 'edge', label: 'от 45к — краем' },
          { verdict: 'ok', label: '2 комн.' },
          { verdict: 'no', label: 'октябрь' },
        ],
      },
      {
        // Цена не указана = «неважно», совпадает со всем (§2.1):
        // в «частично» человек из-за мягкой комнатности, а не из-за NULL-цены.
        id: 'u5',
        name: 'Пётр',
        age: 31,
        habits: 'жаворонок · не курю · собака',
        matches: 4,
        verdicts: [
          { verdict: 'no', label: 'цена не указана' },
          { verdict: 'ok', label: 'сентябрь' },
          { verdict: 'no', label: '3 комн.' },
        ],
      },
    ],
  },
  {
    id: 'none',
    title: 'НЕ СОВПАЛИ · 2',
    dimmed: true,
    collapsedAfter: 1,
    people: [
      {
        id: 'u4',
        name: 'Настя',
        age: 29,
        habits: 'сова · дома · собака',
        matches: 2,
        verdicts: [
          { verdict: 'no', label: '60–80к' },
          { verdict: 'no', label: '1 комн.' },
          { verdict: 'no', label: 'полгода+' },
        ],
      },
      {
        id: 'u6',
        name: 'Лена',
        age: 23,
        habits: 'сова · балкон · кот',
        matches: 2,
        verdicts: [
          { verdict: 'no', label: '20–30к' },
          { verdict: 'no', label: 'ноябрь' },
        ],
      },
    ],
  },
];
