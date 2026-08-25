/* Таб-бар: Соседи / Лайки / Поиски / Профиль (§6).
   Бейдж непросмотренных лайков — на «Лайках», бейдж новизны — на «Поисках».
   Иконки — простые геометрические примитивы, без иллюстраций. */
import type { CSSProperties } from 'react';

export type Tab = 'deck' | 'likes' | 'searches' | 'profile';

const item = (active: boolean): CSSProperties => ({
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'center',
  gap: 5,
  padding: '9px 0 5px',
  color: active ? 'var(--tg-theme-accent-text-color)' : 'var(--tg-theme-hint-color)',
  cursor: 'pointer',
  background: 'none',
  border: 'none',
  font: 'inherit',
});

const label: CSSProperties = { fontSize: 11, fontWeight: 510 };

const icon: CSSProperties = {
  position: 'relative',
  width: 22,
  height: 22,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
};

/** Двое рядом: подбор — это люди, а не список объявлений. */
function PeopleIcon() {
  const head: CSSProperties = { width: 8, height: 8, border: '2.5px solid currentColor', borderRadius: '50%' };
  const body: CSSProperties = {
    width: 14,
    height: 7,
    border: '2.5px solid currentColor',
    borderBottom: 'none',
    borderRadius: '8px 8px 0 0',
  };
  return (
    <div style={{ ...icon, alignItems: 'flex-end', gap: 2 }}>
      <span style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 1, opacity: 0.55 }}>
        <span style={{ ...head, width: 6, height: 6 }} />
        <span style={{ ...body, width: 11, height: 6 }} />
      </span>
      <span style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 1 }}>
        <span style={head} />
        <span style={body} />
      </span>
    </div>
  );
}

/** Сердце — два круга и квадрат под углом: тот же жест, что на карточке. */
function HeartIcon({ badge }: { badge?: number }) {
  const lobe: CSSProperties = {
    position: 'absolute',
    width: 11,
    height: 11,
    borderRadius: '50%',
    border: '2.5px solid currentColor',
    borderBottomColor: 'transparent',
    borderRightColor: 'transparent',
  };
  return (
    <div style={icon}>
      <span
        style={{
          position: 'relative',
          width: 20,
          height: 18,
          transform: 'rotate(-45deg)',
          display: 'block',
        }}
      >
        <span
          style={{
            position: 'absolute',
            left: 2,
            bottom: 1,
            width: 12,
            height: 12,
            border: '2.5px solid currentColor',
            borderTopColor: 'transparent',
            borderRightColor: 'transparent',
          }}
        />
        <span style={{ ...lobe, left: 1.5, top: 0.5, transform: 'rotate(45deg)' }} />
        <span style={{ ...lobe, left: 8, bottom: 0.5, transform: 'rotate(135deg)' }} />
      </span>
      {!!badge && badge > 0 && <CountBadge n={badge} />}
    </div>
  );
}

function SearchesIcon({ dot }: { dot?: boolean }) {
  return (
    <div style={icon}>
      <span style={{ width: 15, height: 15, border: '2.5px solid currentColor', borderRadius: '50%' }} />
      {dot && (
        <span
          style={{
            position: 'absolute',
            top: -3,
            right: -6,
            width: 8,
            height: 8,
            borderRadius: 4,
            background: 'var(--tg-theme-destructive-text-color)',
          }}
        />
      )}
    </div>
  );
}

function ProfileIcon() {
  return (
    <div style={{ ...icon, flexDirection: 'column', gap: 1 }}>
      <span style={{ width: 9, height: 9, border: '2.5px solid currentColor', borderRadius: '50%' }} />
      <span
        style={{
          width: 16,
          height: 8,
          border: '2.5px solid currentColor',
          borderBottom: 'none',
          borderRadius: '9px 9px 0 0',
        }}
      />
    </div>
  );
}

function CountBadge({ n }: { n: number }) {
  return (
    <span
      style={{
        position: 'absolute',
        top: -4,
        right: -9,
        minWidth: 16,
        height: 16,
        padding: '0 4px',
        borderRadius: 8,
        background: 'var(--tg-theme-destructive-text-color)',
        color: '#fff',
        fontSize: 11,
        fontWeight: 640,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        boxSizing: 'border-box',
      }}
    >
      {n}
    </span>
  );
}

export function TabBar({
  active,
  unreadLikes = 0,
  hasNewInSearches,
  onChange,
}: {
  active: Tab;
  unreadLikes?: number;
  hasNewInSearches?: boolean;
  onChange?: (tab: Tab) => void;
}) {
  return (
    <div
      style={{
        position: 'sticky',
        bottom: 0,
        display: 'grid',
        gridTemplateColumns: 'repeat(4, 1fr)',
        background: 'var(--tg-theme-bg-color)',
        borderTop: '1px solid var(--sep)',
        paddingBottom: 'max(8px, env(safe-area-inset-bottom))',
      }}
    >
      <button type="button" style={item(active === 'deck')} onClick={() => onChange?.('deck')}>
        <PeopleIcon />
        <span style={label}>Соседи</span>
      </button>
      <button type="button" style={item(active === 'likes')} onClick={() => onChange?.('likes')}>
        <HeartIcon badge={unreadLikes} />
        <span style={label}>Лайки</span>
      </button>
      <button type="button" style={item(active === 'searches')} onClick={() => onChange?.('searches')}>
        <SearchesIcon dot={hasNewInSearches} />
        <span style={label}>Поиски</span>
      </button>
      <button type="button" style={item(active === 'profile')} onClick={() => onChange?.('profile')}>
        <ProfileIcon />
        <span style={label}>Профиль</span>
      </button>
    </div>
  );
}
