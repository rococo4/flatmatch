/* Формы точек — §6. Создание точки не лайк, поэтому кнопки secondary:
   filled в системе зарезервирован под дорогой лайк.

   Обе формы шлют одно и то же POST /points, тип решает тело. Отказы сервера
   разные по смыслу и по виду: невалидные поля подсвечиваются, лимит и дубль
   говорят словами сервера, и только дубль активной квартиры зовёт войти в её
   пул вместо создания второй такой же (§6). */
import { useRef, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';

import {
  ApiError,
  DuplicatePointError,
  enterPointPool,
  postPoint,
  uploadPointPhoto,
  type Bathrooms,
  type CreatePointBody,
  type PointDTO,
  type PointStage,
  type Restriction,
  type Utilities,
  type Zone,
  type ZoneKind,
} from '../api/client';
import {
  bathroomsOptions,
  money,
  pricePerPerson,
  restrictionOptions,
  utilitiesOptions,
} from '../api/format';
import { S } from '../ui/primitives';
import { HomeStageGate } from './EdgeStates';
import { ZonePicker } from './SearchParamsScreen';

/** Точка стоит на станции: зона выдачи собирается из станций (§5). */
const STATION_ONLY: ZoneKind[] = ['station'];

/**
 * Сервер отвечает на невалидные поля по-английски и списком имён — человеку
 * нужнее подсветка, а строка только зовёт на неё посмотреть.
 */
const INVALID_FIELDS_MESSAGE = 'Проверь выделенные поля';

const F = {
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
    gap: 16,
    padding: '14px 12px 20px',
  } as CSSProperties,
  group: {
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    padding: 12,
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  field: { display: 'flex', flexDirection: 'column', gap: 5 } as CSSProperties,
  label: { fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' } as CSSProperties,
  input: {
    width: '100%',
    padding: '11px 12px',
    borderRadius: 10,
    border: 'none',
    background: 'var(--app-bg)',
    font: 'inherit',
    fontSize: 15,
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  select: {
    padding: '11px 12px',
    borderRadius: 10,
    background: 'var(--app-bg)',
    fontSize: 15,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    cursor: 'pointer',
  } as CSSProperties,
  calc: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 10,
    padding: '11px 12px',
    borderRadius: 10,
    background: 'var(--verdict-ok-bg)',
  } as CSSProperties,
  dropzone: {
    aspectRatio: '16/9',
    borderRadius: 10,
    border: '1px dashed color-mix(in srgb, var(--tg-theme-hint-color) 50%, transparent)',
    background: 'var(--app-bg)',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 4,
    cursor: 'pointer',
    overflow: 'hidden',
  } as CSSProperties,
  fixed: {
    position: 'sticky',
    bottom: 0,
    display: 'flex',
    flexDirection: 'column',
    gap: 7,
    padding: '10px 12px',
    paddingBottom: 'max(12px, env(safe-area-inset-bottom))',
    background: 'var(--tg-theme-bg-color)',
    borderTop: '1px solid var(--sep)',
  } as CSSProperties,
  /** В фикс-зоне поверхность кнопки должна отличаться от полосы в обеих темах:
   *  --app-bg в тёмной равен --tg-theme-bg-color, поэтому здесь --chip-bg. */
  submit: {
    width: '100%',
    padding: 14,
    border: 'none',
    borderRadius: 11,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 16,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  footnote: {
    fontSize: 12,
    lineHeight: 1.4,
    color: 'var(--tg-theme-hint-color)',
    textAlign: 'center',
  } as CSSProperties,
  error: {
    fontSize: 13,
    lineHeight: 1.4,
    color: 'var(--tg-theme-destructive-text-color)',
    textAlign: 'center',
  } as CSSProperties,
  duplicate: {
    display: 'flex',
    flexDirection: 'column',
    gap: 9,
    padding: '11px 12px',
    borderRadius: 11,
    background: 'var(--warning-bg)',
    color: 'var(--warning-fg)',
  } as CSSProperties,
};

const SOURCES: { host: string; label: string }[] = [
  { host: 'avito.ru', label: 'Авито' },
  { host: 'cian.ru', label: 'Циан' },
];

const ROOMS: number[] = [1, 2, 3, 4];

function Field({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div style={F.field}>
      <span style={F.label}>{label}</span>
      {children}
    </div>
  );
}

/** Сегмент-чип для коротких опциональных выборов. */
function SegChip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick?: () => void;
  children: ReactNode;
}) {
  return (
    <span
      onClick={onClick}
      style={{
        flex: '1 1 0',
        padding: '9px 4px',
        borderRadius: 9,
        background: active
          ? 'color-mix(in srgb, var(--tg-theme-button-color) 14%, transparent)'
          : 'var(--app-bg)',
        color: active ? 'var(--tg-theme-accent-text-color)' : 'var(--tg-theme-subtitle-text-color)',
        fontSize: 13,
        fontWeight: active ? 590 : 400,
        textAlign: 'center',
        cursor: 'pointer',
      }}
    >
      {children}
    </span>
  );
}

/** Чипы ограничений: код едет на сервер, метка зависит от типа точки. */
function RestrictionChips({
  options,
  value,
  onToggle,
}: {
  options: { value: Restriction; label: string }[];
  value: Restriction[];
  onToggle: (code: Restriction) => void;
}) {
  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
      {options.map((o) => {
        const active = value.includes(o.value);
        return (
          <span
            key={o.value}
            onClick={() => onToggle(o.value)}
            style={{
              padding: '8px 12px',
              borderRadius: 9,
              background: active
                ? 'color-mix(in srgb, var(--tg-theme-button-color) 14%, transparent)'
                : 'var(--app-bg)',
              color: active
                ? 'var(--tg-theme-accent-text-color)'
                : 'var(--tg-theme-subtitle-text-color)',
              fontSize: 14,
              fontWeight: active ? 590 : 400,
              cursor: 'pointer',
            }}
          >
            {o.label}
          </span>
        );
      })}
    </div>
  );
}

