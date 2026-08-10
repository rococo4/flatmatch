/* Форма параметров поиска (§6, шаг 2) и пикер зон.

   Смотреть — не подписываться: эта форма ничего не сохраняет, поиск создаёт
   только кнопка внутри промо-блока выдачи (§3). Отсюда «Показать» и подпись
   под ней. Режим 'edit' — правка уже сохранённого поиска из «Моих поисков»
   (тот же компонент параметров), там кнопка «Сохранить» и подписи нет.

   Набор фильтров зафиксирован (§2.1) и не расширяется. Чипов «что уже есть»
   здесь нет намеренно: сервер не читает их из фильтров ленты, и поиск
   перестал бы быть снапшотом того, что человек видел. */
import { useEffect, useMemo, useRef, useState } from 'react';
import type { CSSProperties } from 'react';
import {
  backButton,
  hideBackButton,
  onBackButtonClick,
  showBackButton,
} from '@telegram-apps/sdk-react';

import type { SearchParams, SearchResponse, Term, Zone, ZoneKind } from '../api/client';
import { monthOptions } from '../api/format';
import { S } from '../ui/primitives';

/**
 * Черновик формы. Цены — сырые строки инпутов, а не числа: пустое поле это
 * не ноль, а «неважно», и полунабранное «4» не должно становиться 4 ₽.
 */
export type SearchDraft = {
  zoneId: string | null;
  priceMin: string;
  priceMax: string;
  roomsMin: number | null;
  roomsMax: number | null;
  /** «2026-09» */
  moveIn: string | null;
  term: Term | null;
};

export const EMPTY_DRAFT: SearchDraft = {
  zoneId: null,
  priceMin: '',
  priceMax: '',
  roomsMin: null,
  roomsMax: null,
  moveIn: null,
  term: null,
};

// МЦД-платформы отделены от одноимённого метро («Лианозово (МЦД)») — подпись
// «метро» рядом с ними противоречила бы имени.
function zoneKindLabel(z: Zone): string {
  if (z.kind === 'station' && z.name.endsWith('(МЦД)')) return 'станция';
  return ZONE_KIND_LABEL[z.kind];
}

const ZONE_KIND_LABEL: Record<ZoneKind, string> = {
  district: 'район',
  station: 'метро',
  line: 'ветка',
};

/** Порядок групп в пикере: районы шире станций, ветки — самое крупное. */
const PICKER_GROUPS: { kind: ZoneKind; title: string }[] = [
  { kind: 'district', title: 'РАЙОНЫ' },
  { kind: 'station', title: 'СТАНЦИИ МЕТРО' },
  { kind: 'line', title: 'ВЕТКИ' },
];

/** Потолок строк в группе: 309 станций иначе погребают районы и ветки. */
const PICKER_GROUP_LIMIT = 30;

/**
 * Комнатность — пресеты, а не пара «от/до».
 *
 * §6 просит комнатность одним выбором, и пресет физически не может дать
 * невалидную пару вроде «от 3 до 2».
 */
const ROOMS_PRESETS: { label: string; min: number | null; max: number | null }[] = [
  { label: 'неважно', min: null, max: null },
  { label: '1', min: 1, max: 1 },
  { label: '2', min: 2, max: 2 },
  { label: '3', min: 3, max: 3 },
  { label: '4+', min: 4, max: null },
];

const TERM_PRESETS: { label: string; value: Term | null }[] = [
  { label: 'неважно', value: null },
  { label: 'полгода+', value: 'half_year' },
  { label: 'год+', value: 'year' },
  { label: 'бессрочно', value: 'open' },
];

