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
    overflow: 'hidden',
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
  age,
  photoUrl,
  where,
  params,
  profileMatches,
  contact,
  onWrite,
  onToFeed,
}: {
  name: string;
  age: number;
  /** Пусто = фото не одобрено: в кружке пары остаются инициалы (§5). */
  photoUrl?: string;
  /** ГДЕ: «пересеклись в пуле Хамовников». Пусто — контекста не осталось. */
  where?: string;
  /** ПО ПАРАМЕТРАМ: бюджет / комнаты / заезд / срок — что пересеклось */
  params: { label: string; value: string }[];
  /** ПО АНКЕТЕ: зелёные чипы совпавших привычек */
  profileMatches: string[];
  /**
   * Единственный способ связаться во всём продукте (§3).
   *
   * null — контакт скрыт (бан пира или блок пары): кнопки тогда нет вовсе,
   * а не мёртвая ссылка в никуда.
   */
  contact: { label: string; hint?: string } | null;
  onWrite?: () => void;
  onToFeed?: () => void;
}) {
  const initials = name.slice(0, 2).toUpperCase();
  // Пустое саммари — не пустая карточка: у ответного лайка нет пула, а без
  // активных поисков у пары нечего сказать и про параметры.
  const hasSummary = Boolean(where) || params.length > 0 || profileMatches.length > 0;

  return (
    <div style={L.screen}>
      <div style={L.body}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 14 }}>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <span style={L.pairCircle('#D6E6F5', '#33485E', false)}>ТЫ</span>
            <span style={L.pairCircle('#E9D9F2', '#4B3A5E', true)}>
              {photoUrl ? (
                <img
                  src={photoUrl}
                  alt=""
                  style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
                />
              ) : (
                initials
              )}
            </span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 5 }}>
            <span style={{ fontSize: 26, fontWeight: 700, letterSpacing: '-0.6px' }}>Это мэтч</span>
            {/* Имя в именительном: экран открывается и из «Моих мэтчей», где
                без него не понять, с кем именно совпали. Контакт может быть
                скрыт — тогда это единственное, что называет человека. */}
            <span style={{ fontSize: 17, fontWeight: 590 }}>
              {name}, {age}
            </span>
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

        {hasSummary && (
          <div style={L.block}>
            <span style={S.blockTitle}>Почему вы совпали</span>

            {where && (
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
            )}

            {params.length > 0 && (
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
            )}

            {profileMatches.length > 0 && (
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
            )}
          </div>
        )}
      </div>

      <div style={L.fixed}>
        {contact ? (
          <>
            <button type="button" style={L.primaryButton} onClick={onWrite}>
              {contact.label}
            </button>
            {contact.hint && <span style={S.caption}>{contact.hint}</span>}
          </>
        ) : (
          <span style={S.caption}>Контакт недоступен</span>
        )}
        <button type="button" style={L.ghostButton} onClick={onToFeed}>
          К ленте
        </button>
      </div>
    </div>
  );
}
