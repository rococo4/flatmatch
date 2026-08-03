/* Экран «Лента» — выдача по текущим параметрам (§6 v2, search-first).
   Чипы сверху = текущие фильтры просмотра, без кнопок. «Изменить» — в шапке.
   Сохранение поиска живёт только в промо-блоке людей.
   Микрокопия дословная. */
import type { CSSProperties } from 'react';
import {
  FactsBlock,
  HabitsBlock,
  LimitChip,
  PerPerson,
  PersonRow,
  PhotoPlaceholder,
  PoolCount,
  PoolEntryButton,
  S,
  type Fact,
} from '../ui/primitives';
import { PoolPromoBlock, type PromoState } from '../ui/PoolPromoBlock';
import { FreshnessRow, type Freshness } from './EdgeStates';
import { TabBar } from '../ui/TabBar';

export type FlatCard = {
  kind: 'flat';
  id: string;
  source: string;
  postedAt: string;
  title: string;
  priceTotal: number;
  pricePerPerson: number;
  facts: Fact[];
  /** Ограничения собственника — серые чипы на карточке: рушат мэтч до контакта. */
  ownerLimits?: string[];
  /** Источник и свежесть: ссылка на оригинал + строка проверки под ней. */
  sourceName?: string;
  freshness?: Freshness;
  poolCount: number;
};

/** «К себе»: пула нет — счётчик откликов и дорогой лайк сразу автору. */
export type HomeCard = {
  kind: 'intent_home';
  id: string;
  name: string;
  age: number;
  occupation: string;
  lastSeen: string;
  verifiedBadge?: string;
  /** Стадия автора: шапка карточки меняет текст. */
  stage: 'living' | 'moving';
  /** Для 'moving' — дата заезда в шапке: «15 августа» */
  moveInLabel?: string;
  facts: Fact[];
  /** Ограничения квартиры — серые чипы ДО кнопки дорогого лайка. */
  homeLimits?: string[];
  habits: string[];
  responseCount: number; // «5 откликов»
};

export const homeCardTitle = (it: HomeCard) =>
  it.stage === 'moving'
    ? `Сосед к себе — заезжает ${it.moveInLabel ?? ''}`.trim()
    : 'Сосед к себе — уже живёт там';

export type FeedItem = FlatCard | HomeCard;

const money = (n: number) => `${n.toLocaleString('ru-RU')} ₽`;

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
    gap: 8,
    padding: '0 14px',
    height: 52,
    flex: '0 0 52px',
    background: 'var(--tg-theme-header-bg-color)',
    borderBottom: '1px solid var(--sep)',
  } as CSSProperties,
  edit: {
    marginLeft: 'auto',
    fontSize: 15,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
    background: 'none',
    border: 'none',
    font: 'inherit',
  } as CSSProperties,
  body: {
    flex: '1 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
    padding: '12px 0 16px',
  } as CSSProperties,
  chipRow: {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    overflowX: 'auto',
    padding: '0 12px',
  } as CSSProperties,
  paramChip: {
    flex: '0 0 auto',
    padding: '6px 11px',
    borderRadius: 9,
    background: 'var(--chip-bg)',
    fontSize: 13,
    fontWeight: 510,
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  list: { display: 'flex', flexDirection: 'column', gap: 12, padding: '0 12px' } as CSSProperties,
};

