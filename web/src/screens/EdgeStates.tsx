/* Пограничные состояния — §6 / §2.3.
   1) пустая зона в ленте + живые зоны рядом
   2) слоты поисков заняты
   3) шторка выхода из квартирного пула (лайки не сгорают — про лимит молчим)
   4) гейт второго «живу» в форме «к себе» */
import type { CSSProperties } from 'react';
import { S } from '../ui/primitives';

const E = {
  promoCard: {
    borderRadius: 14,
    padding: '14px 12px 12px',
    background: 'var(--search-card-bg)',
    border: '1px solid var(--search-card-border)',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
  } as CSSProperties,
  kicker: {
    padding: '3px 8px',
    borderRadius: 7,
    background: 'var(--type-search-bg)',
    color: 'var(--type-search-fg)',
    fontSize: 11,
    fontWeight: 640,
    letterSpacing: '.05em',
    textTransform: 'uppercase',
  } as CSSProperties,
  zoneMeta: {
    fontSize: 12,
    fontWeight: 510,
    color: 'var(--type-search-fg)',
    opacity: 0.85,
  } as CSSProperties,
  headline: {
    fontSize: 19,
    fontWeight: 640,
    letterSpacing: '-0.4px',
    textWrap: 'pretty',
  } as CSSProperties,
  nearbyButton: {
    flex: '1 1 0',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 8,
    padding: '11px 12px',
    border: 'none',
    borderRadius: 10,
    background: 'var(--card-bg)',
    font: 'inherit',
    cursor: 'pointer',
  } as CSSProperties,
  warnRow: {
    display: 'flex',
    alignItems: 'center',
    gap: 9,
    padding: '10px 11px',
    borderRadius: 10,
    background: 'var(--warning-bg)',
    color: 'var(--warning-fg)',
  } as CSSProperties,
  sheet: {
    borderRadius: 16,
    padding: '16px 14px 14px',
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
  } as CSSProperties,
  grabber: {
    alignSelf: 'center',
    width: 36,
    height: 4,
    borderRadius: 2,
    background: 'var(--chip-bg)',
  } as CSSProperties,
  stay: {
    flex: '1 1 0',
    padding: 13,
    border: 'none',
    borderRadius: 11,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  leave: {
    flex: '1 1 0',
    padding: 13,
    border: '1px solid color-mix(in srgb, var(--tg-theme-destructive-text-color) 30%, transparent)',
    borderRadius: 11,
    background: 'color-mix(in srgb, var(--tg-theme-destructive-text-color) 10%, var(--card-bg))',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-destructive-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
};

/** Пустая зона: единственный способ появиться — войти первым. */
export function EmptyZonePromo({
  zone,
  zoneGen,
  nearby,
  onJoin,
  onOpenZone,
}: {
  zone: string; // «Бутово»
  zoneGen: string; // «Бутово» в родительном
  nearby: { zone: string; count: number }[];
  onJoin?: () => void;
  onOpenZone?: (zone: string) => void;
}) {
  return (
    <article style={E.promoCard}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <span style={E.kicker}>Люди</span>
        <span style={E.zoneMeta}>{zone}</span>
      </div>
      <span style={E.headline}>
        В {zone} пока никто не ищет — войди в пул первым, пришлём пуш
      </span>
      <button type="button" style={S.filledButton} onClick={onJoin}>
        Войти в пул {zoneGen} — сохранить поиск
      </button>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 7, paddingTop: 2 }}>
        <span
          style={{
            fontSize: 12,
            fontWeight: 590,
            letterSpacing: '.04em',
            textTransform: 'uppercase',
            color: 'var(--tg-theme-hint-color)',
          }}
        >
          Рядом ищут
        </span>
        <div style={{ display: 'flex', gap: 8 }}>
          {nearby.map((n) => (
            <button key={n.zone} type="button" style={E.nearbyButton} onClick={() => onOpenZone?.(n.zone)}>
              <span style={{ fontSize: 14, fontWeight: 510, color: 'var(--tg-theme-text-color)' }}>
                {n.zone}
              </span>
              <span style={{ fontSize: 14, fontWeight: 640, color: 'var(--type-search-fg)' }}>
                {n.count}
              </span>
            </button>
          ))}
        </div>
      </div>
    </article>
  );
}

