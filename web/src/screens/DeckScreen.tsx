/* Вкладка «Соседи» — карточный подбор (§6).

   Одна карточка на экран: человек с активным поиском или комната «к себе».
   Жест — свайп (SwipeCard), кнопки под карточкой дублируют его ровно теми же
   полётами: стрелка = пропустить, сердце = дорогой лайк. Данные и решения
   держит оболочка — здесь вёрстка, жест и три состояния колоды (без поиска,
   анкета на проверке, конец подбора).

   Слова «пул» на экране нет: механика называется подбором (§2.1). */
import { useEffect, useRef, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';

import type { DeckCard, DeckRoom, TopZoneDTO } from '../api/client';
import {
  chipLabel,
  habitsLine,
  lastSeenLabel,
  money,
  occupationLabel,
  plural,
  restrictionLabels,
  roomCardTitle,
  VERDICT_UI,
} from '../api/format';
import { GreyChips, PhotoPlaceholder, S, VerdictChip } from '../ui/primitives';
import { SwipeCard, type SwipeDir } from '../ui/SwipeCard';

/** Слотов дорогих лайков на окне 24 ч (§2.3): столько точек под кнопками. */
const LIKE_SLOTS = 5;

/** Сколько длится полёт карточки: столько же, сколько транзишн в SwipeCard. */
const FLIGHT_MS = 220;

/** Доля ширины фото, отданная под тап-листалку с каждого края. */
const PAGER_ZONE = '26%';

const D = {
  screen: {
    flex: '1 1 auto',
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
  body: {
    flex: '0 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
    padding: '12px 12px 0',
    width: '100%',
    maxWidth: 520,
    margin: '0 auto',
    boxSizing: 'border-box',
  } as CSSProperties,
  // Потолок высоты — чтобы кнопки не уезжали за экран: длинная карточка
  // комнаты (факты + описание + автор) листается внутри себя, а жест
  // остаётся горизонтальным (touch-action: pan-y в SwipeCard).
  card: {
    display: 'flex',
    flexDirection: 'column',
    maxHeight: 'calc(100vh - 236px)',
    borderRadius: 18,
    overflow: 'hidden',
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
    boxShadow: '0 6px 20px rgba(0, 0, 0, 0.08)',
  } as CSSProperties,
  photo: {
    position: 'relative',
    flex: '0 0 auto',
    width: '100%',
    aspectRatio: '4/5',
    // На низком экране фото ужимается, а не выдавливает всё остальное:
    // object-fit кадрирует, и лицо остаётся в кадре.
    maxHeight: '54vh',
    background: 'var(--chip-bg)',
    overflow: 'hidden',
  } as CSSProperties,
  gradient: {
    position: 'absolute',
    left: 0,
    right: 0,
    bottom: 0,
    display: 'flex',
    flexDirection: 'column',
    gap: 2,
    padding: '34px 14px 12px',
    background: 'linear-gradient(to top, rgba(0,0,0,.72), rgba(0,0,0,0))',
    color: '#fff',
  } as CSSProperties,
  photoBadges: {
    position: 'absolute',
    left: 10,
    right: 10,
    top: 18,
    display: 'flex',
    flexWrap: 'wrap',
    gap: 6,
  } as CSSProperties,
  photoBadge: {
    padding: '4px 9px',
    borderRadius: 8,
    background: 'rgba(0, 0, 0, .45)',
    backdropFilter: 'blur(6px)',
    color: '#fff',
    fontSize: 12,
    fontWeight: 590,
  } as CSSProperties,
  cardBody: {
    flex: '1 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    padding: 14,
    overflowY: 'auto',
  } as CSSProperties,
  actions: {
    flex: '0 0 auto',
    // Прижаты к низу: большой палец живёт там, а карточка — сверху.
    marginTop: 'auto',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 10,
    padding: '10px 12px 16px',
  } as CSSProperties,
  pill: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    height: 54,
    borderRadius: 16,
    border: 'none',
    font: 'inherit',
    cursor: 'pointer',
  } as CSSProperties,
  h1: {
    fontSize: 23,
    fontWeight: 640,
    letterSpacing: '-0.5px',
    textWrap: 'pretty',
  } as CSSProperties,
  centered: {
    flex: '1 1 auto',
    display: 'grid',
    placeItems: 'center',
    padding: 24,
    fontSize: 14,
    textAlign: 'center',
    color: 'var(--tg-theme-hint-color)',
  } as CSSProperties,
  error: { fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' } as CSSProperties,
};

export function DeckScreen({
  card,
  total,
  passed,
  loading,
  noSearch,
  ended,
  error,
  likesRemaining,
  pending,
  likeLocked,
  filtersActive,
  topZones,
  slotsUsed,
  slotsTotal,
  onOpenFilters,
  onPass,
  onLike,
  onOpenCard,
  onOpenAuthor,
  onFind,
  onPickZone,
  onRestorePassed,
  onAddSearch,
}: {
  /** Верхняя карточка колоды. Пусто — колода кончилась или ещё не приехала. */
  card?: DeckCard;
  /** Сколько всего подошло: «Соседи · N в твоих зонах». */
  total: number;
  /** Пропущенные, ждущие возврата: «Показать пропущенных · N». */
  passed: number;
  loading?: boolean;
  /** Активного поиска нет — подбора не существует (§2.1). */
  noSearch?: boolean;
  ended?: boolean;
  /** Отказ последнего действия — словами сервера. */
  error?: string | null;
  /** Остаток дорогих лайков: столько точек залито из пяти. */
  likesRemaining: number;
  /** Анкета на проверке: жёлтая плашка сверху (§5). */
  pending?: boolean;
  /** Лайк недоступен до одобрения анкеты: сердце серое, стрелка работает. */
  likeLocked?: boolean;
  /** Хоть один фильтр задан — синяя точка на иконке. */
  filtersActive?: boolean;
  /** «ГДЕ УЖЕ ИЩУТ» — топ живых зон, нужен только состоянию без поиска (§6). */
  topZones?: TopZoneDTO[];
  slotsUsed: number;
  slotsTotal: number;
  onOpenFilters?: () => void;
  onPass?: (card: DeckCard) => void;
  /**
   * Лайк. Отвечает, ушла ли карточка из колоды: отказ по лимиту или
   * непроверенной анкете её не съедает — тогда она возвращается на место.
   */
  onLike?: (card: DeckCard) => Promise<boolean>;
  /** Тап по карточке: анкета человека или экран комнаты. */
  onOpenCard?: (card: DeckCard) => void;
  /** Тап по строке автора комнаты — его анкета. */
  onOpenAuthor?: (room: DeckRoom) => void;
  /** «Найти» из состояния без поиска. */
  onFind?: () => void;
  /** Строка «ГДЕ УЖЕ ИЩУТ»: форма поиска с этой зоной. */
  onPickZone?: (zoneId: string) => void;
  onRestorePassed?: () => void;
  onAddSearch?: () => void;
}) {
  const [fly, setFly] = useState<SwipeDir | null>(null);
  const timer = useRef<number | null>(null);

  const key = card ? cardKey(card) : '';
  // Полёт живёт ровно столько, сколько показана карточка: следующая обязана
  // начинаться на месте, а не там, где улетела предыдущая.
  useEffect(() => {
    setFly(null);
    return () => {
      if (timer.current !== null) window.clearTimeout(timer.current);
      timer.current = null;
    };
  }, [key]);

  const throwCard = (dir: SwipeDir) => {
    if (!card || fly) return;
    if (dir === 'right' && likeLocked) return;
    setFly(dir);
    // Действие уходит ПОСЛЕ полёта: до него карточка ещё на экране, и
    // подменять её содержимое на лету — значит показывать чужое лицо в
    // улетающем кадре.
    timer.current = window.setTimeout(() => {
      timer.current = null;
      if (dir === 'left') {
        // Пропуск двигает колоду сразу: следующая карточка встаёт на место
        // улетевшей тем же кадром, и полёт снимать отдельно не нужно.
        setFly(null);
        onPass?.(card);
        return;
      }
      // Лайк ходит на сервер: пока он летит, карточка остаётся за краем —
      // возвращать её на середину, чтобы через мгновение убрать, значит
      // мигать. Не ушла (лимит, анкета на проверке) — возвращаем.
      void onLike?.(card).then((gone) => {
        if (!gone) setFly(null);
      });
    }, FLIGHT_MS);
  };

  return (
    <div style={D.screen}>
      <div style={D.header}>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>
          Соседи{total > 0 ? ` · ${total} в твоих зонах` : ''}
        </span>
        <FiltersButton active={filtersActive} onClick={onOpenFilters} />
      </div>

      {loading ? (
        <div style={D.centered}>Секунду…</div>
      ) : noSearch ? (
        <NoSearchState topZones={topZones} onFind={onFind} onPickZone={onPickZone} />
      ) : (
        <>
          <div style={D.body}>
            {pending && <PendingBanner />}
            {error && <span style={D.error}>{error}</span>}
            {card ? (
              <SwipeCard
                key={key}
                fly={fly}
                lockRight={likeLocked}
                onFling={throwCard}
                onTap={() => onOpenCard?.(card)}
              >
                {card.kind === 'person' ? (
                  <PersonCard card={card} />
                ) : (
                  <RoomCard card={card} onOpenAuthor={onOpenAuthor} />
                )}
              </SwipeCard>
            ) : (
              ended && (
                <EndState
                  passed={passed}
                  slotsUsed={slotsUsed}
                  slotsTotal={slotsTotal}
                  onRestorePassed={onRestorePassed}
                  onAddSearch={onAddSearch}
                />
              )
            )}
          </div>

          {card && (
            <Actions
              likesRemaining={likesRemaining}
              likeLocked={likeLocked}
              room={card.kind === 'room'}
              onPass={() => throwCard('left')}
              onLike={() => throwCard('right')}
            />
          )}
        </>
      )}
    </div>
  );
}

/** Ключ карточки: он же ключ SwipeCard — новая карточка начинается на месте. */
function cardKey(card: DeckCard): string {
  return card.kind === 'person' ? `person:${card.person.user_id}` : `room:${card.room.id}`;
}

/** Иконка фильтров — три ползунка. Синяя точка = фильтры заданы. */
function FiltersButton({ active, onClick }: { active?: boolean; onClick?: () => void }) {
  const line: CSSProperties = {
    position: 'relative',
    height: 2,
    borderRadius: 1,
    background: 'currentColor',
  };
  const knob = (left: string): CSSProperties => ({
    position: 'absolute',
    top: -3,
    left,
    width: 8,
    height: 8,
    borderRadius: 4,
    border: '2px solid currentColor',
    background: 'var(--tg-theme-header-bg-color)',
    boxSizing: 'border-box',
  });
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        marginLeft: 'auto',
        position: 'relative',
        width: 34,
        height: 34,
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'center',
        gap: 5,
        padding: '0 5px',
        border: 'none',
        borderRadius: 10,
        background: 'none',
        color: 'var(--tg-theme-text-color)',
        cursor: 'pointer',
      }}
    >
      <span style={line}>
        <span style={knob('60%')} />
      </span>
      <span style={line}>
        <span style={knob('25%')} />
      </span>
      <span style={line}>
        <span style={knob('70%')} />
      </span>
      {active && (
        <span
          style={{
            position: 'absolute',
            top: 2,
            right: 2,
            width: 7,
            height: 7,
            borderRadius: 4,
            background: 'var(--tg-theme-button-color)',
          }}
        />
      )}
    </button>
  );
}