export function FeedScreen({
  zone,
  params,
  promo,
  items,
  unreadLikes,
  hasNewInSearches,
  onEditParams,
  onJoinPool,
  onOpenPool,
  onEnterFlatPool,
  onOpenSource,
  onWantRoom,
  onOpenProfile,
}: {
  zone: string;
  params: string[]; // текущие фильтры просмотра, только чтение
  promo: PromoState;
  items: FeedItem[];
  unreadLikes: number;
  hasNewInSearches?: boolean;
  onEditParams?: () => void;
  onJoinPool?: () => void;
  onOpenPool?: () => void;
  onEnterFlatPool?: (id: string) => void;
  /** Ссылка на оригинал; после возврата хост показывает StillLiveSheet. */
  onOpenSource?: (id: string) => void;
  onWantRoom?: (id: string) => void;
  /** Тап по строке автора → полный профиль с анкетой и «о себе». */
  onOpenProfile?: (id: string) => void;
}) {
  return (
    <div style={L.screen}>
      <div style={L.header}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 1, minWidth: 0 }}>
          <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>{zone}</span>
          <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
            {params.length} параметра поиска
          </span>
        </div>
        <button type="button" style={L.edit} onClick={onEditParams}>
          Изменить параметры
        </button>
      </div>

      <div style={L.body}>
        <div style={L.chipRow} className="no-scrollbar">
          <span
            style={{
              flex: '0 0 auto',
              fontSize: 11,
              fontWeight: 590,
              letterSpacing: '.05em',
              textTransform: 'uppercase',
              color: 'var(--tg-theme-hint-color)',
              paddingRight: 2,
            }}
          >
            Ищу
          </span>
          {params.map((p) => (
            <span key={p} style={L.paramChip}>
              {p}
            </span>
          ))}
        </div>

        <div style={L.list}>
          <PoolPromoBlock data={promo} onJoin={onJoinPool} onOpenPool={onOpenPool} />

          {items.map((it) =>
            it.kind === 'flat' ? (
              <FlatPointCard key={it.id} it={it} onEnterPool={onEnterFlatPool} onOpenSource={onOpenSource} />
            ) : (
              <HomePointCard key={it.id} it={it} onWantRoom={onWantRoom} onOpenProfile={onOpenProfile} />
            ),
          )}
        </div>
      </div>

      <TabBar active="feed" unreadLikes={unreadLikes} hasNewInSearches={hasNewInSearches} />
    </div>
  );
}

/** Квартира с рынка: пул есть, вход дешёвый — secondary. */
function FlatPointCard({
  it,
  onEnterPool,
  onOpenSource,
}: {
  it: FlatCard;
  onEnterPool?: (id: string) => void;
  onOpenSource?: (id: string) => void;
}) {
  return (
    <article style={S.card}>
      <div style={{ ...S.cardHeadBase, background: 'var(--type-flat-bg)', color: 'var(--type-flat-fg)' }}>
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>Квартира с рынка</span>
        <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>
          {it.source} · {it.postedAt}
        </span>
      </div>
      <div style={S.cardBody}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <PhotoPlaceholder label="скрин объявления" style={{ aspectRatio: '16/9', borderRadius: 10 }} />
          <span style={S.title}>{it.title}</span>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <span style={{ fontSize: 20, fontWeight: 640, letterSpacing: '-0.4px' }}>
              {money(it.pricePerPerson)}{' '}
              <span
                style={{ fontSize: 14, fontWeight: 400, color: 'var(--tg-theme-subtitle-text-color)' }}
              >
                с человека
              </span>
            </span>
            <span style={S.subtitle}>{money(it.priceTotal)} / мес за квартиру · на двоих</span>
          </div>
        </div>
        <FactsBlock title="Квартира" facts={it.facts}>
          {it.ownerLimits && it.ownerLimits.length > 0 && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingTop: 2 }}>
              {it.ownerLimits.map((l) => (
                <span
                  key={l}
                  style={{
                    padding: '4px 8px',
                    borderRadius: 7,
                    background: 'var(--chip-bg)',
                    color: 'var(--tg-theme-subtitle-text-color)',
                    fontSize: 12,
                  }}
                >
                  {l}
                </span>
              ))}
            </div>
          )}
        </FactsBlock>
        {it.sourceName && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <button
              type="button"
              onClick={() => onOpenSource?.(it.id)}
              style={{
                alignSelf: 'flex-start',
                border: 'none',
                background: 'none',
                padding: 0,
                font: 'inherit',
                fontSize: 13,
                fontWeight: 510,
                color: 'var(--tg-theme-accent-text-color)',
                cursor: 'pointer',
              }}
            >
              Открыть на {it.sourceName} ↗
            </button>
            {it.freshness && <FreshnessRow freshness={it.freshness} />}
          </div>
        )}
        <PoolCount n={it.poolCount} kind="flat" hint="увидишь после входа" />
        <PoolEntryButton
          label="Интересно — войти в пул"
          caption="Вход бесплатный и безлимитный"
          onClick={() => onEnterPool?.(it.id)}
        />
      </div>
    </article>
  );
}

/** Сосед к себе: пула нет. Счётчик откликов + дорогой лайк автору (filled).
 *  Тап по строке человека — полный профиль (тот же экран, что из пула). */