/** Слоты заняты: вход недоступен, зовём закрыть один поиск. */
export function SlotsFullPromo({
  zone,
  zoneGen,
  peopleCount,
  fullMatchCount,
  slotsTotal,
  onOpenSearches,
}: {
  zone: string;
  zoneGen: string;
  peopleCount: number;
  fullMatchCount: number;
  slotsTotal: number;
  onOpenSearches?: () => void;
}) {
  return (
    <article style={E.promoCard}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
        <span style={E.kicker}>Люди</span>
        <span style={E.zoneMeta}>{zone}</span>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <span style={E.headline}>
          {peopleCount} человека ищут в {zone === 'Сокол' ? 'Соколе' : zone}
        </span>
        <span style={S.subtitle}>с {fullMatchCount} полное совпадение по твоим параметрам</span>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <button
          type="button"
          disabled
          style={{
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
          }}
        >
          Войти в пул {zoneGen} — сохранить поиск
        </button>
        <div style={E.warnRow} onClick={onOpenSearches}>
          <span style={{ flex: '1 1 auto', fontSize: 13, fontWeight: 590, lineHeight: 1.35 }}>
            Поиски заняты · {slotsTotal} из {slotsTotal} — закрой один из трёх в «Моих поисках»
          </span>
          <span style={{ flex: '0 0 auto', fontSize: 17, lineHeight: 1, opacity: 0.8 }}>›</span>
        </div>
      </div>
    </article>
  );
}

/** Выход из квартирного пула. Лайки не сгорают — про лимит здесь ничего не говорим. */
export function LeavePoolSheet({
  pointTitle,
  onStay,
  onLeave,
}: {
  /** «Двушки на Соколе» — в родительном падеже */
  pointTitle: string;
  onStay?: () => void;
  onLeave?: () => void;
}) {
  return (
    <div style={{ ...E.sheet, gap: 14 }}>
      <span style={E.grabber} />
      <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px', textWrap: 'pretty' }}>
        Выйти из пула {pointTitle}?
      </span>
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" style={E.stay} onClick={onStay}>
          Остаться
        </button>
        <button type="button" style={E.leave} onClick={onLeave}>
          Выйти
        </button>
      </div>
    </div>
  );
}

/** Свежесть точки: спрашиваем один раз на точку на человека, после возврата
 *  из ссылки на оригинал. Закрывается свайпом — отказ ответить нормален. */
export function StillLiveSheet({
  onYes,
  onRented,
}: {
  onYes?: () => void;
  onRented?: () => void;
}) {
  return (
    <div style={{ ...E.sheet, gap: 14 }}>
      <span style={E.grabber} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
        <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px' }}>
          Объявление ещё живое?
        </span>
        <span
          style={{
            fontSize: 13,
            color: 'var(--tg-theme-subtitle-text-color)',
            textWrap: 'pretty',
          }}
        >
          Спросим один раз — чтобы пул не собирался на сданную квартиру
        </span>
      </div>
      <div style={{ display: 'flex', gap: 8 }}>
        <button
          type="button"
          onClick={onYes}
          style={{
            flex: '1 1 0',
            padding: 13,
            border: 'none',
            borderRadius: 11,
            background: 'var(--verdict-ok-bg)',
            font: 'inherit',
            fontSize: 15,
            fontWeight: 590,
            color: 'var(--verdict-ok-fg)',
            cursor: 'pointer',
          }}
        >
          Да
        </button>
        <button
          type="button"
          onClick={onRented}
          style={{
            flex: '1 1 0',
            padding: 13,
            border: 'none',
            borderRadius: 11,
            background: 'var(--warning-bg)',
            font: 'inherit',
            fontSize: 15,
            fontWeight: 590,
            color: 'var(--warning-fg)',
            cursor: 'pointer',
          }}
        >
          Уже сдана
        </button>
      </div>
    </div>
  );
}

export type Freshness =
  | { kind: 'confirmed'; label: string } // «подтверждена вчера»
  | { kind: 'stale'; label: string } // «добавлена 5 дней назад, не проверялась»
  | { kind: 'doubt'; label: string }; // «1 участник считает, что сдана»

