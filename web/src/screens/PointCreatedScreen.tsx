/* Успех создания точки — §6. Автор точки первым видит входящих. */
import type { CSSProperties } from 'react';
import { PhotoPlaceholder, S } from '../ui/primitives';

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
    padding: '28px 12px 20px',
  } as CSSProperties,
  check: {
    width: 60,
    height: 60,
    borderRadius: '50%',
    background: 'var(--verdict-ok-bg)',
    color: 'var(--verdict-ok-fg)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: 26,
    fontWeight: 700,
  } as CSSProperties,
  reviewBanner: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '9px 11px',
    borderRadius: 9,
    background: 'var(--warning-bg)',
    color: 'var(--warning-fg)',
    fontSize: 13,
    fontWeight: 590,
    lineHeight: 1.35,
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
};

export function PointCreatedScreen({
  title,
  pricePerPerson,
  onToSearches,
}: {
  title: string;
  pricePerPerson: string; // «45 000 ₽ с человека»
  onToSearches?: () => void;
}) {
  return (
    <div style={L.screen}>
      <div style={L.body}>
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
          <span style={L.check}>✓</span>
          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 5 }}>
            <span
              style={{ fontSize: 22, fontWeight: 640, letterSpacing: '-0.5px', textAlign: 'center' }}
            >
              Точка создана — ты первый в пуле
            </span>
            <span
              style={{
                fontSize: 14,
                color: 'var(--tg-theme-subtitle-text-color)',
                textAlign: 'center',
                textWrap: 'pretty',
              }}
            >
              Ты видишь всех входящих первым — пуш о каждом
            </span>
          </div>
        </div>

        <article style={S.card}>
          <div
            style={{ ...S.cardHeadBase, background: 'var(--type-flat-bg)', color: 'var(--type-flat-fg)' }}
          >
            <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>
              Квартира с рынка
            </span>
            <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>я добавил · сегодня</span>
          </div>
          <div style={{ ...S.cardBody, gap: 10 }}>
            <div style={{ display: 'flex', gap: 10 }}>
              <PhotoPlaceholder
                label="скрин"
                style={{ flex: '0 0 72px', height: 54, borderRadius: 8 }}
              />
              <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
                <span style={{ fontSize: 15, fontWeight: 590 }}>{title}</span>
                <span style={S.subtitle}>{pricePerPerson}</span>
                <span style={{ fontSize: 13, fontWeight: 510 }}>в пуле 1 — это ты</span>
              </div>
            </div>
            <div style={L.reviewBanner}>
              На быстрой проверке — в ленте у других появится после неё
            </div>
          </div>
        </article>
      </div>

      <div style={L.fixed}>
        {/* Шеринга до аппрува нет: ссылка на pending-точку у получателя не
            откроется (§5 — в общую ленту после проверки), кнопка создавала
            бы мёртвые приглашения. Кнопка «Позвать в чат» ждёт в «Моих
            поисках» — появится, как только точку одобрят. */}
        <span
          style={{
            fontSize: 13,
            color: 'var(--tg-theme-subtitle-text-color)',
            textAlign: 'center',
            textWrap: 'pretty',
          }}
        >
          Пройдёт проверку — позови людей в пул из «Моих поисков»
        </span>
        <button type="button" style={L.ghostButton} onClick={onToSearches}>
          К моим поискам
        </button>
      </div>
    </div>
  );
}
