/* «Тебя лайкнули» — §2.3 / §6.
   Имя сразу, тапабельная мини-карточка контекста, «Ответить» → мэтч.
   Ответный лайк не тратит лимит — строка сверху, не на карточке.
   Отдельного авторского экрана откликов нет by design.

   Контекст пока только зонный: отклики на точку «к себе» придут вместе с
   точками (§12 шаг 5), до тех пор рисовать их нечем. */
import type { CSSProperties } from 'react';
import { Avatar, Badge, S } from '../ui/primitives';
import { TabBar } from '../ui/TabBar';

export type IncomingLike = {
  /** id лайкнувшего: он же адресат ответа и вход в его профиль. */
  id: string;
  name: string;
  age: number;
  /** Пусто = фото не одобрено: у аватара заглушка с инициалами (§5). */
  photoUrl?: string;
  /** «сова · не курю · без животных» */
  habits: string;
  when: string; // «вчера», «сегодня», «26 июля»
  /** §2.3: лайк жив, а человек сейчас не ищет — пометка, а не причина прятать. */
  notSearching?: boolean;
  /** «из пула Хамовников». Нет — зона не пережила лайк или его поставили в ответ. */
  contextZone?: string;
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
  contextRow: {
    display: 'flex',
    alignItems: 'center',
    gap: 9,
    padding: '10px 11px',
    borderRadius: 11,
    background: 'var(--search-card-bg)',
    border: '1px solid var(--search-card-border)',
    cursor: 'pointer',
  } as CSSProperties,
  dot: {
    flex: '0 0 auto',
    width: 7,
    height: 7,
    borderRadius: '50%',
    background: 'var(--type-search-fg)',
  } as CSSProperties,
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
  replyingId,
  error,
  onBack,
  onOpenContext,
  onOpenProfile,
  onReply,
  onToFeed,
}: {
  likes: IncomingLike[];
  unreadLikes: number;
  /** Чей ответ сейчас улетает: кнопки гаснут, второй тап не уходит вторым лайком. */
  replyingId?: string | null;
  error?: string | null;
  onBack?: () => void;
  onOpenContext?: (id: string) => void;
  onOpenProfile?: (id: string) => void;
  onReply?: (id: string) => void;
  onToFeed?: () => void;
}) {
  const busy = Boolean(replyingId);

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

        {error && (
          <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>
            {error}
          </span>
        )}

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
          likes.map((l) => (
            <article key={l.id} style={L.card}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <Avatar name={l.name} size={48} src={l.photoUrl} />
                <div style={{ display: 'flex', flexDirection: 'column', gap: 4, minWidth: 0 }}>
                  <span style={{ fontSize: 16, fontWeight: 590, letterSpacing: '-0.2px' }}>
                    {l.name}, {l.age}
                  </span>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 7, flexWrap: 'wrap' }}>
                    <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>
                      {l.when} · {l.habits}
                    </span>
                    {l.notSearching && <Badge>сейчас не в поиске</Badge>}
                  </div>
                </div>
              </div>

              {l.contextZone && (
                <div style={L.contextRow} onClick={() => onOpenContext?.(l.id)}>
                  <span style={L.dot} />
                  <span style={{ flex: '1 1 auto', fontSize: 13, lineHeight: 1.35, minWidth: 0 }}>
                    {l.contextZone}
                  </span>
                  <span style={{ flex: '0 0 auto', fontSize: 17, color: 'var(--tg-theme-hint-color)', lineHeight: 1 }}>
                    ›
                  </span>
                </div>
              )}

              <div style={L.buttons}>
                {/* Пока ответ в полёте, уйти в профиль нельзя: экран сменился
                    бы под ответом, и мэтч показать было бы уже некому. */}
                <button
                  type="button"
                  style={{ ...L.secondary, cursor: busy ? 'default' : 'pointer' }}
                  disabled={busy}
                  onClick={() => onOpenProfile?.(l.id)}
                >
                  Профиль
                </button>
                <button
                  type="button"
                  style={{
                    ...L.reply,
                    opacity: replyingId === l.id ? 0.6 : 1,
                    cursor: busy ? 'default' : 'pointer',
                  }}
                  disabled={busy}
                  onClick={() => onReply?.(l.id)}
                >
                  Ответить
                </button>
              </div>
            </article>
          ))
        )}
      </div>

      <TabBar active="profile" unreadLikes={unreadLikes} />
    </div>
  );
}
