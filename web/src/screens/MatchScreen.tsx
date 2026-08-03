/* Мэтч — §6. Filled-кнопка здесь ведёт в личку: дорогой выбор уже сделан
   обоими, дешёвых действий на экране нет. */
import type { CSSProperties } from 'react';
import { S } from '../ui/primitives';

const L = {
  screen: {
    display: 'flex',
    flexDirection: 'column',
    minHeight: '100%',
    background: 'var(--app-bg)',
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  body: {
    flex: '1 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: '24px 12px 20px',
  } as CSSProperties,
  block: {
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    padding: '14px 12px',
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  chip: {
    padding: '5px 10px',
    borderRadius: 9,
    background: 'var(--verdict-ok-bg)',
    color: 'var(--verdict-ok-fg)',
    fontSize: 13,
    fontWeight: 590,
  } as CSSProperties,
  subhead: {
    fontSize: 11,
    fontWeight: 590,
    letterSpacing: '.05em',
    color: 'var(--tg-theme-hint-color)',
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
  /** На мэтче дорогой выбор уже сделан обоими — filled здесь не нужен. */
  primaryButton: {
    width: '100%',
    padding: 14,
    border: 'none',
    borderRadius: 11,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 16,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  ghostButton: {
    width: '100%',
    padding: 12,
    border: 'none',
    borderRadius: 11,
    background: 'transparent',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  pairCircle: (bg: string, fg: string, overlap: boolean): CSSProperties => ({
    width: 76,
    height: 76,
    borderRadius: '50%',
    background: bg,
    color: fg,
    border: '3px solid var(--app-bg)',
    marginLeft: overlap ? -18 : 0,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: 27,
    fontWeight: 590,
  }),
};

export function MatchScreen({
  name,
  username,
  where,
  params,
  profileMatches,
  nextSteps,
  onWrite,
  onToFeed,
}: {
  name: string;
  username: string; // без @
  /** ГДЕ: «пересеклись в пуле Хамовников» / «на комнату Кати · м. Сокол» */
  where: string;
  /** ПО ПАРАМЕТРАМ: бюджет / заезд / комнаты */
  params: { label: string; value: string }[];
  /** ПО АНКЕТЕ: зелёные чипы «вы оба совы» */
  profileMatches: string[];
  nextSteps: string[];
  onWrite?: () => void;
  onToFeed?: () => void;
}) {
  const initials = name.slice(0, 2).toUpperCase();

  return (
    <div style={L.screen}>
      <div style={L.body}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 14 }}>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <span style={L.pairCircle('#D6E6F5', '#33485E', false)}>ТЫ</span>
            <span style={L.pairCircle('#E9D9F2', '#4B3A5E', true)}>{initials}</span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 5 }}>
            <span style={{ fontSize: 26, fontWeight: 700, letterSpacing: '-0.6px' }}>Это мэтч</span>
            <span
              style={{
                fontSize: 15,
                color: 'var(--tg-theme-subtitle-text-color)',
                textAlign: 'center',
                textWrap: 'pretty',
              }}
            >
              Вы оба выбрали жить вместе. Дальше — договориться в личке.
            </span>
          </div>
        </div>

        <div style={L.block}>
          <span style={S.blockTitle}>Почему вы совпали</span>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
            <span style={L.subhead}>ГДЕ</span>
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 9,
                padding: '10px 11px',
                borderRadius: 10,
                background: 'var(--app-bg)',
              }}
            >
              <span
                style={{
                  flex: '0 0 auto',
                  width: 7,
                  height: 7,
                  borderRadius: '50%',
                  background: 'var(--type-search-fg)',
                }}
              />
              <span style={{ fontSize: 14, lineHeight: 1.35 }}>{where}</span>
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
            <span style={L.subhead}>ПО ПАРАМЕТРАМ</span>
            <div
              style={{
                display: 'flex',
                flexDirection: 'column',
                gap: 8,
                padding: '10px 12px',
                borderRadius: 10,
                background: 'var(--app-bg)',
              }}
            >
              {params.map((p) => (
                <div
                  key={p.label}
                  style={{
                    display: 'flex',
                    alignItems: 'baseline',
                    justifyContent: 'space-between',
                    gap: 12,
                    fontSize: 14,
                  }}
                >
                  <span style={{ color: 'var(--tg-theme-subtitle-text-color)', flex: '0 0 auto' }}>
                    {p.label}
                  </span>
                  <span style={{ fontWeight: 510, textAlign: 'right' }}>{p.value}</span>
                </div>
              ))}
            </div>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
            <span style={L.subhead}>ПО АНКЕТЕ</span>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {profileMatches.map((r) => (
                <span key={r} style={L.chip}>
                  {r}
                </span>
              ))}
            </div>
          </div>
        </div>

        <div style={L.block}>
          <span style={S.blockTitle}>Что дальше</span>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 7, fontSize: 14, lineHeight: 1.4 }}>
            {nextSteps.map((s, i) => (
              <div key={s} style={{ display: 'flex', gap: 9 }}>
                <span style={{ color: 'var(--tg-theme-hint-color)', fontVariantNumeric: 'tabular-nums' }}>
                  {i + 1}
                </span>
                <span>{s}</span>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div style={L.fixed}>
        <button type="button" style={L.primaryButton} onClick={onWrite}>
          Написать @{username}
        </button>
        <button type="button" style={L.ghostButton} onClick={onToFeed}>
          К ленте
        </button>
      </div>
    </div>
  );
}

/** ГДЕ — одной строкой; для мэтча на комнату: «на комнату Кати · м. Сокол». */
export const DEMO_MATCH_WHERE = 'пересеклись в пуле Хамовников';

export const DEMO_MATCH_PARAMS = [
  { label: 'Бюджет', value: 'у обоих 40–45к' },
  { label: 'Заезд', value: 'сентябрь у обоих' },
  { label: 'Комнаты', value: '2 у обоих' },
];

export const DEMO_MATCH_PROFILE = [
  'вы оба совы',
  'оба не курите',
  'гости редко у обоих',
  'одинаково про уборку',
];

export const DEMO_NEXT_STEPS = [
  'Списаться и созвониться',
  'Вместе посмотреть квартиры из пула',
  'Договориться о деньгах до просмотра',
];
