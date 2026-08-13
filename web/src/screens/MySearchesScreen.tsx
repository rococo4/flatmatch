/* Вкладка «Мои поиски» — три блока (§6):
   1) Мои поиски (до 3),
   2) Присмотренные квартиры · N из 5 (точки с рынка) + вход в форму,
   3) Зову к себе · N из 2 (зелёный акцент): точки «к себе» со стадией и откликами.
   Отдельного экрана выбора типа нет — выбор через кнопки блоков.
   Отклики автор смотрит в общем «Тебя лайкнули» (пометка «на твою комнату»). */
import { useState } from 'react';
import type { CSSProperties } from 'react';
import type { PointDTO } from '../api/client';
import {
  addedLabel,
  money,
  plural,
  pointStatusLabel,
  pointTitle,
  pricePerPerson,
  stageBadge,
} from '../api/format';
import { Badge, LimitChip, PhotoPlaceholder, S } from '../ui/primitives';

export type SavedSearch = {
  id: string;
  zone: string;
  since: string; // «в пуле с 12 июля»
  params: string[];
  peopleCount: number; // «6 ищут»
  fullMatchCount: number; // «с 3 полное совпадение»
  pointsLine: string; // «4 квартиры и 1 комната под эти параметры»; пусто — строки нет
  newCount?: number; // бейдж «1 новый» / «M новых»
};

/** Закрытый или поставленный на паузу поиск. Возвращается одним тапом (§3). */
export type ArchivedSearch = {
  id: string;
  zone: string;
  params: string[];
  /** «нашёл здесь» / «нашёл в другом месте» / «передумал»; пусто — пауза по молчанию. */
  reason: string;
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
    gap: 18,
    padding: '14px 12px 20px',
  } as CSSProperties,
  section: { display: 'flex', flexDirection: 'column', gap: 10 } as CSSProperties,
  sectionHead: {
    display: 'flex',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    gap: 8,
  } as CSSProperties,
  addLink: {
    fontSize: 13,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
    background: 'none',
    border: 'none',
    font: 'inherit',
  } as CSSProperties,
  plainCard: {
    borderRadius: 14,
    padding: 12,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
  } as CSSProperties,
  emptyCard: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    padding: '14px 12px',
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  actions: { display: 'flex', alignItems: 'center', gap: 16, paddingTop: 2 } as CSSProperties,
  actionLink: {
    fontSize: 13,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
    background: 'none',
    border: 'none',
    font: 'inherit',
  } as CSSProperties,
  destructiveLink: {
    fontSize: 13,
    fontWeight: 510,
    color: 'var(--tg-theme-destructive-text-color)',
    cursor: 'pointer',
    background: 'none',
    border: 'none',
    font: 'inherit',
  } as CSSProperties,
  emptyTitle: { fontSize: 15, fontWeight: 590, textWrap: 'pretty' } as CSSProperties,
};

const ENTRY = {
  flat: {
    title: 'Присмотрел квартиру? Найди, с кем её снять',
    hint: 'ссылка на Авито или Циан — соберём пул',
  },
  home: {
    title: 'Ищу соседа к себе',
    hint: 'уже живёшь или заезжаешь — комната свободна',
  },
};

/** Вход в форму создания. Цвет квадрата — тип будущей точки. */
function CreateEntry({ tone, onClick }: { tone: 'flat' | 'home'; onClick?: () => void }) {
  const accent = tone === 'flat' ? 'var(--tg-theme-button-color)' : 'var(--type-home-fg)';
  const titleColor = tone === 'flat' ? 'var(--tg-theme-accent-text-color)' : 'var(--type-home-fg)';
  const badgeBg = tone === 'flat' ? 'var(--type-flat-bg)' : 'var(--type-home-bg)';
  const badgeFg = tone === 'flat' ? 'var(--type-flat-fg)' : 'var(--type-home-fg)';
  const { title, hint } = ENTRY[tone];
  return (
    <button
      type="button"
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 11,
        width: '100%',
        padding: '13px 12px',
        border: `1px dashed color-mix(in srgb, ${accent} 55%, transparent)`,
        borderRadius: 12,
        background: 'transparent',
        font: 'inherit',
        textAlign: 'left',
        cursor: 'pointer',
      }}
    >
      <span
        style={{
          flex: '0 0 auto',
          width: 30,
          height: 30,
          borderRadius: 9,
          background: badgeBg,
          color: badgeFg,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: 16,
          lineHeight: 1,
        }}
      >
        ＋
      </span>
      <span style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0 }}>
        <span style={{ fontSize: 15, fontWeight: 590, color: titleColor }}>{title}</span>
        <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>{hint}</span>
      </span>
    </button>
  );
}

