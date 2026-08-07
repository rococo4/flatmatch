/* Вход в ленту без активного поиска (§6, шаг 1).

   Search-first: без поиска показывать нечего, поэтому экран — один вопрос и
   одна кнопка. Тизер ниже честно пустой: точек в базе пока нет (§12 шаг 5),
   и подставлять сюда демо-квартиру нельзя — пустое состояние это тоже
   состояние. */
import type { CSSProperties } from 'react';
import { S } from '../ui/primitives';
import { PointsSoon } from './FeedScreen';

const E = {
  screen: {
    display: 'flex',
    flexDirection: 'column',
    gap: 18,
    minHeight: '100%',
    width: '100%',
    // Потолок ширины — для десктопа, как во Frame онбординга.
    maxWidth: 520,
    margin: '0 auto',
    padding: '18px 14px 20px',
    background: 'var(--app-bg)',
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  card: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: '18px 16px 20px',
    borderRadius: 18,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  h1: { fontSize: 23, fontWeight: 640, letterSpacing: '-0.5px', textWrap: 'pretty' } as CSSProperties,
};

export function FeedEntryScreen({ onSearch }: { onSearch?: () => void }) {
  return (
    <div style={E.screen}>
      <div style={E.card}>
        <span style={E.h1}>С кем и где будешь жить?</span>
        <button type="button" style={S.filledButton} onClick={onSearch}>
          Найти
        </button>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        <span style={S.sectionTitle}>СВЕЖЕЕ В МОСКВЕ</span>
        <PointsSoon />
      </div>
    </div>
  );
}