/** Анкета ещё на проверке: смотреть можно, лайкать — нет (§5). */
function PendingBanner() {
  return (
    <div
      style={{
        padding: '10px 12px',
        borderRadius: 12,
        background: 'var(--warning-bg)',
        color: 'var(--warning-fg)',
        fontSize: 13,
        lineHeight: 1.4,
        textWrap: 'pretty',
      }}
    >
      Анкета на быстрой проверке — смотреть и пропускать можно уже сейчас. Лайк откроется после
      одобрения — тебя тоже станет видно
    </div>
  );
}

function PersonCard({ card }: { card: Extract<DeckCard, { kind: 'person' }> }) {
  const p = card.person;
  const pair = card.pair;
  const params = pair.params;
  const occupation = occupationLabel(p.occupation);

  return (
    <article style={D.card}>
      <PhotoPager
        urls={p.photo_urls}
        emptyLabel="фото на проверке"
        badges={
          <>
            <span style={D.photoBadge}>
              {pair.zone.name}
              {pair.zone_match ? ' · твоя зона' : ''}
            </span>
            {pair.section === 'all' && <span style={D.photoBadge}>совпали по всем параметрам</span>}
          </>
        }
      >
        <div style={D.gradient}>
          <span style={{ fontSize: 24, fontWeight: 640, letterSpacing: '-0.5px' }}>
            {p.name}, {p.age}
          </span>
          <span style={{ fontSize: 14, opacity: 0.9 }}>
            {[occupation, lastSeenLabel(p.last_seen)].filter(Boolean).join(' · ')}
          </span>
        </div>
      </PhotoPager>

      <div style={D.cardBody}>
        {params && (pair.chips ?? []).length > 0 && (
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
            {(pair.chips ?? []).map((c) => (
              <VerdictChip key={c.param} verdict={VERDICT_UI[c.verdict]}>
                {chipLabel(c, params)}
              </VerdictChip>
            ))}
          </div>
        )}
        <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
          {habitsLine(p)}
        </span>
      </div>
    </article>
  );
}

