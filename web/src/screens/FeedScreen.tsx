/* Экран «Лента» — выдача по текущим параметрам (§6 v2, search-first).
   Чипы сверху = текущие фильтры просмотра, без кнопок. «Изменить» — в шапке.
   Сохранение поиска живёт только в промо-блоке людей.
   Микрокопия дословная. */
import type { CSSProperties } from 'react';
import type { PointDTO } from '../api/client';
import {
  addedLabel,
  dayMonth,
  flatFacts,
  habitChips,
  lastSeenLabel,
  money,
  occupationLabel,
  plural,
  pointTitle,
  pricePerPerson,
  restrictionLabels,
} from '../api/format';
import {
  FactsBlock,
  GreyChips,
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
  myUserId,
  respondedPoints,
  onEditParams,
  onJoinPool,
  onOpenPool,
  onFreeSlot,
  onEnterFlatPool,
  onOpenFlat,
  onWantRoom,
  onOpenProfile,
}: {
  zone: string;
  params: string[]; // текущие фильтры просмотра, только чтение
  promo: PromoState;
  points: PointDTO[];
  /** Отказ сохранения поиска или отклика — строкой над списком, а не молчанием. */
  error?: string | null;
  /** Остаток дорогих лайков для подписи «N из 5» под «Хочу эту комнату». */
  likesRemaining?: number;
  /** Своё «к себе» в своей же ленте: откликаться самому себе не на что. */
  myUserId?: string;
  /** Точки, на которые отклик уже улетел в этой сессии: кнопка отработала. */
  respondedPoints?: string[];
  onEditParams?: () => void;
  onJoinPool?: () => void;
  onOpenPool?: () => void;
  /** «Освободить слот» из состояния full — в «Мои поиски». */
  onFreeSlot?: () => void;
  onEnterFlatPool?: (id: string) => void;
  /** Тап по карточке квартиры → её карточка целиком и пул. */
  onOpenFlat?: (point: PointDTO) => void;
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
          Изменить
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
                <FlatPointCard
                  key={p.id}
                  point={p}
                  onEnterPool={onEnterFlatPool}
                  onOpenFlat={onOpenFlat}
                />
              ) : (
                <HomePointCard
                  key={p.id}
                  point={p}
                  likesRemaining={likesRemaining}
                  mine={p.author?.user_id != null && p.author.user_id === myUserId}
                  responded={respondedPoints?.includes(p.id)}
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
  onOpenFlat,
}: {
  point: PointDTO;
  onEnterPool?: (id: string) => void;
  onOpenFlat?: (point: PointDTO) => void;
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
        {/* Фото и цены — вход в карточку целиком: там и параметры, и ссылка на
            оригинал, и сам пул. Кнопка входа остаётся отдельным жестом. */}
        <div
          onClick={onOpenFlat ? () => onOpenFlat(point) : undefined}
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 10,
            cursor: onOpenFlat ? 'pointer' : 'default',
          }}
        >
          <PointPhoto url={point.photo_urls[0] ?? ''} label="скрин объявления" />
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
        {/* Счётчик считает других: вошедшего в своём пуле в нём нет, и «0»
            без подписи выглядел бы поломкой рядом с «✓ Ты в пуле». */}
        <PoolCount
          n={point.pool_count}
          kind="flat"
          hint={
            point.in_pool
              ? point.pool_count === 0
                ? 'пока только ты'
                : undefined
              : 'увидишь после входа'
          }
        />
        {point.in_pool ? (
          <InPoolRow onOpenPool={onOpenFlat ? () => onOpenFlat(point) : undefined} poolCount={point.pool_count} />
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
  mine,
  responded,
  onWantRoom,
  onOpenProfile,
}: {
  point: PointDTO;
  likesRemaining?: number;
  /** Своя точка: откликаться самому себе не на что, кнопки нет. */
  mine?: boolean;
  responded?: boolean;
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
            occupation={occupationLabel(author.occupation)}
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
          <PointPhoto url={point.photo_urls[0] ?? ''} label="фото комнаты" inCard />
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
        {/* Своя точка кнопки не несёт: отклик самому себе бессмыслен, и сервер
            его всё равно отобьёт. Остаток показываем, только когда он известен:
            своего запроса за бюджетом у ленты нет. */}
        {onWantRoom && !mine && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
            <button
              type="button"
              style={responded ? S.secondaryButton : S.filledButton}
              disabled={responded}
              onClick={() => onWantRoom(point)}
            >
              {responded ? 'Отклик отправлен' : 'Хочу эту комнату'}
            </button>
            {!responded && likesRemaining != null && (
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

/** Вошёл — кнопки больше нет: вход одноразовый, выход живёт в самом пуле (§6). */
function InPoolRow({ onOpenPool, poolCount }: { onOpenPool?: () => void; poolCount: number }) {
  return (
    <div
      onClick={onOpenPool}
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 7,
        padding: 12,
        borderRadius: 10,
        background: 'var(--verdict-ok-bg)',
        color: 'var(--verdict-ok-fg)',
        fontSize: 15,
        fontWeight: 590,
        cursor: onOpenPool ? 'pointer' : 'default',
      }}
    >
      <span>✓ Ты в пуле</span>
      {onOpenPool && (
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
          Смотреть пул{poolCount > 0 ? ` · ${poolCount}` : ''}
          <span aria-hidden style={{ fontSize: 13 }}>›</span>
        </span>
      )}
    </div>
  );
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
