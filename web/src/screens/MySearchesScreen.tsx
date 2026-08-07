/* Вкладка «Мои поиски» — три блока (§6):
   1) Мои поиски (до 3),
   2) Присмотренные квартиры · N из 5 (точки с рынка) + вход в форму,
   3) Зову к себе · N из 2 (зелёный акцент): точки «к себе» со стадией и откликами.
   Отдельного экрана выбора типа нет — выбор через кнопки блоков.
   Отклики автор смотрит в общем «Тебя лайкнули» (пометка «на твою комнату»). */
import { useState } from 'react';
import type { CSSProperties } from 'react';
import { plural } from '../api/format';
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

export type MarketFlat = {
  id: string;
  title: string;
  pricePerPerson: string;
  addedAt: string;
  poolCount: number;
  change?: string; // жёлтая плашка изменения точки
  /** Участники пула отметили квартиру сданной — точка скрыта из ленты. */
  hiddenAsRented?: boolean;
};

export type HomePoint = {
  id: string;
  title: string; // «Комната · м. Сокол»
  pricePerPerson: string;
  stage: 'living' | 'moving';
  /** «живёт здесь» / «заезжает · ищет со-съёмщика со старта» */
  stageBadge: string;
  responseCount: number; // «N откликов»
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
  changeBanner: {
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '8px 10px',
    borderRadius: 9,
    background: 'var(--warning-bg)',
    color: 'var(--warning-fg)',
    fontSize: 13,
    fontWeight: 590,
    lineHeight: 1.35,
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
  onRemoveFlat,
  onConfirmAlive,
  onOpenIncomingLikes,
  onEditHome,
  onRemoveHome,
  onCreateFlatPoint,
  onCreateHomePoint,
}: {
  /** Только активные: иначе «N ИЗ 3» перестаёт быть счётчиком слотов. */
  searches: SavedSearch[];
  searchSlotsTotal: number;
  archived: ArchivedSearch[];
  /** Отказ последнего действия с поиском — обычно «поиски заняты 3 из 3» от сервера. */
  error?: string | null;
  flats: MarketFlat[];
  flatSlotsTotal: number;
  homes: HomePoint[];
  homeSlotsTotal: number;
  /** → выдача поиска (лента с его параметрами) */
  onOpenSearch?: (id: string) => void;
  /** → зонный пул этого поиска */
  onOpenPool?: (id: string) => void;
  onEditSearch?: (id: string) => void;
  onCloseSearch?: (id: string) => void;
  onAddSearch?: () => void;
  onReactivate?: (id: string) => void;
  onOpenFlatPool?: (id: string) => void;
  onRemoveFlat?: (id: string) => void;
  /** «Проверил — она живая»: снимает скрытие по сигналам пула. */
  onConfirmAlive?: (id: string) => void;
  /** Отклики на комнату живут в общем «Тебя лайкнули» (пометка «на твою комнату»). */
  onOpenIncomingLikes?: () => void;
  onEditHome?: (id: string) => void;
  onRemoveHome?: (id: string) => void;
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

        {/* 2. Присмотренные квартиры с рынка */}
        <div style={L.section}>
          <span style={S.sectionTitle}>
            ПРИСМОТРЕННЫЕ КВАРТИРЫ · {flats.length} ИЗ {flatSlotsTotal}
          </span>

          {flats.length === 0 ? (
            <div style={L.emptyCard}>
              <span style={L.emptyTitle}>Присмотрел вариант на Авито? Собери на него людей</span>
              <CreateEntry tone="flat" onClick={onCreateFlatPoint} />
            </div>
          ) : (
            <>
              {flats.map((f) => (
                <article key={f.id} style={S.card}>
                  <div
                    style={{
                      ...S.cardHeadBase,
                      background: 'var(--type-flat-bg)',
                      color: 'var(--type-flat-fg)',
                    }}
                  >
                    <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>
                      Квартира с рынка
                    </span>
                    <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>{f.addedAt}</span>
                  </div>
                  <div style={{ ...S.cardBody, gap: 10 }}>
                    <div style={{ display: 'flex', gap: 10 }}>
                      <PhotoPlaceholder
                        label="скрин"
                        style={{ flex: '0 0 72px', height: 54, borderRadius: 8 }}
                      />
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
                        <span style={{ fontSize: 15, fontWeight: 590 }}>{f.title}</span>
                        <span style={S.subtitle}>{f.pricePerPerson}</span>
                        <span style={{ fontSize: 13, fontWeight: 510 }}>в пуле {f.poolCount}</span>
                      </div>
                    </div>
                    {f.change && <div style={L.changeBanner}>{f.change}</div>}
                    {f.hiddenAsRented && (
                      <div
                        style={{
                          display: 'flex',
                          flexDirection: 'column',
                          gap: 8,
                          padding: 10,
                          borderRadius: 10,
                          background: 'var(--warning-bg)',
                          color: 'var(--warning-fg)',
                        }}
                      >
                        <span style={{ fontSize: 13, fontWeight: 590, lineHeight: 1.35 }}>
                          Скрыта: участники пула отметили, что сдана
                        </span>
                        <button
                          type="button"
                          onClick={() => onConfirmAlive?.(f.id)}
                          style={{
                            alignSelf: 'flex-start',
                            padding: '8px 12px',
                            border: 'none',
                            borderRadius: 9,
                            background: 'var(--card-bg)',
                            font: 'inherit',
                            fontSize: 13,
                            fontWeight: 590,
                            color: 'var(--tg-theme-accent-text-color)',
                            cursor: 'pointer',
                          }}
                        >
                          Проверил — она живая
                        </button>
                      </div>
                    )}
                    <button type="button" style={S.secondaryButton} onClick={() => onOpenFlatPool?.(f.id)}>
                      Смотреть пул · {f.poolCount}{' '}
                      {plural(f.poolCount, 'человек', 'человека', 'человек')}
                    </button>
                    <div style={L.actions}>
                      <button type="button" style={L.destructiveLink} onClick={() => onRemoveFlat?.(f.id)}>
                        Закрыть точку
                      </button>
                    </div>
                  </div>
                </article>
              ))}
              <CreateEntry tone="flat" onClick={onCreateFlatPoint} />
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
                <article key={h.id} style={S.card}>
                  <div
                    style={{
                      ...S.cardHeadBase,
                      background: 'var(--type-home-bg)',
                      color: 'var(--type-home-fg)',
                    }}
                  >
                    <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>
                      Сосед к себе
                    </span>
                    <span style={{ fontSize: 12, fontWeight: 590, opacity: 0.9 }}>{h.stageBadge}</span>
                  </div>
                  <div style={{ ...S.cardBody, gap: 10 }}>
                    <div style={{ display: 'flex', gap: 10 }}>
                      <PhotoPlaceholder
                        label="фото"
                        style={{ flex: '0 0 72px', height: 54, borderRadius: 8 }}
                      />
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
                        <span style={{ fontSize: 15, fontWeight: 590 }}>{h.title}</span>
                        <span style={S.subtitle}>{h.pricePerPerson}</span>
                        <span style={{ fontSize: 13, fontWeight: 510 }}>
                          {h.responseCount} откликов
                        </span>
                      </div>
                    </div>
                    <button type="button" style={S.secondaryButton} onClick={onOpenIncomingLikes}>
                      Тебя лайкнули · {h.responseCount}
                    </button>
                    <div style={L.actions}>
                      <button type="button" style={L.actionLink} onClick={() => onEditHome?.(h.id)}>
                        Редактировать
                      </button>
                      <button
                        type="button"
                        style={L.destructiveLink}
                        onClick={() => onRemoveHome?.(h.id)}
                      >
                        Закрыть точку
                      </button>
                    </div>
                  </div>
                </article>
              ))}
              {homes.length < homeSlotsTotal && <CreateEntry tone="home" onClick={onCreateHomePoint} />}
            </>
          )}
        </div>
      </div>
    </div>
  );
}

/* Демо-данные для вёрстки блоков 2 и 3: точки приедут с §12 шагом 5.
   Поиски здесь живые — их демо-заготовка удалена. */
export const DEMO_MARKET_FLATS: MarketFlat[] = [
  {
    id: 'f1',
    title: '2-комнатная · Хамовники',
    pricePerPerson: '45 000 ₽ с человека',
    addedAt: 'я добавил · 20 июля',
    poolCount: 6,
    change: 'Цена изменилась: 96 000 → 90 000 ₽',
    /** Скрыта по сигналам пула — создатель может вернуть её одним тапом. */
    hiddenAsRented: true,
  },
];

export const DEMO_MY_HOMES: HomePoint[] = [
  {
    id: 'h1',
    title: 'Комната · м. Сокол',
    pricePerPerson: '28 000 ₽ с человека',
    stage: 'living',
    stageBadge: 'живёт здесь',
    responseCount: 5,
  },
];