function RoomCard({
  card,
  onOpenAuthor,
}: {
  card: Extract<DeckCard, { kind: 'room' }>;
  onOpenAuthor?: (room: DeckRoom) => void;
}) {
  const room = card.room;
  const restrictions = restrictionLabels(room.restrictions, 'intent_home');

  return (
    <article style={D.card}>
      <div
        style={{
          ...S.cardHeadBase,
          background: 'var(--type-home-bg)',
          color: 'var(--type-home-fg)',
        }}
      >
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>
          {roomCardTitle(room.stage)}
        </span>
        <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>
          {room.responses} {plural(room.responses, 'отклик', 'отклика', 'откликов')}
        </span>
      </div>

      <PhotoPager urls={room.photo_urls} emptyLabel="фото комнаты" ratio="4/3" maxHeight="34vh" />

      <div style={D.cardBody}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {roomFacts(room).map((f) => (
            <div key={f.label} style={S.row}>
              <span style={S.rowLabel}>{f.label}</span>
              <span style={{ fontWeight: 510, textAlign: 'right' }}>{f.value}</span>
            </div>
          ))}
        </div>
        {room.description && (
          <span style={{ fontSize: 14, lineHeight: 1.45, textWrap: 'pretty' }}>
            {room.description}
          </span>
        )}
        <GreyChips items={restrictions} />
        <AuthorRow room={room} onOpen={onOpenAuthor} />
      </div>
    </article>
  );
}

