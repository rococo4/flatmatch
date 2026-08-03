/* Таб-бар: Лента / Мои поиски / Профиль (+бейдж входящих лайков).
   Иконки — простые геометрические примитивы, без иллюстраций. */
import type { CSSProperties } from 'react';

export type Tab = 'feed' | 'searches' | 'profile';

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

function FeedIcon() {
  const bar: CSSProperties = { height: 3, borderRadius: 2, background: 'currentColor' };
  return (
    <div style={{ width: 22, height: 22, display: 'flex', flexDirection: 'column', justifyContent: 'center', gap: 3 }}>
      <span style={bar} />
      <span style={bar} />
      <span style={{ ...bar, width: '60%' }} />
    </div>
  );
}

function SearchesIcon({ dot }: { dot?: boolean }) {
  return (
    <div
      style={{
        position: 'relative',
        width: 22,
        height: 22,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
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

function ProfileIcon({ badge }: { badge?: number }) {
  return (
    <div
      style={{
        position: 'relative',
        width: 22,
        height: 22,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 1,
      }}
    >
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
      {!!badge && badge > 0 && (
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
          {badge}
        </span>
      )}
    </div>
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
        gridTemplateColumns: 'repeat(3, 1fr)',
        background: 'var(--tg-theme-bg-color)',
        borderTop: '1px solid var(--sep)',
        paddingBottom: 'max(8px, env(safe-area-inset-bottom))',
      }}
    >
      <button type="button" style={item(active === 'feed')} onClick={() => onChange?.('feed')}>
        <FeedIcon />
        <span style={label}>Лента</span>
      </button>
      <button type="button" style={item(active === 'searches')} onClick={() => onChange?.('searches')}>
        <SearchesIcon dot={hasNewInSearches} />
        <span style={label}>Мои поиски</span>
      </button>
      <button type="button" style={item(active === 'profile')} onClick={() => onChange?.('profile')}>
        <ProfileIcon badge={unreadLikes} />
        <span style={label}>Профиль</span>
      </button>
    </div>
  );
}
