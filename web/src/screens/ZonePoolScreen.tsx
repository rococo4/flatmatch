/* Зонный пул — §6. Открывается из промо-блока ленты или из «Моих поисков».
   Пул виртуален: жёсткая граница — только зона; цена/комнатность/даты —
   вердикт-чипы и три секции ранжирования, а не отсечка. */
import { useState } from 'react';
import type { CSSProperties } from 'react';
import { plural } from '../api/format';
import { Avatar, Badge, S, VerdictChip, type Verdict } from '../ui/primitives';

export type PoolPerson = {
  id: string;
  name: string;
  age: number;
  verifiedBadge?: string;
  habits: string; // «сова · не курю · без животных»
  matches: number; // «N совпадений анкеты»
  /** «ищет на красной ветке» — точный сосед и веточник это разные намерения. */
  searchZone?: string;
  verdicts: { verdict: Verdict; label: string }[];
  /** Пусто = фото не одобрено: вместо него инициалы, как требует §5. */
  photoUrl?: string;
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
      <Avatar name={p.name} size={48} src={p.photoUrl} />
      <div style={{ flex: '1 1 auto', display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
          <span style={{ fontSize: 16, fontWeight: 590, letterSpacing: '-0.2px' }}>
            {p.name}, {p.age}
          </span>
          {p.verifiedBadge && <Badge tone="accent">{p.verifiedBadge}</Badge>}
        </div>
        <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>{p.habits}</span>
        {p.searchZone && (
          <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>{p.searchZone}</span>
        )}
        <span style={{ fontSize: 13, fontWeight: 510 }}>
          {p.matches} {plural(p.matches, 'совпадение', 'совпадения', 'совпадений')} анкеты
        </span>
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

/**
 * Подсказка плотности — один компонент на три состояния (§6).
 *
 * Молчит в норме: совет «уточни параметры» на десяти людях был бы вредным,
 * а «подпишись первым» при непустом пуле — ложью.
 */
function DensityHint({ total, matched }: { total: number; matched: number }) {
  const text =
    total === 0
      ? 'подпишись первым — пришлём пуш'
      : matched > 50
        ? 'уточни бюджет или сузь зону — список станет точнее'
        : null;
  if (!text) return null;
  return (
    <div
      style={{
        padding: '10px 12px',
        borderRadius: 10,
        background: 'var(--chip-bg)',
        fontSize: 13,
        lineHeight: 1.4,
        color: 'var(--tg-theme-subtitle-text-color)',
      }}
    >
      {text}
    </div>
  );
}

export function ZonePoolScreen({
  zone,
  zoneGen,
  saved,
  sections,
  pointsCount,
  onBack,
  onOpenPerson,
  onOpenPoints,
}: {
  zone: string; // «Хамовники»
  zoneGen: string; // «Хамовников»
  /**
   * Есть ли у смотрящего поиск ровно этой зоны. Ранжирование довольствуется
   * поиском соседней пересекающейся зоны, поэтому чипы бывают и без подписки —
   * и плашка «Ты в пуле» тогда соврала бы.
   */
  saved: boolean;
  sections: PoolSection[];
  /** Сколько точек подходит под те же параметры. */
  pointsCount?: number;
  onBack?: () => void;
  onOpenPerson?: (id: string) => void;
  onOpenPoints?: () => void;
}) {
  const total = sections.reduce((n, s) => n + s.people.length, 0);
  const matched = sections.find((s) => s.id === 'all')?.people.length ?? 0;
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
        {saved && (
          <div style={L.savedBanner}>
            <span style={{ fontSize: 14, fontWeight: 590, color: 'var(--type-home-fg)', lineHeight: 1.35 }}>
              Ты в пуле {zoneGen} · поиск сохранён
            </span>
            <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
              пуш о каждом новом совпавшем
            </span>
          </div>
        )}

        <DensityHint total={total} matched={matched} />

        {sections.map((s) => (
          <Section key={s.id} s={s} onOpen={onOpenPerson} />
        ))}

        {/* Тихая ссылка в выдачу точек под те же параметры — не кнопка.
            На нуле скрыта целиком: «· 0» вело бы в пустоту. */}
        {!!pointsCount && (
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
    </div>
  );
}