/** Строка свежести под ссылкой на оригинал. Жёлтый — только сомнение. */
export function FreshnessRow({ freshness }: { freshness: Freshness }) {
  const dot =
    freshness.kind === 'confirmed'
      ? 'var(--verdict-ok-fg)'
      : freshness.kind === 'doubt'
        ? 'var(--warning-fg)'
        : 'var(--tg-theme-hint-color)';
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
      <span style={{ flex: '0 0 auto', width: 6, height: 6, borderRadius: '50%', background: dot }} />
      <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>{freshness.label}</span>
    </div>
  );
}

/** «Похоже, сдана»: карточка приглушена, вход в пул заменён похожими рядом. */
export function LikelyRentedCard({
  title,
  pricePerPerson,
  reportedBy,
  onSimilar,
}: {
  title: string;
  pricePerPerson: string;
  reportedBy: string; // «3 участника отметили, что сдана»
  onSimilar?: () => void;
}) {
  return (
    <article style={{ ...S.card, opacity: 0.62 }}>
      <div style={{ ...S.cardHeadBase, background: 'var(--type-flat-bg)', color: 'var(--type-flat-fg)' }}>
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>Квартира с рынка</span>
        <span
          style={{
            padding: '2px 8px',
            borderRadius: 7,
            background: 'var(--warning-bg)',
            color: 'var(--warning-fg)',
            fontSize: 12,
            fontWeight: 640,
          }}
        >
          похоже, сдана
        </span>
      </div>
      <div style={{ ...S.cardBody, gap: 10 }}>
        <div style={{ display: 'flex', gap: 10 }}>
          <div className="ph-photo" style={{ flex: '0 0 72px', height: 54, borderRadius: 8 }}>
            <span>скрин</span>
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
            <span style={{ fontSize: 15, fontWeight: 590 }}>{title}</span>
            <span style={S.subtitle}>{pricePerPerson}</span>
            <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>{reportedBy}</span>
          </div>
        </div>
        <button type="button" style={S.secondaryButton} onClick={onSimilar}>
          Похожие квартиры рядом
        </button>
      </div>
    </article>
  );
}

/** Гейт стадии в форме «к себе»: вторая точка «живу здесь» невозможна. */
export function HomeStageGate({ onPickMoving }: { onPickMoving?: () => void }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 11,
          padding: '13px 12px',
          borderRadius: 12,
          background: 'var(--card-bg)',
          border: '1px solid var(--sep)',
          opacity: 0.55,
          cursor: 'not-allowed',
        }}
      >
        <span
          style={{
            flex: '0 0 auto',
            width: 20,
            height: 20,
            borderRadius: '50%',
            boxSizing: 'border-box',
            border: '1.5px solid color-mix(in srgb, var(--tg-theme-hint-color) 60%, transparent)',
          }}
        />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
          <span style={{ fontSize: 15, fontWeight: 590 }}>Уже живу здесь</span>
          <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
            ты уже зовёшь к себе там, где живёшь
          </span>
        </div>
      </div>

      <div
        onClick={onPickMoving}
        style={{
          display: 'flex',
          alignItems: 'flex-start',
          gap: 11,
          padding: '13px 12px',
          borderRadius: 12,
          background: 'var(--card-bg)',
          border: '1.5px solid var(--type-home-fg)',
          cursor: 'pointer',
        }}
      >
        <span
          style={{
            flex: '0 0 auto',
            width: 20,
            height: 20,
            borderRadius: '50%',
            boxSizing: 'border-box',
            border: '6px solid var(--type-home-fg)',
            background: 'var(--card-bg)',
            marginTop: 1,
          }}
        />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
          <span style={{ fontSize: 15, fontWeight: 590 }}>Договорился и заезжаю</span>
          <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
            квартира есть, ищу соседа со старта
          </span>
          <span
            style={{
              marginTop: 2,
              alignSelf: 'flex-start',
              padding: '3px 8px',
              borderRadius: 7,
              background: 'var(--type-home-bg)',
              color: 'var(--type-home-fg)',
              fontSize: 12,
              fontWeight: 590,
            }}
          >
            заезжает · ищет со-съёмщика со старта
          </span>
        </div>
      </div>
    </div>
  );
}

export const DEMO_NEARBY = [
  { zone: 'Тёплый Стан', count: 6 },
  { zone: 'Ясенево', count: 4 },
];