/** Строка автора комнаты: тап по ней ведёт в анкету, а не в комнату (§2.2). */
function AuthorRow({ room, onOpen }: { room: DeckRoom; onOpen?: (room: DeckRoom) => void }) {
  const author = room.author;
  const occupation = occupationLabel(author.occupation);
  return (
    <div
      onPointerDown={(e) => e.stopPropagation()}
      onClick={() => onOpen?.(room)}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        padding: '10px 11px',
        borderRadius: 12,
        background: 'var(--app-bg)',
        cursor: onOpen ? 'pointer' : 'default',
      }}
    >
      <Face name={author.name} url={author.photo_urls[0]} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0 }}>
        <span style={{ fontSize: 15, fontWeight: 590 }}>
          {author.name}, {author.age}
        </span>
        <span style={{ fontSize: 12, color: 'var(--tg-theme-subtitle-text-color)' }}>
          {[occupation, habitsLine(author)].filter(Boolean).join(' · ')}
        </span>
      </div>
      <span
        style={{
          marginLeft: 'auto',
          flex: '0 0 auto',
          fontSize: 13,
          fontWeight: 510,
          color: 'var(--tg-theme-accent-text-color)',
        }}
      >
        Анкета ›
      </span>
    </div>
  );
}

/** Лицо автора в строке. Пустой адрес = набор не одобрен: тогда инициал (§5). */
function Face({ name, url }: { name: string; url?: string }) {
  return (
    <span
      style={{
        flex: '0 0 auto',
        width: 40,
        height: 40,
        borderRadius: '50%',
        overflow: 'hidden',
        background: 'var(--chip-bg)',
        color: 'var(--tg-theme-subtitle-text-color)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        fontSize: 15,
        fontWeight: 590,
      }}
    >
      {url ? (
        <img
          src={url}
          alt=""
          style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
        />
      ) : (
        name.slice(0, 1).toUpperCase()
      )}
    </span>
  );
}

/**
 * Фото карточки с пейджером.
 *
 * Листалка — два узких края поверх фото: они гасят pointerdown, поэтому
 * тянуть карточку с середины фото жест не мешает. Сегменты сверху
 * показывают, сколько кадров и какой открыт.
 */