const F = {
  frame: {
    display: 'flex',
    flexDirection: 'column',
    gap: 14,
    minHeight: '100vh',
    width: '100%',
    // Потолок ширины — для десктопа, как во Frame онбординга: приложение
    // рисуется под телефон.
    maxWidth: 520,
    margin: '0 auto',
    padding: '14px 14px 26px',
    background: 'var(--app-bg)',
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  header: { display: 'flex', alignItems: 'center', gap: 6 } as CSSProperties,
  back: {
    fontSize: 22,
    lineHeight: 1,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  headerTitle: { fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' } as CSSProperties,
  card: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: '18px 16px 20px',
    borderRadius: 18,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  field: { display: 'flex', flexDirection: 'column', gap: 8 } as CSSProperties,
  label: { fontSize: 15, fontWeight: 590 } as CSSProperties,
  hint: { fontSize: 12, lineHeight: 1.4, color: 'var(--tg-theme-hint-color)' } as CSSProperties,
  input: {
    width: '100%',
    padding: '11px 12px',
    borderRadius: 10,
    border: 'none',
    background: 'var(--app-bg)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  zoneRow: {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    padding: '11px 12px',
    borderRadius: 11,
    background: 'var(--app-bg)',
    border: '1.5px solid transparent',
    cursor: 'pointer',
  } as CSSProperties,
  chipRow: { display: 'flex', flexWrap: 'wrap', gap: 7 } as CSSProperties,
  error: { fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' } as CSSProperties,
  picker: {
    position: 'fixed',
    inset: 0,
    zIndex: 10,
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
    padding: '14px 14px 20px',
    background: 'var(--app-bg)',
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  pickerList: {
    flex: '1 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    overflowY: 'auto',
    // Список длинный, а полотно узкое: без потолка ширины на десктопе строки
    // растягиваются через весь монитор.
    width: '100%',
    maxWidth: 520,
    margin: '0 auto',
  } as CSSProperties,
  pickerRow: {
    display: 'flex',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    gap: 12,
    padding: '11px 12px',
    borderRadius: 10,
    background: 'var(--card-bg)',
    cursor: 'pointer',
  } as CSSProperties,
  moreButton: {
    width: '100%',
    padding: 11,
    border: 'none',
    borderRadius: 10,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 14,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  centered: {
    flex: '1 1 auto',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    fontSize: 14,
    color: 'var(--tg-theme-hint-color)',
    textAlign: 'center',
  } as CSSProperties,
};

/** Готов ли черновик к отправке: зона и «до» обязательны, вилка не вывернута. */
export function isDraftReady(d: SearchDraft): boolean {
  const max = Number(d.priceMax);
  if (!d.zoneId || !d.priceMax || !Number.isFinite(max) || max <= 0) return false;
  const min = d.priceMin ? Number(d.priceMin) : null;
  return min === null || min <= max;
}

/**
 * Черновик → параметры провода.
 *
 * Пустое поле едет как null — документированная форма «неважно». has_stuff
 * всегда пуст: чипов «что уже есть» в форме нет (см. шапку файла).
 */
export function draftToParams(d: SearchDraft): SearchParams {
  return {
    zone_id: d.zoneId ?? '',
    price_min: d.priceMin ? Number(d.priceMin) : null,
    price_max: Number(d.priceMax),
    rooms_min: d.roomsMin,
    rooms_max: d.roomsMax,
    move_in: d.moveIn,
    term: d.term,
    has_stuff: [],
  };
}

/** Обратное направление: правка сохранённого поиска открывается заполненной. */
export function paramsToDraft(s: SearchResponse): SearchDraft {
  return {
    zoneId: s.zone.id,
    priceMin: s.price_min != null ? String(s.price_min) : '',
    priceMax: String(s.price_max),
    roomsMin: s.rooms_min ?? null,
    roomsMax: s.rooms_max ?? null,
    moveIn: s.move_in ?? null,
    term: s.term ?? null,
  };
}

export function SearchParamsScreen({
  mode,
  zones,
  zoneNameFallback,
  updatesSearch,
  value,
  onChange,
  onSubmit,
  onBack,
  busy,
  error,
  invalidFields,
}: {
  mode: 'look' | 'edit';
  /** null — справочник ещё грузится; пикер об этом честно скажет. */
  zones: Zone[] | null;
  /**
   * Имя зоны из самого поиска — на время, пока справочник грузится.
   *
   * Без него правка сохранённого поиска открывалась бы с плашкой «Выбери
   * район, станцию или ветку» поверх уже выбранной зоны: id известен, а имя
   * живёт только в справочнике.
   */
  zoneNameFallback?: string;
  /** На зону формы есть активный поиск: «Показать» обновит и его (§6). */
  updatesSearch?: boolean;
  value: SearchDraft;
  onChange: (next: SearchDraft) => void;
  onSubmit: () => void;
  onBack: () => void;
  busy?: boolean;
  error?: string | null;
  /** Имена полей из ApiError.fields — что именно не понравилось серверу. */
  invalidFields?: string[];
}) {
  const [pickerOpen, setPickerOpen] = useState(false);

  // Свежий onBack в ref, а не в зависимостях подписки: вызывающий передаёт
  // inline-стрелку, и подписка иначе пересобиралась бы на каждый рендер —
  // то есть на каждую цифру, набранную в бюджете.
  const onBackRef = useRef(onBack);
  useEffect(() => {
    onBackRef.current = onBack;
  }, [onBack]);

  // Родная кнопка «назад» Telegram. При открытом пикере она закрывает его, а
  // не уносит с экрана: иначе тап по системной стрелке терял набранную форму.
  useEffect(() => {
    if (!backButton.isMounted()) return;
    if (!showBackButton.isAvailable()) {
      if (hideBackButton.isAvailable()) hideBackButton();
      return undefined;
    }
    showBackButton();
    return onBackButtonClick(() => {
      if (pickerOpen) setPickerOpen(false);
      else onBackRef.current();
    });
  }, [pickerOpen]);

  // Гасим стрелку при уходе с экрана — отдельным эффектом на монтирование, а
  // не в cleanup подписки выше: та пересобирается на каждое открытие пикера, и
  // стрелка мигала бы. Прятать её больше некому: из всех экранов кнопкой
  // управляют только онбординг и эта форма, поэтому после «Показать» она
  // осталась бы висеть на ленте и табах мёртвой кнопкой.
  useEffect(
    () => () => {
      if (hideBackButton.isAvailable()) hideBackButton();
    },
    [],
  );

  const zone = useMemo(
    () => (value.zoneId ? (zones ?? []).find((z) => z.id === value.zoneId) ?? null : null),
    [zones, value.zoneId],
  );

  const bad = (field: string) => Boolean(invalidFields?.includes(field));
  const min = value.priceMin ? Number(value.priceMin) : null;
  const max = value.priceMax ? Number(value.priceMax) : null;
  const rangeInverted = min !== null && max !== null && min > max;
  const ready = isDraftReady(value) && !busy;

  const digits = (raw: string) => raw.replace(/\D/g, '').slice(0, 7);

  return (
    <div style={F.frame}>
      {/* Своя стрелка вдобавок к родной: та живёт в чужой шапке, и при
          разработке в браузере её не видно. */}
      <div style={F.header}>
        <span style={F.back} onClick={onBack}>
          ‹
        </span>
        <span style={F.headerTitle}>{mode === 'look' ? 'Найти' : 'Параметры поиска'}</span>
      </div>

      <div style={F.card}>
        {/* Зона — единственная жёсткая граница пула (§2.1), поэтому первой.
            В режиме редактирования она заперта: зона — идентичность поиска
            (§3), смена зоны = новый поиск. */}
        <div
          onClick={mode === 'edit' ? undefined : () => setPickerOpen(true)}
          style={{
            ...F.zoneRow,
            cursor: mode === 'edit' ? 'default' : 'pointer',
            borderColor: bad('zone_id') ? 'var(--tg-theme-destructive-text-color)' : 'transparent',
          }}
        >
          <span style={{ flex: '1 1 auto', display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
            <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
              Где ищешь
            </span>
            {zone ? (
              <span style={{ minWidth: 0 }}>
                <span style={{ fontSize: 16, fontWeight: 590 }}>{zone.name}</span>
                <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
                  {` · ${zoneKindLabel(zone)}`}
                </span>
              </span>
            ) : value.zoneId && zoneNameFallback ? (
              // Вид зоны не показываем: в поиске его нет, а угадывать нельзя.
              <span style={{ fontSize: 16, fontWeight: 590, minWidth: 0 }}>{zoneNameFallback}</span>
            ) : (
              <span style={{ fontSize: 15, color: 'var(--tg-theme-hint-color)' }}>
                Выбери район, станцию или ветку
              </span>
            )}
          </span>
          {mode !== 'edit' && (
            <span style={{ flex: '0 0 auto', fontSize: 20, color: 'var(--tg-theme-hint-color)' }}>›</span>
          )}
        </div>
        {mode === 'edit' && (
          <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)', padding: '0 2px' }}>
            Зона не меняется — другой район или ветка это новый поиск
          </span>
        )}

        {/* Бюджет — «с человека»: цена доли, не аренда целиком. */}
        <div style={F.field}>
          <span style={F.label}>Бюджет с человека</span>
          <div style={{ display: 'flex', gap: 8 }}>
            <div style={{ flex: '1 1 0', display: 'flex', flexDirection: 'column', gap: 5, minWidth: 0 }}>
              <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>от</span>
              <input
                style={{
                  ...F.input,
                  outline: bad('price_min') ? '1.5px solid var(--tg-theme-destructive-text-color)' : 'none',
                }}
                inputMode="numeric"
                placeholder="неважно"
                value={value.priceMin}
                onChange={(e) => onChange({ ...value, priceMin: digits(e.target.value) })}
              />
            </div>
            <div style={{ flex: '1 1 0', display: 'flex', flexDirection: 'column', gap: 5, minWidth: 0 }}>
              <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>до</span>
              <input
                style={{
                  ...F.input,
                  outline: bad('price_max') ? '1.5px solid var(--tg-theme-destructive-text-color)' : 'none',
                }}
                inputMode="numeric"
                placeholder="45 000"
                value={value.priceMax}
                onChange={(e) => onChange({ ...value, priceMax: digits(e.target.value) })}
              />
            </div>
          </div>
          {rangeInverted ? (
            <span style={F.error}>«от» больше «до»</span>
          ) : (
            <span style={F.hint}>Цена доли, а не за квартиру целиком</span>
          )}
        </div>

        <div style={F.field}>
          <span style={F.label}>Комнатность</span>
          <div style={F.chipRow}>
            {ROOMS_PRESETS.map((p) => (
              <Chip
                key={p.label}
                label={p.label}
                active={value.roomsMin === p.min && value.roomsMax === p.max}
                onClick={() => onChange({ ...value, roomsMin: p.min, roomsMax: p.max })}
              />
            ))}
          </div>
        </div>

        <div style={F.field}>
          <span style={F.label}>Заезд</span>
          <div style={F.chipRow}>
            <Chip
              label="неважно"
              active={value.moveIn === null}
              onClick={() => onChange({ ...value, moveIn: null })}
            />
            {monthOptions(6).map((m) => (
              <Chip
                key={m.value}
                label={m.label}
                active={value.moveIn === m.value}
                onClick={() => onChange({ ...value, moveIn: m.value })}
              />
            ))}
          </div>
        </div>

        <div style={F.field}>
          <span style={F.label}>Срок</span>
          <div style={F.chipRow}>
            {TERM_PRESETS.map((t) => (
              <Chip
                key={t.label}
                label={t.label}
                active={value.term === t.value}
                onClick={() => onChange({ ...value, term: t.value })}
              />
            ))}
          </div>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
          <button
            type="button"
            style={{ ...S.filledButton, opacity: ready ? 1 : 0.4 }}
            disabled={!ready}
            onClick={onSubmit}
          >
            {mode === 'look' ? 'Показать' : 'Сохранить'}
          </button>
          {/* Только в 'look': в 'edit' это как раз подписка. */}
          {mode === 'look' && (
            <span style={S.caption}>
              {updatesSearch
                ? 'Обновит твой сохранённый поиск в этой зоне'
                : 'Это ещё не подписка — просто посмотрим, что есть'}
            </span>
          )}
          {error && <span style={F.error}>{error}</span>}
        </div>
      </div>

      {pickerOpen && (
        <ZonePicker
          zones={zones}
          onPick={(id) => {
            onChange({ ...value, zoneId: id });
            setPickerOpen(false);
          }}
          onClose={() => setPickerOpen(false)}
        />
      )}
    </div>
  );
}

/** Чип одиночного выбора. Идиома та же, что у чипов животных в онбординге. */
function Chip({ label, active, onClick }: { label: string; active: boolean; onClick: () => void }) {
  return (
    <span
      onClick={onClick}
      style={{
        padding: '10px 14px',
        borderRadius: 10,
        background: active
          ? 'color-mix(in srgb, var(--tg-theme-button-color) 12%, transparent)'
          : 'var(--app-bg)',
        border: active ? '1.5px solid var(--tg-theme-button-color)' : '1.5px solid transparent',
        fontSize: 15,
        fontWeight: active ? 590 : 400,
        color: active ? 'var(--tg-theme-accent-text-color)' : 'var(--tg-theme-text-color)',
        cursor: 'pointer',
      }}
    >
      {label}
    </span>
  );
}

/**
 * Пикер зон — полноэкранный слой поверх формы.
 *
 * Своего экрана у него нет намеренно: выбор зоны это шаг формы, а не место,
 * куда можно прийти. Справочник конечен (~450 зон) и ищется целиком на
 * клиенте — сетевого поиска здесь нет.
 */
function ZonePicker({
  zones,
  onPick,
  onClose,
}: {
  zones: Zone[] | null;
  onPick: (id: string) => void;
  onClose: () => void;
}) {
  const [query, setQuery] = useState('');

  // Индекс строится один раз на справочник, а не на каждое нажатие клавиши:
  // 454 нормализации на символ ввода — заметная работа в WebView телефона.
  const index = useMemo(
    () => (zones ?? []).map((zone) => ({ zone, key: normalize(zone.name) })),
    [zones],
  );

  const q = normalize(query.trim());
  const matched = useMemo(() => {
    if (!q) return index;
    const hits = index.filter((e) => e.key.includes(q));
    // Совпадение в начале строки выше остальных; внутри групп сортировка
    // стабильна, то есть сохраняет порядок справочника.
    return hits.sort((a, b) => Number(b.key.startsWith(q)) - Number(a.key.startsWith(q)));
  }, [index, q]);

  return (
    <div style={F.picker}>
      <div style={F.header}>
        <span style={F.back} onClick={onClose}>
          ‹
        </span>
        <span style={F.headerTitle}>Где ищешь</span>
      </div>

      <input
        style={{ ...F.input, maxWidth: 520, margin: '0 auto' }}
        autoFocus
        placeholder="Район, станция или ветка"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
      />

      {zones === null ? (
        <span style={F.centered}>Загружаю справочник…</span>
      ) : matched.length === 0 ? (
        <span style={F.centered}>Ничего не нашлось — попробуй короче</span>
      ) : (
        <div style={F.pickerList} className="no-scrollbar">
          {PICKER_GROUPS.map(({ kind, title }) => {
            const group = matched.filter((e) => e.zone.kind === kind);
            if (group.length === 0) return null;
            return (
              // key с запросом: новый запрос должен снова показывать первые 30,
              // а не остаток раскрытой прошлой выдачи.
              <PickerGroup
                key={`${kind}:${q}`}
                title={`${title} · ${group.length}`}
                zones={group.map((e) => e.zone)}
                onPick={onPick}
              />
            );
          })}
        </div>
      )}
    </div>
  );
}

function PickerGroup({
  title,
  zones,
  onPick,
}: {
  title: string;
  zones: Zone[];
  onPick: (id: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const shown = expanded ? zones : zones.slice(0, PICKER_GROUP_LIMIT);
  const rest = zones.length - shown.length;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <span style={S.sectionTitle}>{title}</span>
      {shown.map((z) => (
        <div key={z.id} style={F.pickerRow} onClick={() => onPick(z.id)}>
          <span style={{ fontSize: 15, fontWeight: 510, minWidth: 0 }}>{z.name}</span>
          <span style={{ flex: '0 0 auto', fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
            {zoneKindLabel(z)}
          </span>
        </div>
      ))}
      {rest > 0 && (
        <button type="button" style={F.moreButton} onClick={() => setExpanded(true)}>
          ещё {rest}
        </button>
      )}
    </div>
  );
}

/**
 * Приведение к сравнимому виду.
 *
 * «ё» складывается с «е» с обеих сторон: без этого «жел» не найдёт «жёлтую
 * ветку», а «теплый» — «Тёплый Стан».
 */
function normalize(s: string): string {
  return s.toLowerCase().replace(/ё/g, 'е');
}
