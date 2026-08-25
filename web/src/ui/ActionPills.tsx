/* Две кнопки-иконки под карточкой: стрелка — пропустить, сердце — лайк (§6).

   Один компонент на подбор и на анкету лайкнувшего: жест там и там один и тот
   же, и разъехавшиеся кнопки читались бы как разные действия. Точки остатка
   рисует только подбор — ответный лайк лимита не тратит (§2.3). */
import type { CSSProperties } from 'react';

const pill: CSSProperties = {
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  height: 54,
  borderRadius: 16,
  border: 'none',
  font: 'inherit',
  cursor: 'pointer',
};

export function ActionPills({
  likeLabel,
  likeLocked,
  onPass,
  onLike,
}: {
  /** Что обещает сердце — для голоса экрана: «Хочу эту комнату», «Взаимно». */
  likeLabel: string;
  /** Лайк недоступен до одобрения анкеты: сердце серое, стрелка работает (§5). */
  likeLocked?: boolean;
  onPass?: () => void;
  onLike?: () => void;
}) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
      <button
        type="button"
        aria-label="Пропустить"
        onClick={onPass}
        style={{
          ...pill,
          width: 88,
          background: 'var(--card-bg)',
          border: '1px solid var(--sep)',
          color: 'var(--tg-theme-subtitle-text-color)',
        }}
      >
        <ArrowIcon />
      </button>
      <button
        type="button"
        aria-label={likeLabel}
        disabled={likeLocked}
        onClick={onLike}
        style={{
          ...pill,
          width: 132,
          background: likeLocked ? 'var(--chip-bg)' : 'var(--tg-theme-button-color)',
          color: likeLocked ? 'var(--tg-theme-hint-color)' : 'var(--tg-theme-button-text-color)',
          boxShadow: likeLocked
            ? 'none'
            : '0 6px 16px color-mix(in srgb, var(--tg-theme-button-color) 35%, transparent)',
          cursor: likeLocked ? 'default' : 'pointer',
        }}
      >
        <HeartIcon />
      </button>
    </div>
  );
}

/** Стрелка вправо: «пропустить, дальше». Та же линия, что у жеста влево. */
export function ArrowIcon() {
  return (
    <span style={{ display: 'flex', alignItems: 'center', width: 24, height: 10 }}>
      <span style={{ flex: '1 1 auto', height: 2, borderRadius: 1, background: 'currentColor' }} />
      <span
        style={{
          flex: '0 0 auto',
          width: 9,
          height: 9,
          marginLeft: -7,
          borderTop: '2px solid currentColor',
          borderRight: '2px solid currentColor',
          transform: 'rotate(45deg)',
        }}
      />
    </span>
  );
}

export function HeartIcon() {
  const lobe: CSSProperties = {
    position: 'absolute',
    width: 13,
    height: 13,
    borderRadius: '50%',
    background: 'currentColor',
  };
  return (
    <span style={{ position: 'relative', width: 24, height: 22, display: 'block' }}>
      <span style={{ ...lobe, left: 0, top: 1 }} />
      <span style={{ ...lobe, right: 0, top: 1 }} />
      <span
        style={{
          position: 'absolute',
          left: 3,
          top: 5,
          width: 18,
          height: 18,
          background: 'currentColor',
          borderRadius: '3px 0 4px 0',
          transform: 'rotate(45deg)',
          transformOrigin: '50% 50%',
        }}
      />
    </span>
  );
}