function PhotoPager({
  urls,
  emptyLabel,
  ratio,
  maxHeight,
  badges,
  children,
}: {
  urls: string[];
  emptyLabel: string;
  ratio?: string;
  maxHeight?: string;
  badges?: ReactNode;
  children?: ReactNode;
}) {
  const [at, setAt] = useState(0);
  const shown = Math.min(at, Math.max(urls.length - 1, 0));
  const many = urls.length > 1;
  const step = (delta: number) =>
    setAt((i) => Math.min(urls.length - 1, Math.max(0, i + delta)));

  return (
    <div
      style={{
        ...D.photo,
        aspectRatio: ratio ?? D.photo.aspectRatio,
        maxHeight: maxHeight ?? D.photo.maxHeight,
      }}
    >
      {urls.length > 0 ? (
        <img
          src={urls[shown]}
          alt=""
          style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
        />
      ) : (
        <PhotoPlaceholder label={emptyLabel} style={{ width: '100%', height: '100%' }} />
      )}

      {many && (
        <>
          <div
            style={{
              position: 'absolute',
              top: 8,
              left: 10,
              right: 10,
              display: 'flex',
              gap: 4,
            }}
          >
            {urls.map((url, i) => (
              <span
                key={`${url}:${i}`}
                style={{
                  flex: '1 1 0',
                  height: 3,
                  borderRadius: 2,
                  background: i === shown ? '#fff' : 'rgba(255,255,255,.4)',
                }}
              />
            ))}
          </div>
          <div
            onPointerDown={(e) => e.stopPropagation()}
            onClick={() => step(-1)}
            style={{ position: 'absolute', left: 0, top: 0, bottom: 0, width: PAGER_ZONE }}
          />
          <div
            onPointerDown={(e) => e.stopPropagation()}
            onClick={() => step(1)}
            style={{ position: 'absolute', right: 0, top: 0, bottom: 0, width: PAGER_ZONE }}
          />
        </>
      )}

      {badges && <div style={D.photoBadges}>{badges}</div>}
      {children}
    </div>
  );
}

/**
 * Две кнопки-иконки и остаток лайков точками.
 *
 * Точки без текста: цифра «3 из 5» под каждой карточкой превращает подбор в
 * бухгалтерию, а пять кружков читаются одним взглядом (§6).
 */
function Actions({
  likesRemaining,
  likeLocked,
  room,
  onPass,
  onLike,
}: {
  likesRemaining: number;
  likeLocked?: boolean;
  room?: boolean;
  onPass?: () => void;
  onLike?: () => void;
}) {
  return (
    <div style={D.actions}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
        <button
          type="button"
          aria-label="Пропустить"
          onClick={onPass}
          style={{
            ...D.pill,
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
          aria-label={room ? 'Хочу эту комнату' : 'Хочу жить вместе'}
          disabled={likeLocked}
          onClick={onLike}
          style={{
            ...D.pill,
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
      <div style={{ display: 'flex', gap: 6 }}>
        {Array.from({ length: LIKE_SLOTS }, (_, i) => (
          <span
            key={i}
            style={{
              width: 7,
              height: 7,
              borderRadius: 4,
              background:
                i < likesRemaining ? 'var(--tg-theme-button-color)' : 'var(--verdict-no-bg)',
            }}
          />
        ))}
      </div>
    </div>
  );
}

/** Стрелка вправо: «пропустить, дальше». Та же линия, что у жеста влево. */
function ArrowIcon() {
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

function HeartIcon() {
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

/**
 * Первый заход: подбора без поиска не существует (§2.1), поэтому экран не
 * пустой список, а один вопрос, кнопка и объяснение жестов.
 */
function NoSearchState({
  topZones,
  onFind,
  onPickZone,
}: {
  topZones?: TopZoneDTO[];
  onFind?: () => void;
  onPickZone?: (zoneId: string) => void;
}) {
  return (
    <div style={{ ...D.body, flex: '1 1 auto', gap: 18, paddingBottom: 20 }}>
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: 14,
          padding: '18px 16px 20px',
          borderRadius: 18,
          background: 'var(--card-bg)',
          border: '1px solid var(--sep)',
        }}
      >
        <span style={D.h1}>С кем будешь жить?</span>
        <span style={{ fontSize: 14, color: 'var(--tg-theme-subtitle-text-color)', textWrap: 'pretty' }}>
          Покажем тех, кто ищет там же
        </span>
        <button type="button" style={S.filledButton} onClick={onFind}>
          Найти
        </button>
      </div>

      <HowItWorks />

      <TopZonesBlock zones={topZones ?? []} onPick={onPickZone} />
    </div>
  );
}

function HowItWorks() {
  const steps: { title: string; note?: string }[] = [
    { title: 'Сохрани, где ищешь' },
    { title: 'Свайп вправо — „хочу жить вместе“', note: 'влево — пропустить, вернём через неделю' },
    { title: 'Ответный лайк открывает контакты' },
  ];
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 12,
        padding: '16px 16px 18px',
        borderRadius: 18,
        background: 'var(--card-bg)',
        border: '1px solid var(--sep)',
      }}
    >
      <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>
        Как это работает
      </span>
      {steps.map((s, i) => (
        <div key={s.title} style={{ display: 'flex', alignItems: 'flex-start', gap: 10 }}>
          <span
            style={{
              flex: '0 0 auto',
              width: 24,
              height: 24,
              borderRadius: 12,
              background: 'var(--type-search-bg)',
              color: 'var(--type-search-fg)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              fontSize: 13,
              fontWeight: 640,
            }}
          >
            {i + 1}
          </span>
          <span style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0 }}>
            <span style={{ fontSize: 15, fontWeight: 510, textWrap: 'pretty' }}>{s.title}</span>
            {s.note && (
              <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
                {s.note}
              </span>
            )}
          </span>
        </div>
      ))}
    </div>
  );
}

