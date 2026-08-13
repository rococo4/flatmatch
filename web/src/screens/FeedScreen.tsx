/* Экран «Лента» — выдача по текущим параметрам (§6 v2, search-first).
   Чипы сверху = текущие фильтры просмотра, без кнопок. «Изменить» — в шапке.
   Сохранение поиска живёт только в промо-блоке людей.
   Микрокопия дословная. */
import type { CSSProperties } from 'react';
import type { PointDTO } from '../api/client';
import {
  addedLabel,
  bathroomsLabel,
  dayMonth,
  habitChips,
  lastSeenLabel,
  money,
  plural,
  pointTitle,
  pricePerPerson,
  restrictionLabels,
  utilitiesLabel,
} from '../api/format';
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

/** Шапка «к себе»: стадия автора решает, что на ней написано (§2.2). */
function homeCardTitle(p: PointDTO): string {
  if (p.stage !== 'moving_in') return 'Сосед к себе — уже живёт там';
  return p.move_in ? `Сосед к себе — заезжает ${dayMonth(p.move_in)}` : 'Сосед к себе — заезжает';
}

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
  points,
  error,
  likesRemaining,
  onEditParams,
  onJoinPool,
  onOpenPool,
  onFreeSlot,
  onEnterFlatPool,
  onWantRoom,
  onOpenProfile,
}: {
  zone: string;
  params: string[]; // текущие фильтры просмотра, только чтение
  promo: PromoState;
  points: PointDTO[];
  /** Отказ сохранения поиска — строкой над промо-блоком, а не молчанием. */
  error?: string | null;
  /** Остаток дорогих лайков для подписи «N из 5» под «Хочу эту комнату». */
  likesRemaining?: number;
  onEditParams?: () => void;
  onJoinPool?: () => void;
  onOpenPool?: () => void;
  /** «Освободить слот» из состояния full — в «Мои поиски». */
  onFreeSlot?: () => void;
  onEnterFlatPool?: (id: string) => void;
  /** «Хочу эту комнату» — дорогой лайк автору «к себе» прямо с карточки. */
  onWantRoom?: (point: PointDTO) => void;
  /** Тап по строке автора → полный профиль с анкетой и «о себе». */
  onOpenProfile?: (point: PointDTO) => void;
}) {
  return (
    <div style={L.screen}>
      <div style={L.header}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 1, minWidth: 0 }}>
          <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>{zone}</span>
          <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
            {params.length} {plural(params.length, 'параметр', 'параметра', 'параметров')} поиска
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
          {error && (
            <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>
              {error}
            </span>
          )}
          <PoolPromoBlock
            data={promo}
            onJoin={onJoinPool}
            onOpenPool={onOpenPool}
            onFreeSlot={onFreeSlot}
          />

          {points.length === 0 ? (
            <PointsSoon />
          ) : (
            points.map((p) =>
              p.type === 'flat' ? (
                <FlatPointCard key={p.id} point={p} onEnterPool={onEnterFlatPool} />
              ) : (
                <HomePointCard
                  key={p.id}
                  point={p}
                  likesRemaining={likesRemaining}
                  onWantRoom={onWantRoom}
                  onOpenProfile={onOpenProfile}
                />
              ),
            )
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Место точек, пока их нет.
 *
 * Ни одной выдуманной квартиры: пустой список — это пустое состояние, а не
 * повод показать демо-карточку. Тот же блок стоит на экране входа в ленту,
 * где параметров ещё нет и спрашивать нечего.
 */
export function PointsSoon() {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8, opacity: 0.6 }}>
      <PhotoPlaceholder label="скрин объявления" style={{ aspectRatio: '16/9', borderRadius: 10 }} />
      <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>
        Квартиры появятся здесь — сейчас собираем
      </span>
    </div>
  );
}

/** Квартира с рынка: пул есть, вход дешёвый — secondary. */
function FlatPointCard({
  point,
  onEnterPool,
}: {
  point: PointDTO;
  onEnterPool?: (id: string) => void;
}) {
  const share = pricePerPerson(point.price_total, point.rooms);
  return (
    <article style={S.card}>
      <div style={{ ...S.cardHeadBase, background: 'var(--type-flat-bg)', color: 'var(--type-flat-fg)' }}>
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>Квартира с рынка</span>
        <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>
          {addedLabel(point.created_days_ago)}
        </span>
      </div>
      <div style={S.cardBody}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <PointPhoto url={point.photo_url} label="скрин объявления" />
          <span style={S.title}>{pointTitle(point)}</span>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            {/* Цена доли — только когда есть на что делить: комнатность
                объявление называет не всегда, а придумывать «на двоих» за него
                нельзя. */}
            {share !== null ? (
              <>
                <span style={{ fontSize: 20, fontWeight: 640, letterSpacing: '-0.4px' }}>
                  {money(share)}{' '}
                  <span
                    style={{ fontSize: 14, fontWeight: 400, color: 'var(--tg-theme-subtitle-text-color)' }}
                  >
                    с человека
                  </span>
                </span>
                <span style={S.subtitle}>{money(point.price_total ?? 0)} / мес за квартиру</span>
              </>
            ) : (
              <span style={{ fontSize: 20, fontWeight: 640, letterSpacing: '-0.4px' }}>
                {money(point.price_total ?? 0)}{' '}
                <span
                  style={{ fontSize: 14, fontWeight: 400, color: 'var(--tg-theme-subtitle-text-color)' }}
                >
                  / мес за квартиру
                </span>
              </span>
            )}
          </div>
        </div>
        <FactsBlock title="Квартира" facts={flatFacts(point)}>
          <GreyChips items={restrictionLabels(point.restrictions, 'flat')} />
        </FactsBlock>
        <PoolCount
          n={point.pool_count}
          kind="flat"
          hint={point.in_pool ? undefined : 'увидишь после входа'}
        />
        {point.in_pool ? (
          <InPoolRow />
        ) : (
          <PoolEntryButton
            label="Интересно — войти в пул"
            caption="Вход бесплатный и безлимитный"
            onClick={() => onEnterPool?.(point.id)}
          />
        )}
      </div>
    </article>
  );
}