/** Строка выбора станции. Метро у точки одно и обязательно (§5). */
function ZoneField({
  zone,
  invalid,
  onOpen,
}: {
  zone: Zone | null;
  invalid: boolean;
  onOpen: () => void;
}) {
  return (
    <div
      onClick={onOpen}
      style={{
        ...F.select,
        border: invalid ? '1.5px solid var(--tg-theme-destructive-text-color)' : '1.5px solid transparent',
      }}
    >
      {zone ? (
        <span style={{ fontWeight: 590 }}>{zone.name}</span>
      ) : (
        <span style={{ color: 'var(--tg-theme-hint-color)' }}>Выбери станцию метро</span>
      )}
      <span style={{ color: 'var(--tg-theme-hint-color)', fontSize: 12 }}>▾</span>
    </div>
  );
}

/**
 * Фото точки: файл уходит в хранилище сразу, в форме остаётся ключ.
 *
 * Отдельным вызовом, а не вместе с формой: скрин весит мегабайты, и переносить
 * его через каждую неудачную валидацию — терять минуту на каждую опечатку.
 */
function PhotoPicker({
  title,
  hint,
  invalid,
  onUploaded,
}: {
  title: string;
  hint: string;
  invalid: boolean;
  onUploaded: (key: string) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const upload = async (file: File | undefined) => {
    if (!file || busy) return;
    setBusy(true);
    setError(null);
    try {
      const { photo_key } = await uploadPointPhoto(file);
      setPreview((old) => {
        if (old) URL.revokeObjectURL(old);
        return URL.createObjectURL(file);
      });
      onUploaded(photo_key);
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div style={F.field}>
      <span style={F.label}>{title}</span>
      <div
        onClick={() => input.current?.click()}
        style={{
          ...F.dropzone,
          borderColor: invalid
            ? 'var(--tg-theme-destructive-text-color)'
            : 'color-mix(in srgb, var(--tg-theme-hint-color) 50%, transparent)',
        }}
      >
        {preview ? (
          <img
            src={preview}
            alt=""
            style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
          />
        ) : (
          <>
            <span style={{ fontSize: 20, color: 'var(--tg-theme-accent-text-color)', lineHeight: 1 }}>
              ＋
            </span>
            <span
              style={{ fontSize: 13, fontWeight: 510, color: 'var(--tg-theme-accent-text-color)' }}
            >
              {busy ? 'Загружаю…' : 'Добавить фото'}
            </span>
            <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>{hint}</span>
          </>
        )}
      </div>
      <input
        ref={input}
        type="file"
        accept="image/*"
        style={{ display: 'none' }}
        onChange={(e) => void upload(e.target.files?.[0])}
      />
      {error && <span style={{ ...F.error, textAlign: 'left' }}>{error}</span>}
    </div>
  );
}

/**
 * Дубль ссылки (§6): активная квартира зовёт в свой пул, точка на проверке
 * ждёт модератора и входить в неё некуда — она ещё ничья (§5).
 */
function DuplicateBanner({
  duplicate,
  busy,
  onEnter,
}: {
  duplicate: DuplicatePointError;
  busy: boolean;
  onEnter: () => void;
}) {
  return (
    <div style={F.duplicate}>
      <span style={{ fontSize: 13, fontWeight: 590, lineHeight: 1.35 }}>{duplicate.message}</span>
      {duplicate.pointStatus === 'active' && (
        <button
          type="button"
          disabled={busy}
          onClick={onEnter}
          style={{
            alignSelf: 'flex-start',
            padding: '8px 14px',
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
          Войти
        </button>
      )}
    </div>
  );
}

/** Общее для обеих форм: отправка, отказы сервера и вход в пул дубля. */
function usePointForm(onCreated: (point: PointDTO) => void, onEntered?: (pointID: string) => void) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [invalid, setInvalid] = useState<string[]>([]);
  const [duplicate, setDuplicate] = useState<DuplicatePointError | null>(null);

  const submit = async (body: CreatePointBody) => {
    if (busy) return;
    setBusy(true);
    setError(null);
    setInvalid([]);
    setDuplicate(null);
    try {
      onCreated(await postPoint(body));
    } catch (e: unknown) {
      if (e instanceof DuplicatePointError) {
        setDuplicate(e);
      } else if (e instanceof ApiError && e.code === 'invalid_field') {
        setInvalid(e.fields);
        setError(INVALID_FIELDS_MESSAGE);
      } else {
        // Лимит точек сервер уже назвал по-русски и по типу формы.
        setError(errorMessage(e));
      }
    } finally {
      setBusy(false);
    }
  };

  const enter = async (pointID: string) => {
    if (busy) return;
    setBusy(true);
    try {
      await enterPointPool(pointID);
      onEntered?.(pointID);
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return { busy, error, invalid, duplicate, submit, enter };
}

/** Квартира по ссылке: валидация источника + живой калькулятор доли. */
export function FlatLinkFormScreen({
  zones,
  onBack,
  onCreated,
  onEnteredPool,
}: {
  zones: Zone[] | null;
  onBack?: () => void;
  onCreated: (point: PointDTO) => void;
  /** Вошли в чужой пул вместо создания дубля — точки своей не появилось. */
  onEnteredPool: (pointID: string) => void;
}) {
  const [url, setUrl] = useState('');
  const [zoneId, setZoneId] = useState<string | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [price, setPrice] = useState('');
  const [rooms, setRooms] = useState<number | null>(null);
  const [isolated, setIsolated] = useState<boolean | null>(null);
  const [bathrooms, setBathrooms] = useState<Bathrooms | null>(null);
  const [utilities, setUtilities] = useState<Utilities | null>(null);
  const [restrictions, setRestrictions] = useState<Restriction[]>([]);
  const [photoKey, setPhotoKey] = useState('');

  const form = usePointForm(onCreated, onEnteredPool);

  const source = SOURCES.find((s) => url.includes(s.host));
  const zone = zoneId ? ((zones ?? []).find((z) => z.id === zoneId) ?? null) : null;
  const priceTotal = Number(price) || 0;
  const perPerson = pricePerPerson(priceTotal || undefined, rooms ?? undefined);
  const bad = (field: string) => form.invalid.includes(field);
  const ready = Boolean(url.trim() && zoneId && priceTotal > 0 && photoKey) && !form.busy;

  const send = () => {
    if (!ready) return;
    void form.submit({
      type: 'flat',
      url: url.trim(),
      zone_id: zoneId ?? '',
      price_total: priceTotal,
      rooms,
      isolated,
      bathrooms,
      utilities,
      restrictions,
      photo_key: photoKey,
    });
  };

  return (
    <div style={F.screen}>
      <div style={F.header}>
        <span
          style={{ fontSize: 22, lineHeight: 1, color: 'var(--tg-theme-accent-text-color)', cursor: 'pointer' }}
          onClick={onBack}
        >
          ‹
        </span>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>С кем снять</span>
      </div>

      <div style={F.body}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
          <span style={{ fontSize: 22, fontWeight: 640, letterSpacing: '-0.5px', textWrap: 'pretty' }}>
            Найди, с кем снять эту квартиру
          </span>
          <span
            style={{
              fontSize: 15,
              color: 'var(--tg-theme-subtitle-text-color)',
              textWrap: 'pretty',
            }}
          >
            Соберём людей, готовых снять её с тобой
          </span>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
          <span style={S.sectionTitle}>ССЫЛКА НА КВАРТИРУ</span>
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 9,
              padding: 12,
              borderRadius: 11,
              background: 'var(--card-bg)',
              border: `1px solid ${
                bad('url')
                  ? 'var(--tg-theme-destructive-text-color)'
                  : source
                    ? 'var(--verdict-ok-fg)'
                    : 'var(--sep)'
              }`,
            }}
          >
            <input
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="ссылка на Авито или Циан"
              style={{
                flex: '1 1 auto',
                minWidth: 0,
                border: 'none',
                background: 'transparent',
                font: 'inherit',
                fontSize: 15,
                color: 'var(--tg-theme-text-color)',
                outline: 'none',
              }}
            />
            {source && (
              <span
                style={{
                  flex: '0 0 auto',
                  width: 22,
                  height: 22,
                  borderRadius: '50%',
                  background: 'var(--verdict-ok-bg)',
                  color: 'var(--verdict-ok-fg)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  fontSize: 12,
                  fontWeight: 700,
                }}
              >
                ✓
              </span>
            )}
          </div>
          <span
            style={{
              fontSize: 12,
              fontWeight: 510,
              color: source ? 'var(--verdict-ok-fg)' : 'var(--tg-theme-hint-color)',
            }}
          >
            {source ? `${source.label} — ссылка распознана` : 'подойдёт ссылка с Авито или Циан'}
          </span>
        </div>

        <div style={F.group}>
          <span style={S.blockTitle}>Про квартиру</span>

          <Field label="Метро">
            <ZoneField zone={zone} invalid={bad('zone_id')} onOpen={() => setPickerOpen(true)} />
          </Field>

          <div style={{ display: 'flex', gap: 8 }}>
            <div style={{ flex: '1 1 0', minWidth: 0 }}>
              <Field label="Цена за квартиру">
                <input
                  style={{
                    ...F.input,
                    fontWeight: 590,
                    outline: bad('price_total')
                      ? '1.5px solid var(--tg-theme-destructive-text-color)'
                      : 'none',
                  }}
                  inputMode="numeric"
                  placeholder="90 000 ₽"
                  value={price ? `${Number(price).toLocaleString('ru-RU')} ₽` : ''}
                  onChange={(e) => setPrice(digits(e.target.value))}
                />
              </Field>
            </div>
            <div style={{ flex: '0 0 132px' }}>
              <Field label="Комнаты">
                <div style={{ display: 'flex', gap: 5 }}>
                  {ROOMS.map((n) => (
                    <SegChip key={n} active={rooms === n} onClick={() => setRooms(n)}>
                      {n}
                    </SegChip>
                  ))}
                </div>
              </Field>
            </div>
          </div>

          <Field label="Комнаты изолированные?">
            <div style={{ display: 'flex', gap: 5 }}>
              <SegChip active={isolated === true} onClick={() => setIsolated(true)}>
                изолированные
              </SegChip>
              <SegChip active={isolated === false} onClick={() => setIsolated(false)}>
                проходные
              </SegChip>
            </div>
          </Field>

          {perPerson !== null && (
            <div style={{ ...F.calc, flexDirection: 'column', alignItems: 'stretch', gap: 3 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 10 }}>
                <span style={{ fontSize: 14, fontWeight: 510, color: 'var(--verdict-ok-fg)' }}>
                  Выйдет с человека
                </span>
                <span
                  style={{
                    fontSize: 17,
                    fontWeight: 700,
                    letterSpacing: '-0.3px',
                    color: 'var(--verdict-ok-fg)',
                  }}
                >
                  {money(perPerson)}
                </span>
              </div>
              <span
                style={{
                  fontSize: 12,
                  color: 'color-mix(in srgb, var(--verdict-ok-fg) 78%, transparent)',
                }}
              >
                делим на комнаты — по человеку в комнату
              </span>
            </div>
          )}

          <PhotoPicker
            title="Фото-скрин квартиры"
            hint="чтобы карточку не листали мимо"
            invalid={bad('photo_key')}
            onUploaded={setPhotoKey}
          />
        </div>

        {/* Опциональные детали: показываются на карточке, чтобы дил-брейкеры
            выяснялись до мэтча, а не после. */}
        <div style={F.group}>
          <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
            <span style={{ fontSize: 15, fontWeight: 590 }}>Детали</span>
            <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>необязательно</span>
          </div>

          <div style={{ display: 'flex', gap: 8 }}>
            <Field label="Санузлы">
              <div style={{ display: 'flex', gap: 5 }}>
                {bathroomsOptions().map((b) => (
                  <SegChip
                    key={b.value}
                    active={bathrooms === b.value}
                    onClick={() => setBathrooms(b.value)}
                  >
                    {b.label}
                  </SegChip>
                ))}
              </div>
            </Field>
            <Field label="Коммуналка">
              <div style={{ display: 'flex', gap: 5 }}>
                {utilitiesOptions().map((u) => (
                  <SegChip
                    key={u.value}
                    active={utilities === u.value}
                    onClick={() => setUtilities(u.value)}
                  >
                    {u.label}
                  </SegChip>
                ))}
              </div>
            </Field>
          </div>

          <Field label="Ограничения собственника">
            <RestrictionChips
              options={restrictionOptions('flat')}
              value={restrictions}
              onToggle={(code) => setRestrictions(toggle(restrictions, code))}
            />
            <span style={{ fontSize: 12, lineHeight: 1.4, color: 'var(--tg-theme-hint-color)' }}>
              Покажем на карточке — чтобы не выяснялось после мэтча
            </span>
          </Field>
        </div>

        {form.duplicate && (
          <DuplicateBanner
            duplicate={form.duplicate}
            busy={form.busy}
            onEnter={() => void form.enter(form.duplicate?.pointId ?? '')}
          />
        )}
      </div>

      <div style={F.fixed}>
        <button
          type="button"
          style={{ ...F.submit, opacity: ready ? 1 : 0.4 }}
          disabled={!ready}
          onClick={send}
        >
          Создать точку
        </button>
        {form.error ? (
          <span style={F.error}>{form.error}</span>
        ) : (
          <span style={F.footnote}>Появится в ленте после быстрой проверки — тебе доступна сразу</span>
        )}
      </div>

      {pickerOpen && (
        <ZonePicker
          zones={zones}
          kinds={STATION_ONLY}
          title="Метро"
          onPick={(id) => {
            setZoneId(id);
            setPickerOpen(false);
          }}
          onClose={() => setPickerOpen(false)}
        />
      )}
    </div>
  );
}

/** «К себе»: гейт стадии решает, обязательна ли дата заезда и что за фото просим. */
export function HomePointFormScreen({
  zones,
  hasLivePoint,
  onBack,
  onCreated,
}: {
  zones: Zone[] | null;
  /** «Живу здесь» уже есть: факт жизни один, вторую такую точку не завести (§2.2). */
  hasLivePoint: boolean;
  onBack?: () => void;
  onCreated: (point: PointDTO) => void;
}) {
  const [stage, setStage] = useState<PointStage>(hasLivePoint ? 'moving_in' : 'live');
  const [zoneId, setZoneId] = useState<string | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [price, setPrice] = useState('');
  const [deposit, setDeposit] = useState('');
  const [isolated, setIsolated] = useState<boolean | null>(null);
  const [moveIn, setMoveIn] = useState('');
  const [restrictions, setRestrictions] = useState<Restriction[]>([]);
  const [photoKey, setPhotoKey] = useState('');

  const form = usePointForm(onCreated);

  const zone = zoneId ? ((zones ?? []).find((z) => z.id === zoneId) ?? null) : null;
  const priceShare = Number(price) || 0;
  const bad = (field: string) => form.invalid.includes(field);
  const ready =
    Boolean(zoneId && priceShare > 0 && photoKey && (stage === 'live' || moveIn)) && !form.busy;

  const send = () => {
    if (!ready) return;
    void form.submit({
      type: 'intent_home',
      stage,
      zone_id: zoneId ?? '',
      price_share: priceShare,
      isolated,
      restrictions,
      photo_key: photoKey,
      move_in: moveIn || null,
      deposit: deposit ? Number(deposit) : null,
    });
  };

  const option = (value: PointStage, title: string, sub: string, badge: string) => {
    const active = stage === value;
    return (
      <div
        onClick={() => setStage(value)}
        style={{
          display: 'flex',
          alignItems: value === 'moving_in' ? 'flex-start' : 'center',
          gap: 11,
          padding: '13px 12px',
          borderRadius: 12,
          background: 'var(--card-bg)',
          border: active ? '1.5px solid var(--type-home-fg)' : '1px solid var(--sep)',
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
            border: active
              ? '6px solid var(--type-home-fg)'
              : '1.5px solid color-mix(in srgb, var(--tg-theme-hint-color) 70%, transparent)',
            background: active ? 'var(--card-bg)' : 'transparent',
            marginTop: value === 'moving_in' ? 1 : 0,
          }}
        />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
          <span style={{ fontSize: 15, fontWeight: 590 }}>{title}</span>
          <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>{sub}</span>
          {value === 'moving_in' && (
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
              {badge}
            </span>
          )}
        </div>
        {value === 'live' && (
          <span
            style={{
              marginLeft: 'auto',
              flex: '0 0 auto',
              padding: '3px 8px',
              borderRadius: 7,
              background: 'var(--type-home-bg)',
              color: 'var(--type-home-fg)',
              fontSize: 12,
              fontWeight: 590,
            }}
          >
            {badge}
          </span>
        )}
      </div>
    );
  };

  return (
    <div style={F.screen}>
      <div style={F.header}>
        <span
          style={{ fontSize: 22, lineHeight: 1, color: 'var(--tg-theme-accent-text-color)', cursor: 'pointer' }}
          onClick={onBack}
        >
          ‹
        </span>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Сосед к себе</span>
      </div>

      <div style={F.body}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <span style={S.sectionTitle}>НА КАКОЙ ТЫ СТАДИИ</span>
          {hasLivePoint ? (
            <HomeStageGate onPickMoving={() => setStage('moving_in')} />
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              {option('live', 'Уже живу здесь', 'комната освободилась, ищу к себе', 'живёт здесь')}
              {option(
                'moving_in',
                'Договорился и заезжаю',
                'квартира есть, ищу соседа со старта',
                'заезжает · ищет со-съёмщика со старта',
              )}
            </div>
          )}
        </div>

        <div style={F.group}>
          <span style={S.blockTitle}>Квартира и комната</span>

          <Field label="Метро">
            <ZoneField zone={zone} invalid={bad('zone_id')} onOpen={() => setPickerOpen(true)} />
          </Field>

          <div style={{ display: 'flex', gap: 8 }}>
            <div style={{ flex: '1 1 0', minWidth: 0 }}>
              <Field label="Цена доли">
                <input
                  style={{
                    ...F.input,
                    fontWeight: 590,
                    outline: bad('price_share')
                      ? '1.5px solid var(--tg-theme-destructive-text-color)'
                      : 'none',
                  }}
                  inputMode="numeric"
                  placeholder="28 000 ₽"
                  value={price ? `${Number(price).toLocaleString('ru-RU')} ₽` : ''}
                  onChange={(e) => setPrice(digits(e.target.value))}
                />
              </Field>
            </div>
            <div style={{ flex: '1 1 0', minWidth: 0 }}>
              <Field label="Депозит">
                <input
                  style={{
                    ...F.input,
                    fontWeight: 590,
                    outline: bad('deposit')
                      ? '1.5px solid var(--tg-theme-destructive-text-color)'
                      : 'none',
                  }}
                  inputMode="numeric"
                  placeholder="необязательно"
                  value={deposit ? `${Number(deposit).toLocaleString('ru-RU')} ₽` : ''}
                  onChange={(e) => setDeposit(digits(e.target.value))}
                />
              </Field>
            </div>
          </div>

          <Field label="Комната">
            <div style={{ display: 'flex', gap: 5 }}>
              <SegChip active={isolated === true} onClick={() => setIsolated(true)}>
                изолированная
              </SegChip>
              <SegChip active={isolated === false} onClick={() => setIsolated(false)}>
                проходная
              </SegChip>
            </div>
          </Field>

          <Field
            label={
              <>
                Дата заезда{' '}
                {stage === 'moving_in' && (
                  <span style={{ color: 'var(--tg-theme-destructive-text-color)' }}>· обязательно</span>
                )}
              </>
            }
          >
            <input
              type="date"
              value={moveIn}
              onChange={(e) => setMoveIn(e.target.value)}
              style={{
                ...F.input,
                outline: bad('move_in')
                  ? '1.5px solid var(--tg-theme-destructive-text-color)'
                  : 'none',
              }}
            />
          </Field>

          <PhotoPicker
            title="Фото комнаты"
            hint={
              stage === 'moving_in'
                ? 'фото из квартиры, не скрин объявления'
                : 'как комната выглядит сейчас'
            }
            invalid={bad('photo_key')}
            onUploaded={setPhotoKey}
          />

          {/* Дил-брейкеры собственника — видны на карточке до дорогого лайка. */}
          <Field
            label={
              <span
                style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}
              >
                Ограничения квартиры
                <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>необязательно</span>
              </span>
            }
          >
            <RestrictionChips
              options={restrictionOptions('intent_home')}
              value={restrictions}
              onToggle={(code) => setRestrictions(toggle(restrictions, code))}
            />
            <span style={{ fontSize: 12, lineHeight: 1.4, color: 'var(--tg-theme-hint-color)' }}>
              Покажем на карточке до лайка — чтобы никто не тратил его зря
            </span>
          </Field>
        </div>
      </div>

      <div style={F.fixed}>
        <button
          type="button"
          style={{ ...F.submit, opacity: ready ? 1 : 0.4 }}
          disabled={!ready}
          onClick={send}
        >
          Создать точку
        </button>
        {form.error ? (
          <span style={F.error}>{form.error}</span>
        ) : (
          <span style={F.footnote}>Появится в ленте после быстрой проверки — тебе доступна сразу</span>
        )}
      </div>

      {pickerOpen && (
        <ZonePicker
          zones={zones}
          kinds={STATION_ONLY}
          title="Метро"
          onPick={(id) => {
            setZoneId(id);
            setPickerOpen(false);
          }}
          onClose={() => setPickerOpen(false)}
        />
      )}
    </div>
  );
}

function digits(raw: string): string {
  return raw.replace(/\D/g, '').slice(0, 7);
}

function toggle(list: Restriction[], code: Restriction): Restriction[] {
  return list.includes(code) ? list.filter((x) => x !== code) : [...list, code];
}

/** Отказ сервера показываем его же словами: они русские и точные (§6). */
function errorMessage(e: unknown): string {
  return e instanceof ApiError ? e.message : 'Не получилось — попробуй ещё раз';
}
