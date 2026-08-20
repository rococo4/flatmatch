/* Общие примитивы слоя поверх Telegram UI (модель v2: поиски-подписки).
   Стили — только через CSS-переменные из styles/tokens.css.

   Единый скелет карточки:
     1) цветная шапка (teal/green — тип точки, pink — поиск) + мета справа
     2) субъект: фото+заголовок квартиры ИЛИ строка человека
     3) блок фактов label→value с заголовком секции
     4) блок «Привычки» (у карточек с лицом)
     5) счётчик пула
     6) кнопка: secondary «войти в пул» у точек, filled — только подписка на поиск
*/
import type { CSSProperties, ReactNode } from 'react';

/** Типы шапок. intent (заявка) отменён в v2 — см. §9 спеки. */
export type CardKind = 'flat' | 'intent_home' | 'search';

export const CARD_HEADER: Record<CardKind, { bg: string; fg: string }> = {
  flat: { bg: 'var(--type-flat-bg)', fg: 'var(--type-flat-fg)' },
  intent_home: { bg: 'var(--type-home-bg)', fg: 'var(--type-home-fg)' },
  search: { bg: 'var(--type-search-bg)', fg: 'var(--type-search-fg)' },
};

export const S = {
  card: {
    borderRadius: 14,
    overflow: 'hidden',
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  cardBody: { display: 'flex', flexDirection: 'column', gap: 12, padding: 12 } as CSSProperties,
  cardHeadBase: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 8,
    padding: '8px 12px',
  } as CSSProperties,
  block: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    padding: '10px 12px',
    borderRadius: 10,
    background: 'var(--app-bg)',
  } as CSSProperties,
  blockTitle: {
    fontSize: 11,
    fontWeight: 590,
    letterSpacing: '.05em',
    textTransform: 'uppercase',
    color: 'var(--tg-theme-hint-color)',
  } as CSSProperties,
  sectionTitle: {
    fontSize: 11,
    fontWeight: 590,
    letterSpacing: '.05em',
    color: 'var(--tg-theme-hint-color)',
  } as CSSProperties,
  row: {
    display: 'flex',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    gap: 12,
    fontSize: 14,
  } as CSSProperties,
  rowLabel: { color: 'var(--tg-theme-subtitle-text-color)', flex: '0 0 auto' } as CSSProperties,
  chip: {
    padding: '5px 9px',
    borderRadius: 8,
    background: 'var(--card-bg)',
    fontSize: 13,
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  title: { fontSize: 17, fontWeight: 590, letterSpacing: '-0.3px' } as CSSProperties,
  subtitle: { fontSize: 14, color: 'var(--tg-theme-subtitle-text-color)' } as CSSProperties,
  caption: {
    fontSize: 12,
    color: 'var(--tg-theme-hint-color)',
    textAlign: 'center',
  } as CSSProperties,
  secondaryButton: {
    width: '100%',
    padding: 12,
    border: 'none',
    borderRadius: 10,
    background: 'var(--app-bg)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  filledButton: {
    width: '100%',
    padding: 13,
    border: 'none',
    borderRadius: 10,
    background: 'var(--tg-theme-button-color)',
    font: 'inherit',
    fontSize: 16,
    fontWeight: 590,
    color: 'var(--tg-theme-button-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  dashedButton: {
    width: '100%',
    padding: '13px 12px',
    border: '1px dashed color-mix(in srgb, var(--tg-theme-button-color) 55%, transparent)',
    borderRadius: 12,
    background: 'transparent',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
};

export function Card({
  kind,
  title,
  meta,
  children,
}: {
  kind: CardKind;
  title: string;
  meta?: string;
  children: ReactNode;
}) {
  const h = CARD_HEADER[kind];
  return (
    <article style={S.card}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 8,
          padding: '8px 12px',
          background: h.bg,
          color: h.fg,
        }}
      >
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>{title}</span>
        {meta && <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>{meta}</span>}
      </div>
      <div style={S.cardBody}>{children}</div>
    </article>
  );
}

export type Fact = { label: string; value: ReactNode };