/** Сосед к себе: пула нет. Счётчик откликов + дорогой лайк автору (filled).
 *  Тап по строке человека — полный профиль (тот же экран, что из пула). */
function HomePointCard({
  point,
  likesRemaining,
  onWantRoom,
  onOpenProfile,
}: {
  point: PointDTO;
  likesRemaining?: number;
  onWantRoom?: (point: PointDTO) => void;
  onOpenProfile?: (point: PointDTO) => void;
}) {
  const author = point.author;
  // Витрина «к себе» — это автор: без него карточке нечего показывать (§2.2).
  if (!author) return null;

  return (
    <article style={S.card}>
      <div style={{ ...S.cardHeadBase, background: 'var(--type-home-bg)', color: 'var(--type-home-fg)' }}>
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>
          {homeCardTitle(point)}
        </span>
        <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>
          {lastSeenLabel(author.last_seen)}
        </span>
      </div>
      <div style={S.cardBody}>
        <div
          onClick={onOpenProfile ? () => onOpenProfile(point) : undefined}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            padding: 8,
            margin: -8,
            borderRadius: 11,
            cursor: onOpenProfile ? 'pointer' : 'default',
          }}
        >
          <PersonRow
            name={author.name}
            age={author.age}
            occupation=""
            photoUrl={author.photo_url || undefined}
          />
          {onOpenProfile && (
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
          )}
        </div>
        <FactsBlock title={`Комната у ${author.name}`} facts={homeFacts(point)}>
          <PointPhoto url={point.photo_url} label="фото комнаты" inCard />
          <GreyChips items={restrictionLabels(point.restrictions, 'intent_home')} />
        </FactsBlock>
        <HabitsBlock items={habitChips(author)} />
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
            {point.responses} {plural(point.responses, 'отклик', 'отклика', 'откликов')}
          </span>
        </div>
        {/* Кнопка появляется вместе с обработчиком: дорогой лайк с карточки —
            это §12 шаг 5 в части точек, и рисовать её мёртвой незачем. */}
        {onWantRoom && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
            <button type="button" style={S.filledButton} onClick={() => onWantRoom(point)}>
              Хочу эту комнату
            </button>
            {likesRemaining != null && (
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <LimitChip small>{likesRemaining} из 5</LimitChip>
              </div>
            )}
          </div>
        )}
      </div>
    </article>
  );
}

/** Фото точки. Пустой адрес — заглушка: карточка без фото всё равно карточка. */
function PointPhoto({ url, label, inCard }: { url: string; label: string; inCard?: boolean }) {
  const style: CSSProperties = { aspectRatio: '16/9', borderRadius: inCard ? 8 : 10 };
  if (!url) return <PhotoPlaceholder inCard={inCard} label={label} style={style} />;
  return (
    <img
      src={url}
      alt=""
      style={{ ...style, width: '100%', objectFit: 'cover', display: 'block' }}
    />
  );
}

/** Ограничения — серые чипы: их читают до лайка, а не после мэтча (§2.2). */
function GreyChips({ items }: { items: string[] }) {
  if (items.length === 0) return null;
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingTop: 2 }}>
      {items.map((l) => (
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
  );
}

/** Вошёл — кнопки больше нет: вход одноразовый, выход живёт в самом пуле (§6). */
function InPoolRow() {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 7,
        padding: 12,
        borderRadius: 10,
        background: 'var(--verdict-ok-bg)',
        color: 'var(--verdict-ok-fg)',
        fontSize: 15,
        fontWeight: 590,
      }}
    >
      ✓ Ты в пуле
    </div>
  );
}

function flatFacts(p: PointDTO): Fact[] {
  const facts: Fact[] = [];
  if (p.zone.name) facts.push({ label: 'Метро', value: p.zone.name });
  if (p.rooms) {
    facts.push({
      label: 'Комнаты',
      value: p.isolated == null ? String(p.rooms) : `${p.rooms}, ${p.isolated ? 'изолированные' : 'проходные'}`,
    });
  }
  if (p.bathrooms) facts.push({ label: 'Санузлы', value: bathroomsLabel(p.bathrooms) });
  if (p.utilities) facts.push({ label: 'Коммуналка', value: utilitiesLabel(p.utilities) });
  if (p.deposit) facts.push({ label: 'Депозит', value: money(p.deposit) });
  return facts;
}

function homeFacts(p: PointDTO): Fact[] {
  const facts: Fact[] = [];
  if (p.price_share) {
    facts.push({ label: 'Цена доли', value: <PerPerson>{money(p.price_share)}</PerPerson> });
  }
  if (p.zone.name) facts.push({ label: 'Метро', value: p.zone.name });
  if (p.isolated != null) {
    facts.push({ label: 'Комната', value: p.isolated ? 'изолированная' : 'проходная' });
  }
  if (p.deposit) facts.push({ label: 'Депозит', value: money(p.deposit) });
  if (p.move_in) facts.push({ label: 'Заезд', value: `с ${dayMonth(p.move_in)}` });
  return facts;
}
