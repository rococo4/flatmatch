/* «Тебя лайкнули» — §2.3 / §6.
   Два вида контекста: зонный пул и отклик на мою точку «к себе».
   Ответный лайк не тратит лимит — строка сверху, не на карточке.
   Отдельного авторского экрана откликов нет by design. */
import type { CSSProperties } from 'react';
import { Avatar, Badge, S } from '../ui/primitives';
import { TabBar } from '../ui/TabBar';

export type IncomingLike = {
  id: string;
  name: string;
  age: number;
  /** «лайкнул тебя» / «хочет заехать» */
  action: string;
  when: string; // «вчера», «сегодня»
  verifiedBadge?: string;
  context:
    | { kind: 'zone'; title: string; hint: string } // «из пула Хамовников» + «твой поиск 35–45к»
    | { kind: 'room'; title: string; hint: string }; // «на твою комнату:» + описание комнаты
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
    gap: 12,
    padding: '12px 12px 20px',
  } as CSSProperties,
  topLine: { fontSize: 13, color: 'var(--tg-theme-hint-color)', padding: '0 2px' } as CSSProperties,
  card: {
    display: 'flex',
    flexDirection: 'column',
    gap: 11,
    padding: 12,
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  contextRow: (room: boolean): CSSProperties => ({
    display: 'flex',
    alignItems: 'center',
    gap: 9,
    padding: '10px 11px',
    borderRadius: 11,
    background: room ? 'var(--saved-card-bg)' : 'var(--search-card-bg)',
    border: `1px solid ${room ? 'var(--saved-card-border)' : 'var(--search-card-border)'}`,
    cursor: 'pointer',
  }),
  dot: (room: boolean): CSSProperties => ({
    flex: '0 0 auto',
    width: 7,
    height: 7,
    borderRadius: '50%',
    background: room ? 'var(--type-home-fg)' : 'var(--type-search-fg)',
  }),
  buttons: { display: 'flex', gap: 8 } as CSSProperties,
  secondary: {
    flex: '1 1 0',
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
  /** Filled: ответный дорогой лайк → мэтч. */
  reply: {
    flex: '1 1 0',
    padding: 12,
    border: 'none',
    borderRadius: 10,
    background: 'var(--tg-theme-button-color)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-button-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  empty: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 10,
    padding: '28px 16px',
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
};

export function IncomingLikesScreen({
  likes,
  unreadLikes,
  onBack,
  onOpenContext,
  onOpenProfile,
  onReply,
  onToFeed,
}: {
  likes: IncomingLike[];
  unreadLikes: number;
  onBack?: () => void;
  onOpenContext?: (id: string) => void;
  onOpenProfile?: (id: string) => void;
  onReply?: (id: string) => void;
  onToFeed?: () => void;
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
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Тебя лайкнули</span>
        {likes.length > 0 && (
          <span
            style={{
              marginLeft: 8,
              padding: '2px 8px',
              borderRadius: 7,
              background: 'var(--tg-theme-destructive-text-color)',
              color: '#fff',
              fontSize: 12,
              fontWeight: 640,
            }}
          >
            {likes.length}
          </span>
        )}
      </div>

      <div style={L.body}>
        <span style={L.topLine}>Ответный лайк не тратит лимит</span>

        {likes.length === 0 ? (
          <div style={L.empty}>
            <span style={{ fontSize: 16, fontWeight: 590, textAlign: 'center', textWrap: 'pretty' }}>
              Пока никто не лайкнул. Появись в пулах — сохрани поиск
            </span>
            <button
              type="button"
              style={{ ...S.secondaryButton, background: 'var(--chip-bg)', padding: 13 }}
              onClick={onToFeed}
            >
              К ленте — настроить поиск
            </button>
          </div>
        ) : (
          likes.map((l) => {
            const room = l.context.kind === 'room';
            return (
              <article key={l.id} style={L.card}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <Avatar name={l.name} size={48} />
                  <div style={{ display: 'flex', flexDirection: 'column', gap: l.verifiedBadge ? 4 : 3, minWidth: 0 }}>
                    <span style={{ fontSize: 16, fontWeight: 590, letterSpacing: '-0.2px' }}>
                      {l.name}, {l.age} {l.action}
                    </span>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 7, flexWrap: 'wrap' }}>
                      <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>{l.when}</span>
                      {l.verifiedBadge && <Badge tone="accent">{l.verifiedBadge}</Badge>}
                    </div>
                  </div>
                </div>

                <div style={L.contextRow(room)} onClick={() => onOpenContext?.(l.id)}>
                  <span style={L.dot(room)} />
                  <span style={{ flex: '1 1 auto', fontSize: 13, lineHeight: 1.35, minWidth: 0 }}>
                    <b style={{ fontWeight: room ? 590 : 400, color: room ? 'var(--type-home-fg)' : 'inherit' }}>
                      {l.context.title}
                    </b>{' '}
                    <span style={{ color: room ? 'inherit' : 'var(--tg-theme-subtitle-text-color)' }}>
                      {l.context.hint}
                    </span>
                  </span>
                  <span style={{ flex: '0 0 auto', fontSize: 17, color: 'var(--tg-theme-hint-color)', lineHeight: 1 }}>
                    ›
                  </span>
                </div>

                <div style={L.buttons}>
                  <button type="button" style={L.secondary} onClick={() => onOpenProfile?.(l.id)}>
                    Профиль
                  </button>
                  <button type="button" style={L.reply} onClick={() => onReply?.(l.id)}>
                    Хочу жить вместе
                  </button>
                </div>
              </article>
            );
          })
        )}
      </div>

      <TabBar active="profile" unreadLikes={unreadLikes} />
    </div>
  );
}

/* Демо-данные для вёрстки. */
export const DEMO_INCOMING: IncomingLike[] = [
  {
    id: 'l1',
    name: 'Игорь',
    age: 24,
    action: 'лайкнул тебя',
    when: 'вчера',
    context: { kind: 'zone', title: 'из пула Хамовников ·', hint: 'твой поиск 35–45к' },
  },
  {
    id: 'l2',
    name: 'Аня',
    age: 21,
    action: 'хочет заехать',
    when: 'сегодня',
    verifiedBadge: 'Студент ВШЭ',
    context: {
      kind: 'room',
      title: 'на твою комнату:',
      hint: 'комната в двушке · м. Аэропорт · 44к',
    },
  },
];