export function MySearchesScreen({
  searches,
  searchSlotsTotal,
  archived,
  error,
  flats,
  enteredFlats,
  flatSlotsTotal,
  homes,
  homeSlotsTotal,
  onOpenSearch,
  onOpenPool,
  onEditSearch,
  onCloseSearch,
  onAddSearch,
  onReactivate,
  onOpenFlatPool,
  onSharePoint,
  onClosePoint,
  onCreateFlatPoint,
  onCreateHomePoint,
}: {
  /** Только активные: иначе «N ИЗ 3» перестаёт быть счётчиком слотов. */
  searches: SavedSearch[];
  searchSlotsTotal: number;
  archived: ArchivedSearch[];
  /** Отказ последнего действия с поиском — обычно «поиски заняты 3 из 3» от сервера. */
  error?: string | null;
  /** Свои квартиры с рынка: только они и считаются в лимит пяти. */
  flats: PointDTO[];
  /** Чужие пулы, в которые вошёл: лимита у них нет, слоты они не занимают. */
  enteredFlats: PointDTO[];
  flatSlotsTotal: number;
  homes: PointDTO[];
  homeSlotsTotal: number;
  /** → выдача поиска (лента с его параметрами) */
  onOpenSearch?: (id: string) => void;
  /** → зонный пул этого поиска */
  onOpenPool?: (id: string) => void;
  onEditSearch?: (id: string) => void;
  onCloseSearch?: (id: string) => void;
  onAddSearch?: () => void;
  onReactivate?: (id: string) => void;
  /** → пул квартиры (список вошедших) */
  onOpenFlatPool?: (id: string) => void;
  /** «Позвать в чат» — диплинк на свою квартиру (§6, вирусная петля). */
  onSharePoint?: (id: string) => void;
  /** Закрытие своей точки. Причин у неё нет — в отличие от поиска (§3). */
  onClosePoint?: (id: string) => void;
  /** → форма «Найди со-съёмщика на свою квартиру» */
  onCreateFlatPoint?: () => void;
  /** → форма «К себе», сразу на гейте стадии */
  onCreateHomePoint?: () => void;
}) {
  // Отказ по слотам показывается только после тапа: висящая плашка «занято»
  // над списком из трёх карточек и так очевидна, а до тапа она просто шум.
  const [slotsNotice, setSlotsNotice] = useState(false);
  const slotsFull = searches.length >= searchSlotsTotal;

  return (
    <div style={L.screen}>
      <div style={L.header}>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Мои поиски</span>
      </div>

      <div style={L.body}>
        {/* 1. Поиски людей */}
        <div style={L.section}>
          <div style={L.sectionHead}>
            <span style={S.sectionTitle}>
              МОИ ПОИСКИ · {searches.length} ИЗ {searchSlotsTotal}
            </span>
            <button
              type="button"
              style={L.addLink}
              onClick={() => (slotsFull ? setSlotsNotice(true) : onAddSearch?.())}
            >
              + поиск
            </button>
          </div>

          {/* Одно место для отказов по слотам: и своего («+ поиск» при трёх
              активных), и серверного — из закрытия или реактивации. */}
          {(error || (slotsNotice && slotsFull)) && (
            <div>
              <LimitChip>
                {error ?? `поиски заняты ${searchSlotsTotal} из ${searchSlotsTotal}`}
              </LimitChip>
            </div>
          )}

          {searches.map((s) => (
            <article key={s.id} style={L.plainCard}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <span style={S.title}>{s.zone}</span>
                {!!s.newCount && (
                  <Badge tone="search">
                    {s.newCount} {plural(s.newCount, 'новый', 'новых', 'новых')}
                  </Badge>
                )}
                <span style={{ marginLeft: 'auto', fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
                  {s.since}
                </span>
              </div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                {s.params.map((p) => (
                  <span key={p} style={{ ...S.chip, background: 'var(--app-bg)' }}>
                    {p}
                  </span>
                ))}
              </div>
              <span style={{ fontSize: 15, fontWeight: 590 }}>
                {s.peopleCount} ищут · с {s.fullMatchCount} полное совпадение
              </span>
              <button type="button" style={S.secondaryButton} onClick={() => onOpenPool?.(s.id)}>
                Смотреть пул · {s.peopleCount} {plural(s.peopleCount, 'человек', 'человека', 'человек')}
              </button>
              {/* Строка точек — отдельный переход в выдачу поиска. Пустая
                  строка = точек нет, и строки тогда нет вовсе: вести в ноль
                  некуда. */}
              {s.pointsLine && (
                <div
                  onClick={() => onOpenSearch?.(s.id)}
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 8,
                    padding: '9px 11px',
                    borderRadius: 10,
                    background: 'var(--app-bg)',
                    cursor: 'pointer',
                  }}
                >
                  <span
                    style={{
                      flex: '1 1 auto',
                      fontSize: 13,
                      color: 'var(--tg-theme-subtitle-text-color)',
                      minWidth: 0,
                    }}
                  >
                    {s.pointsLine}
                  </span>
                  <span style={{ flex: '0 0 auto', fontSize: 17, lineHeight: 1, color: 'var(--tg-theme-hint-color)' }}>
                    ›
                  </span>
                </div>
              )}
              <div style={L.actions}>
                <button type="button" style={L.actionLink} onClick={() => onEditSearch?.(s.id)}>
                  Редактировать
                </button>
                <button type="button" style={L.destructiveLink} onClick={() => onCloseSearch?.(s.id)}>
                  Закрыть поиск
                </button>
              </div>
            </article>
          ))}
        </div>

        {/* Архив: закрытые и поставленные на паузу. Пустого состояния нет —
            секция появляется, только когда в архиве что-то есть. */}
        {archived.length > 0 && (
          <div style={L.section}>
            <span style={S.sectionTitle}>АРХИВ · {archived.length}</span>
            {archived.map((a) => (
              <article key={a.id} style={{ ...L.plainCard, opacity: 0.6 }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span style={S.title}>{a.zone}</span>
                  {a.reason && (
                    <span style={{ marginLeft: 'auto', fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
                      {a.reason}
                    </span>
                  )}
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {a.params.map((p) => (
                    <span key={p} style={{ ...S.chip, background: 'var(--app-bg)' }}>
                      {p}
                    </span>
                  ))}
                </div>
                <button type="button" style={S.secondaryButton} onClick={() => onReactivate?.(a.id)}>
                  Вернуть в поиск
                </button>
              </article>
            ))}
          </div>
        )}

        {/* 2. Присмотренные квартиры с рынка: свои считаются в лимит, чужие
            пулы стоят рядом без счёта — вход в них бесплатен и безлимитен. */}
        <div style={L.section}>
          <span style={S.sectionTitle}>
            ПРИСМОТРЕННЫЕ КВАРТИРЫ · {flats.length} ИЗ {flatSlotsTotal}
          </span>

          {flats.length === 0 && enteredFlats.length === 0 ? (
            <div style={L.emptyCard}>
              <span style={L.emptyTitle}>Присмотрел вариант на Авито? Собери на него людей</span>
              <CreateEntry tone="flat" onClick={onCreateFlatPoint} />
            </div>
          ) : (
            <>
              {flats.map((p) => (
                <FlatRow
                  key={p.id}
                  point={p}
                  mine
                  onOpenPool={onOpenFlatPool}
                  onShare={onSharePoint}
                  onClose={onClosePoint}
                />
              ))}
              {enteredFlats.map((p) => (
                <FlatRow key={p.id} point={p} onOpenPool={onOpenFlatPool} />
              ))}
              {flats.length < flatSlotsTotal && <CreateEntry tone="flat" onClick={onCreateFlatPoint} />}
            </>
          )}
        </div>

        {/* 3. Зову к себе — точки «к себе», свой лимит слотов */}
        <div style={L.section}>
          <span style={S.sectionTitle}>
            ЗОВУ К СЕБЕ · {homes.length} ИЗ {homeSlotsTotal}
          </span>

          {homes.length === 0 ? (
            <div style={L.emptyCard}>
              <span style={L.emptyTitle}>Живёшь в квартире или заезжаешь — найди соседа</span>
              <CreateEntry tone="home" onClick={onCreateHomePoint} />
            </div>
          ) : (
            <>
              {homes.map((h) => (
                <HomeRow key={h.id} point={h} onClose={onClosePoint} />
              ))}
              {homes.length < homeSlotsTotal && <CreateEntry tone="home" onClick={onCreateHomePoint} />}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Строка квартиры в «Присмотренных».
 *
 * Своя и чужая различаются пометкой и действиями: у обеих один и тот же пул, но
 * звать в чат и закрывать можно только свою. Правки нет ни у той, ни у другой —
 * редактирования точек в продукте нет вовсе (§6).
 */
function FlatRow({
  point,
  mine,
  onOpenPool,
  onShare,
  onClose,
}: {
  point: PointDTO;
  mine?: boolean;
  onOpenPool?: (id: string) => void;
  onShare?: (id: string) => void;
  onClose?: (id: string) => void;
}) {
  const share = pricePerPerson(point.price_total, point.rooms);
  return (
    <article style={S.card}>
      <div style={{ ...S.cardHeadBase, background: 'var(--type-flat-bg)', color: 'var(--type-flat-fg)' }}>
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>
          {mine ? 'Квартира с рынка' : 'Я в пуле'}
        </span>
        <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>
          {addedLabel(point.created_days_ago)}
        </span>
      </div>
      <div style={{ ...S.cardBody, gap: 10 }}>
        <div style={{ display: 'flex', gap: 10 }}>
          <PointThumb url={point.photo_url} label="скрин" />
          <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
            <span style={{ fontSize: 15, fontWeight: 590 }}>{pointTitle(point)}</span>
            <span style={S.subtitle}>
              {share !== null ? `${money(share)} с человека` : money(point.price_total ?? 0)}
            </span>
            <span style={{ fontSize: 13, fontWeight: 510 }}>в пуле {point.pool_count}</span>
          </div>
        </div>
        <StatusRow status={point.status} />
        <button type="button" style={S.secondaryButton} onClick={() => onOpenPool?.(point.id)}>
          Смотреть пул · {point.pool_count}{' '}
          {plural(point.pool_count, 'человек', 'человека', 'человек')}
        </button>
        {mine && (
          <PointActions
            id={point.id}
            onShare={onShare}
            onClose={onClose}
            question="Закрыть квартиру?"
          />
        )}
      </div>
    </article>
  );
}

/** Строка «к себе» в своём блоке: стадия, цена доли, отклики, закрытие. */
function HomeRow({ point, onClose }: { point: PointDTO; onClose?: (id: string) => void }) {
  return (
    <article style={S.card}>
      <div
        style={{ ...S.cardHeadBase, background: 'var(--type-home-bg)', color: 'var(--type-home-fg)' }}
      >
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>Сосед к себе</span>
        <span style={{ fontSize: 12, fontWeight: 590, opacity: 0.9 }}>
          {point.stage ? stageBadge(point.stage) : ''}
        </span>
      </div>
      <div style={{ ...S.cardBody, gap: 10 }}>
        <div style={{ display: 'flex', gap: 10 }}>
          <PointThumb url={point.photo_url} label="фото" />
          <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
            <span style={{ fontSize: 15, fontWeight: 590 }}>{pointTitle(point)}</span>
            <span style={S.subtitle}>{money(point.price_share ?? 0)} с человека</span>
            <span style={{ fontSize: 13, fontWeight: 510 }}>
              {point.responses} {plural(point.responses, 'отклик', 'отклика', 'откликов')}
            </span>
          </div>
        </div>
        <StatusRow status={point.status} />
        {/* Отклики автор смотрит в общем «Тебя лайкнули» — своего экрана
            разбора у него нет by design (§2.2). */}
        <PointActions id={point.id} onClose={onClose} question="Закрыть точку?" />
      </div>
    </article>
  );
}

/**
 * Действия над своей точкой.
 *
 * Подтверждение закрытия — вторым тапом прямо в карточке, а не шторкой: у
 * поиска шторка спрашивает ПРИЧИНУ (она же метрика продукта, §3), а у точки
 * причин нет, и шторка ради одного «точно?» была бы тяжелее самого действия.
 */
function PointActions({
  id,
  question,
  onShare,
  onClose,
}: {
  id: string;
  question: string;
  onShare?: (id: string) => void;
  onClose?: (id: string) => void;
}) {
  const [confirming, setConfirming] = useState(false);
  if (!onShare && !onClose) return null;

  return (
    <div style={L.actions}>
      {onShare && (
        <button type="button" style={L.actionLink} onClick={() => onShare(id)}>
          Позвать в чат
        </button>
      )}
      {onClose &&
        (confirming ? (
          <>
            <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
              {question}
            </span>
            <button type="button" style={L.destructiveLink} onClick={() => onClose(id)}>
              Закрыть
            </button>
            <button type="button" style={L.actionLink} onClick={() => setConfirming(false)}>
              Отмена
            </button>
          </>
        ) : (
          <button
            type="button"
            style={{ ...L.destructiveLink, marginLeft: 'auto' }}
            onClick={() => setConfirming(true)}
          >
            Закрыть
          </button>
        ))}
    </div>
  );
}

/** Статус своей точки — только когда есть что сказать: активная и так в ленте. */
function StatusRow({ status }: { status: PointDTO['status'] }) {
  const label = pointStatusLabel(status);
  if (!label) return null;
  return (
    <div>
      <LimitChip>{label}</LimitChip>
    </div>
  );
}

function PointThumb({ url, label }: { url: string; label: string }) {
  const style: CSSProperties = { flex: '0 0 72px', width: 72, height: 54, borderRadius: 8 };
  if (!url) return <PhotoPlaceholder label={label} style={style} />;
  return <img src={url} alt="" style={{ ...style, objectFit: 'cover', display: 'block' }} />;
}