/**
 * «ГДЕ УЖЕ ИЩУТ» — топ живых зон по числу видимых ищущих.
 *
 * Пустой топ не рисуется вовсе: строка «нигде никого» не помогает никому, а
 * кнопка «Найти» выше и так зовёт войти первым в свою зону.
 */
function TopZonesBlock({
  zones,
  onPick,
}: {
  zones: TopZoneDTO[];
  onPick?: (zoneId: string) => void;
}) {
  if (zones.length === 0) return null;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <span style={S.sectionTitle}>ГДЕ УЖЕ ИЩУТ</span>
      {zones.map((z) => (
        <div
          key={z.id}
          onClick={onPick ? () => onPick(z.id) : undefined}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 8,
            padding: '11px 12px',
            borderRadius: 11,
            background: 'var(--card-bg)',
            border: '1px solid var(--sep)',
            cursor: onPick ? 'pointer' : 'default',
          }}
        >
          <span style={{ fontSize: 15, fontWeight: 510 }}>
            {z.name} · {z.people}
          </span>
          <span
            style={{
              marginLeft: 'auto',
              fontSize: 17,
              lineHeight: 1,
              color: 'var(--tg-theme-hint-color)',
            }}
          >
            ›
          </span>
        </div>
      ))}
    </div>
  );
}

/** Конец подбора: вернуть пропущенных или расширить зоны ещё одним поиском. */
function EndState({
  passed,
  slotsUsed,
  slotsTotal,
  onRestorePassed,
  onAddSearch,
}: {
  passed: number;
  slotsUsed: number;
  slotsTotal: number;
  onRestorePassed?: () => void;
  onAddSearch?: () => void;
}) {
  const slot = slotsUsed + 1;
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 14,
        padding: '20px 16px',
        borderRadius: 18,
        background: 'var(--card-bg)',
        border: '1px solid var(--sep)',
      }}
    >
      <span style={D.h1}>Пока все</span>
      {passed > 0 && (
        <button type="button" style={S.secondaryButton} onClick={onRestorePassed}>
          Показать пропущенных · {passed}
        </button>
      )}
      {/* Слотов больше нет — звать в четвёртый поиск нечем: кнопки просто нет. */}
      {slot <= slotsTotal && (
        <button type="button" style={S.dashedButton} onClick={onAddSearch}>
          Сохранить ещё поиск · слот {slot} из {slotsTotal}
        </button>
      )}
    </div>
  );
}

function roomFacts(room: DeckRoom): { label: string; value: ReactNode }[] {
  const facts: { label: string; value: ReactNode }[] = [];
  if (room.zone.name) facts.push({ label: 'Метро', value: room.zone.name });
  if (room.price_share) {
    facts.push({
      label: 'Твоя доля',
      value: `${money(room.price_share)}${room.utilities === 'meters' ? ' + счётчики' : ''}`,
    });
  }
  if (room.isolated != null) {
    facts.push({ label: 'Комната', value: room.isolated ? 'изолированная' : 'проходная' });
  }
  if (room.deposit) facts.push({ label: 'Депозит', value: money(room.deposit) });
  return facts;
}
