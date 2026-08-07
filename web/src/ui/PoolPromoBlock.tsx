/* Промо-блок пула людей — единственное место, где сохраняется поиск (§6 v2).
   Розовая подложка — предложение войти; зелёная — ты уже в пуле.
   Кнопки «Сохранить поиск» в панели чипов нет by design. */
import type { CSSProperties } from 'react';
import { plural } from '../api/format';
import { AvatarStack, LimitChip, S } from './primitives';

/**
 * Четыре состояния блока (§6).
 *
 * `zoneIn` есть во всех четырёх, включая saved: склонения приходят с сервера
 * готовыми, и разный их набор по вариантам только плодил бы ветки у
 * вызывающего. `avatars` — URL фотографий, а не имена (в стек попадают
 * только люди с одобренным фото).
 */
export type PromoState =
  | { state: 'offer'; zone: string; zoneIn: string; zoneGen: string; peopleCount: number; fullMatchCount: number; avatars: string[]; slot: number; slotsTotal: number }
  | { state: 'empty'; zone: string; zoneIn: string; zoneGen: string; peopleCount: 0; slot: number; slotsTotal: number }
  | { state: 'full'; zone: string; zoneIn: string; zoneGen: string; peopleCount: number; fullMatchCount: number; avatars: string[]; slotsTotal: number }
  | { state: 'saved'; zone: string; zoneIn: string; zoneGen: string; peopleCount: number; fullMatchCount: number; avatars: string[]; slot: number; slotsTotal: number };

const P = {
  card: (saved: boolean): CSSProperties => ({
    borderRadius: 14,
    padding: '14px 12px 12px',
    background: saved ? 'var(--saved-card-bg)' : 'var(--search-card-bg)',
    border: `1px solid ${saved ? 'var(--saved-card-border)' : 'var(--search-card-border)'}`,
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
  }),
  kicker: (saved: boolean): CSSProperties => ({
    padding: '3px 8px',
    borderRadius: 7,
    background: saved ? 'var(--type-home-bg)' : 'var(--type-search-bg)',
    color: saved ? 'var(--type-home-fg)' : 'var(--type-search-fg)',
    fontSize: 11,
    fontWeight: 640,
    letterSpacing: '.05em',
    textTransform: 'uppercase',
  }),
  meta: (saved: boolean): CSSProperties => ({
    fontSize: 12,
    fontWeight: 510,
    color: saved ? 'var(--type-home-fg)' : 'var(--type-search-fg)',
    opacity: 0.85,
  }),
  headline: { fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px' } as CSSProperties,
  disabledButton: {
    width: '100%',
    padding: 13,
    border: 'none',
    borderRadius: 10,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 16,
    fontWeight: 590,
    color: 'var(--tg-theme-hint-color)',
    cursor: 'not-allowed',
  } as CSSProperties,
  footnote: {
    fontSize: 12,
    lineHeight: 1.4,
    color: 'var(--tg-theme-hint-color)',
    textAlign: 'center',
  } as CSSProperties,
};

export function PoolPromoBlock({
  data,
  onJoin,
  onOpenPool,
  onFreeSlot,
}: {
  data: PromoState;
  onJoin?: () => void;
  onOpenPool?: () => void;
  onFreeSlot?: () => void;
}) {
  const saved = data.state === 'saved';

  return (
    <article style={P.card(saved)}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <span style={P.kicker(saved)}>{saved ? 'Ты в пуле' : 'Люди'}</span>
        <span style={P.meta(saved)}>
          {saved ? `слот ${data.slot} из ${data.slotsTotal}` : data.zone}
        </span>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        {/* Предлог не дописываем: name_in приходит с ним («в Хамовниках»,
            «на красной ветке», «у метро Сокол»). */}
        <span style={P.headline}>
          {data.state === 'saved'
            ? `Ты в пуле ${data.zoneGen} · поиск сохранён`
            : `${data.peopleCount} ${plural(data.peopleCount, 'человек', 'человека', 'человек')} ищут ${data.zoneIn}`}
        </span>
        <span style={S.subtitle}>
          {data.state === 'saved'
            ? 'пуш о каждом новом совпавшем'
            : data.state === 'empty'
              ? 'никто пока не ищет — войди в пул первым, пришлём пуш'
              : `с ${data.fullMatchCount} полное совпадение по твоим параметрам`}
        </span>
      </div>

      <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
        {data.state === 'empty' ? (
          <span
            style={{
              width: 36,
              height: 36,
              borderRadius: '50%',
              border: '1px dashed color-mix(in srgb, var(--type-search-fg) 45%, transparent)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 15,
              color: 'var(--type-search-fg)',
              opacity: 0.7,
            }}
          >
            ＋
          </span>
        ) : (
          <AvatarStack urls={data.avatars} total={data.peopleCount} size={36} />
        )}
        <span
          style={{
            fontSize: 13,
            color: saved ? 'var(--tg-theme-subtitle-text-color)' : 'var(--tg-theme-hint-color)',
          }}
        >
          {data.state === 'saved'
            ? `с ${data.fullMatchCount} полное совпадение`
            : data.state === 'empty'
              ? 'ты будешь первым в пуле'
              : 'анкеты откроются в пуле'}
        </span>
      </div>

      {data.state === 'saved' ? (
        <button type="button" style={{ ...S.secondaryButton, background: 'var(--card-bg)' }} onClick={onOpenPool}>
          Смотреть пул · {data.peopleCount}{' '}
          {plural(data.peopleCount, 'человек', 'человека', 'человек')}
        </button>
      ) : data.state === 'full' ? (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
          <button type="button" style={P.disabledButton} disabled>
            Войти в пул {data.zoneGen} — сохранить поиск
          </button>
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 8,
              flexWrap: 'wrap',
            }}
          >
            <LimitChip>поиски заняты {data.slotsTotal} из {data.slotsTotal}</LimitChip>
            <span
              style={{
                fontSize: 12,
                fontWeight: 510,
                color: 'var(--tg-theme-accent-text-color)',
                cursor: 'pointer',
              }}
              onClick={onFreeSlot}
            >
              Освободить слот
            </span>
          </div>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
          <button type="button" style={S.filledButton} onClick={onJoin}>
            Войти в пул {data.zoneGen} — сохранить поиск
          </button>
          <span style={P.footnote}>
            Пул собирается по месту. Твои параметры сохранятся как поиск · слот {data.slot} из{' '}
            {data.slotsTotal}
          </span>
        </div>
      )}
    </article>
  );
}