function HomePointCard({
  it,
  onWantRoom,
  onOpenProfile,
}: {
  it: HomeCard;
  onWantRoom?: (id: string) => void;
  onOpenProfile?: (id: string) => void;
}) {
  return (
    <article style={S.card}>
      <div style={{ ...S.cardHeadBase, background: 'var(--type-home-bg)', color: 'var(--type-home-fg)' }}>
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>
          {homeCardTitle(it)}
        </span>
        <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>{it.lastSeen}</span>
      </div>
      <div style={S.cardBody}>
        <div
          onClick={() => onOpenProfile?.(it.id)}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            padding: 8,
            margin: -8,
            borderRadius: 11,
            cursor: 'pointer',
          }}
        >
          <PersonRow
            name={it.name}
            age={it.age}
            occupation={it.occupation}
            verifiedBadge={it.verifiedBadge}
          />
          <span
            style={{
              marginLeft: 'auto',
              flex: '0 0 auto',
              display: 'flex',
              alignItems: 'center',
              gap: 4,
              fontSize: 13,
              fontWeight: 510,
              color: 'var(--tg-theme-accent-text-color)',
            }}
          >
            Анкета<span style={{ fontSize: 17, lineHeight: 1 }}>›</span>
          </span>
        </div>
        <FactsBlock title={`Комната у ${it.name}`} facts={it.facts}>
          <PhotoPlaceholder inCard label="фото комнаты" style={{ aspectRatio: '16/9', borderRadius: 8 }} />
          {it.homeLimits && it.homeLimits.length > 0 && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingTop: 2 }}>
              {it.homeLimits.map((l) => (
                <span
                  key={l}
                  style={{
                    padding: '4px 8px',
                    borderRadius: 7,
                    background: 'var(--chip-bg)',
                    color: 'var(--tg-theme-subtitle-text-color)',
                    fontSize: 12,
                  }}
                >
                  {l}
                </span>
              ))}
            </div>
          )}
        </FactsBlock>
        <HabitsBlock items={it.habits} />
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            fontSize: 13,
            color: 'var(--tg-theme-subtitle-text-color)',
          }}
        >
          <span
            style={{
              width: 6,
              height: 6,
              borderRadius: '50%',
              background: 'var(--type-home-fg)',
              flex: '0 0 auto',
            }}
          />
          <span style={{ color: 'var(--tg-theme-text-color)', fontWeight: 510 }}>
            {it.responseCount} откликов
          </span>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
          <button type="button" style={S.filledButton} onClick={() => onWantRoom?.(it.id)}>
            Хочу жить вместе
          </button>
          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <LimitChip small>3 из 5</LimitChip>
          </div>
        </div>
      </div>
    </article>
  );
}

/* Демо-данные для вёрстки. */
export const DEMO_PARAMS = ['Хамовники', '35–45к с человека', '2 комнаты', 'с 1 сентября'];

export const DEMO_PROMO: PromoState = {
  state: 'offer',
  zone: 'Хамовники',
  zoneIn: 'Хамовниках',
  zoneGen: 'Хамовников',
  peopleCount: 6,
  fullMatchCount: 3,
  avatars: ['Аня', 'Даша', 'Иван', 'Настя'],
  slot: 1,
  slotsTotal: 3,
};

export const DEMO_FEED: FeedItem[] = [
  {
    kind: 'flat',
    id: 'p1',
    source: 'Авито',
    postedAt: '27 июля',
    title: '2-комнатная · Хамовники',
    priceTotal: 90000,
    pricePerPerson: 45000,
    facts: [
      { label: 'Метро', value: 'Фрунзенская · 12 мин пешком' },
      { label: 'Комнаты', value: '2, изолированные' },
      { label: 'Мебель и техника', value: 'есть' },
      { label: 'Заезд', value: 'с 15 августа' },
      { label: 'Санузлы', value: '1 раздельный' },
      { label: 'Коммуналка', value: 'по счётчикам' },
    ],
    ownerLimits: ['без животных', 'не сдают парам'],
    sourceName: 'Авито',
    freshness: { kind: 'confirmed', label: 'подтверждена вчера' },
    poolCount: 6,
  },
  {
    kind: 'intent_home',
    id: 'p2',
    name: 'Ира',
    age: 27,
    occupation: 'удалёнка',
    lastSeen: 'была вчера',
    verifiedBadge: 'Рабочая почта',
    stage: 'living',
    facts: [
      { label: 'Цена доли', value: <PerPerson>28 000 ₽</PerPerson> },
      { label: 'Метро', value: 'Сокол · 8 мин пешком' },
      { label: 'Комната', value: 'изолированная, 14 м²' },
      { label: 'Депозит', value: '28 000 ₽' },
      { label: 'Заезд', value: 'с 15 августа' },
    ],
    homeLimits: ['нельзя с животными', 'курить нельзя'],
    habits: ['жаворонок', 'не курю', 'кот'],
    responseCount: 5,
  },
];