/** Структурный блок фактов — один формат у квартиры, комнаты и параметров поиска. */
export function FactsBlock({
  title,
  facts,
  children,
}: {
  title: string;
  facts: Fact[];
  children?: ReactNode; // фото над списком
}) {
  return (
    <div style={{ ...S.block, gap: 10 }}>
      <span style={S.blockTitle}>{title}</span>
      {children}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {facts.map((f) => (
          <div key={f.label} style={S.row}>
            <span style={S.rowLabel}>{f.label}</span>
            <span style={{ fontWeight: 510, textAlign: 'right' }}>{f.value}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

export function PerPerson({ children }: { children: ReactNode }) {
  return (
    <span style={{ fontWeight: 590 }}>
      {children}{' '}
      <span style={{ fontWeight: 400, color: 'var(--tg-theme-subtitle-text-color)' }}>
        с человека
      </span>
    </span>
  );
}

/** Ограничения собственника — серые чипы: их читают до лайка, а не после мэтча (§2.2). */
export function GreyChips({ items }: { items: string[] }) {
  if (items.length === 0) return null;
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingTop: 2 }}>
      {items.map((l) => (
        <span
          key={l}
          style={{
            padding: '4px 8px',
            borderRadius: 7,
            background: 'var(--chip-bg)',
            color: 'var(--tg-theme-subtitle-text-color)',
            fontSize: 12,
          }}
        >
          {l}
        </span>
      ))}
    </div>
  );
}

export function HabitsBlock({ items }: { items: string[] }) {
  return (
    <div style={S.block}>
      <span style={S.blockTitle}>Привычки</span>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
        {items.map((c) => (
          <span key={c} style={S.chip}>
            {c}
          </span>
        ))}
      </div>
    </div>
  );
}

export function PersonRow({
  name,
  age,
  occupation,
  verifiedBadge,
  photoUrl,
}: {
  name: string;
  age: number;
  /** Пусто — строки нет: занятость есть не в каждом ответе сервера. */
  occupation?: string;
  verifiedBadge?: string;
  /** Пусто = фото не одобрено: вместо него инициалы (§5). */
  photoUrl?: string;
}) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
      <Avatar name={name} size={48} src={photoUrl} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4, minWidth: 0 }}>
        <span style={S.title}>
          {name}, {age}
        </span>
        {(occupation || verifiedBadge) && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
            {occupation && (
              <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
                {occupation}
              </span>
            )}
            {verifiedBadge && <Badge tone="accent">{verifiedBadge}</Badge>}
          </div>
        )}
      </div>
    </div>
  );
}

/** Счётчик пула точки. hint = «увидишь после входа» у квартир с рынка. */
export function PoolCount({
  n,
  kind,
  hint,
  sep = '—',
}: {
  n: number;
  kind: CardKind;
  hint?: string;
  /** Разделитель перед подсказкой: тире у обещания, точка у состояния (§6). */
  sep?: string;
}) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 6,
        fontSize: 13,
        color: 'var(--tg-theme-subtitle-text-color)',
      }}
    >
      <span
        style={{
          width: 6,
          height: 6,
          borderRadius: '50%',
          background: CARD_HEADER[kind].fg,
          flex: '0 0 auto',
        }}
      />
      <span>
        <b style={{ color: 'var(--tg-theme-text-color)', fontWeight: 510 }}>в пуле {n}</b>
        {hint ? ` ${sep} ${hint}` : ''}
      </span>
    </div>
  );
}

/** Дешёвый лайк — только у точек §2.2. Всегда secondary и со словами. */
export function PoolEntryButton({
  label,
  caption,
  onClick,
}: {
  label: string;
  caption: string;
  onClick?: () => void;
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <button type="button" style={S.secondaryButton} onClick={onClick}>
        {label}
      </button>
      <span style={S.caption}>{caption}</span>
    </div>
  );
}

/** Вердикт параметра против моего поиска. Красного здесь нет by design. */
export type Verdict = 'ok' | 'edge' | 'no';

const VERDICTS: Record<Verdict, CSSProperties> = {
  ok: { background: 'var(--verdict-ok-bg)', color: 'var(--verdict-ok-fg)', fontWeight: 590 },
  edge: { background: 'var(--verdict-edge-bg)', color: 'var(--verdict-edge-fg)', fontWeight: 590 },
  no: { background: 'var(--verdict-no-bg)', color: 'var(--verdict-no-fg)' },
};

export function VerdictChip({ verdict, children }: { verdict: Verdict; children: ReactNode }) {
  return (
    <span style={{ padding: '4px 8px', borderRadius: 8, fontSize: 12, ...VERDICTS[verdict] }}>
      {verdict === 'ok' ? '✓ ' : ''}
      {children}
    </span>
  );
}

/** Жёлтый только здесь: лимиты («поиски заняты 3 из 3», «3 из 5») и плашки изменений точки. */
export function LimitChip({ children, small }: { children: ReactNode; small?: boolean }) {
  return (
    <span
      style={{
        padding: small ? '3px 8px' : '4px 9px',
        borderRadius: small ? 7 : 8,
        background: 'var(--warning-bg)',
        color: 'var(--warning-fg)',
        fontSize: small ? 12 : 13,
        fontWeight: 590,
      }}
    >
      {children}
    </span>
  );
}

/** Заглушка аватара: инициалы на пастельном круге, не иконка-человечек. */
const PASTELS = [
  ['#E9D9F2', '#4B3A5E'],
  ['#D6E6F5', '#33485E'],
  ['#F5E3D2', '#5E4632'],
  ['#DCEFE4', '#2F5140'],
  ['#F2DCDC', '#5E3838'],
];

function pastelOf(name: string) {
  let h = 0;
  for (const ch of name) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return PASTELS[h % PASTELS.length];
}

function initialsOf(name: string) {
  return name
    .split(/[\s,]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase())
    .join('');
}

/**
 * Аватар человека. Без `src` — инициалы на пастельном круге.
 *
 * Пустой `src` = фото не одобрено модератором, и заглушка здесь не «пока
 * грузится», а ровно то, что требует §5: до аппрува чужие глаза видят
 * заглушку. Адрес берётся как есть («/media/…»): собирать его — дело
 * хранилища, а не карточки.
 */
export function Avatar({ name, size = 48, src }: { name: string; size?: number; src?: string }) {
  const [bg, fg] = pastelOf(name);
  return (
    <div
      style={{
        flex: '0 0 auto',
        width: size,
        height: size,
        borderRadius: '50%',
        overflow: 'hidden',
        background: bg,
        color: fg,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: Math.round(size * 0.375),
        fontWeight: 590,
        letterSpacing: '-0.2px',
      }}
    >
      {src ? (
        <img
          src={src}
          alt=""
          style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
        />
      ) : (
        initialsOf(name)
      )}
    </div>
  );
}

/**
 * Стек аватаров в промо-блоке пула: кто уже ищет в зоне.
 *
 * Сервер шлёт URL фотографий, а не имена (в стек попадают только люди с
 * одобренным фото), поэтому `urls` главнее: когда он передан, кружки рисуются
 * фотографиями, а `names` остаётся запасным вариантом для вёрстки без сети.
 */
export function AvatarStack({
  names,
  urls,
  total,
  size = 34,
}: {
  names?: string[];
  urls?: string[];
  total: number;
  size?: number;
}) {
  const rest = total - (urls ?? names ?? []).length;
  const circle = (extra: CSSProperties): CSSProperties => ({
    width: size,
    height: size,
    borderRadius: '50%',
    border: '2px solid currentColor',
    borderColor: 'var(--stack-ring, var(--card-bg))',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: 13,
    fontWeight: 590,
    ...extra,
  });
  return (
    <div style={{ display: 'flex', alignItems: 'center' }}>
      {urls
        ? urls.map((url, i) => (
            // Ключ с индексом: URL могут повториться (одно и то же фото у
            // повторного человека в списке сервер не обещает исключать).
            <span
              key={`${url}:${i}`}
              style={circle({
                background: 'var(--chip-bg)',
                marginLeft: i ? -10 : 0,
                overflow: 'hidden',
              })}
            >
              <img
                src={url}
                alt=""
                style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
              />
            </span>
          ))
        : (names ?? []).map((n, i) => {
            const [bg, fg] = pastelOf(n);
            return (
              <span key={n} style={circle({ background: bg, color: fg, marginLeft: i ? -10 : 0 })}>
                {initialsOf(n)}
              </span>
            );
          })}
      {rest > 0 && (
        <span
          style={circle({
            background: 'var(--chip-bg)',
            color: 'var(--tg-theme-subtitle-text-color)',
            marginLeft: -10,
            fontSize: 12,
          })}
        >
          +{rest}
        </span>
      )}
    </div>
  );
}

export type BadgeTone = 'accent' | 'neutral' | 'home' | 'search';

const BADGE_TONES: Record<BadgeTone, CSSProperties> = {
  accent: {
    background: 'color-mix(in srgb, var(--tg-theme-button-color) 14%, transparent)',
    color: 'var(--tg-theme-accent-text-color)',
    fontWeight: 510,
  },
  neutral: { background: 'var(--chip-bg)', color: 'var(--tg-theme-subtitle-text-color)' },
  home: { background: 'var(--type-home-bg)', color: 'var(--type-home-fg)', fontWeight: 590 },
  search: { background: 'var(--type-search-bg)', color: 'var(--type-search-fg)', fontWeight: 590 },
};

export function Badge({ children, tone = 'neutral' }: { children: ReactNode; tone?: BadgeTone }) {
  return (
    <span style={{ padding: '2px 7px', borderRadius: 6, fontSize: 12, ...BADGE_TONES[tone] }}>
      {children}
    </span>
  );
}

export function PhotoPlaceholder({
  label,
  style,
  inCard,
}: {
  label: string;
  style?: CSSProperties;
  inCard?: boolean;
}) {
  return (
    <div className={`ph-photo${inCard ? ' ph-photo--in-card' : ''}`} style={style}>
      <span>{label}</span>
    </div>
  );
}
