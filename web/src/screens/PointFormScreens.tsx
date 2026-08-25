/* Форма «Зову соседа к себе» — §6. Создание точки не лайк, поэтому кнопка
   secondary: filled в системе зарезервирован под дорогой лайк.

   Формы рыночной квартиры здесь больше нет: квартиры выключены этапом 1
   (§2.2), и сервер её тело всё равно отбивает 400. Отказы разные по смыслу и
   по виду: невалидные поля подсвечиваются, лимит говорит словами сервера. */
import { useRef, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';

import {
  ApiError,
  postPoint,
  uploadPointPhoto,
  type CreatePointBody,
  type PointDTO,
  type PointStage,
  type Restriction,
  type Zone,
  type ZoneKind,
} from '../api/client';
import { restrictionOptions } from '../api/format';
import { S } from '../ui/primitives';
import { HomeStageGate } from './EdgeStates';
import { ZonePicker } from './SearchParamsScreen';

/** Точка стоит на станции: зона выдачи собирается из станций (§5). */
const STATION_ONLY: ZoneKind[] = ['station'];

/**
 * Ответ гейта «к себе» (§2.2). Третий — не стадия точки, а тупик: сдающему
 * квартиру мы не продукт, и узнать он это должен до полей, а не после отправки.
 */
type StageChoice = PointStage | 'renting';

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
/**
 * Денежное поле. В фокусе показывает голые цифры: форматированная строка
 * («45 000 ₽») пересобиралась бы на каждый ввод, React ставил бы курсор в
 * конец, а backspace по пробелу и «₽» не удалял бы ничего.
 */
function MoneyInput({
  value,
  invalid,
  placeholder,
  onChange,
}: {
  value: string;
  invalid: boolean;
  placeholder: string;
  onChange: (digitsOnly: string) => void;
}) {
  const [focused, setFocused] = useState(false);
  const shown = focused ? value : value ? `${Number(value).toLocaleString('ru-RU')} ₽` : '';
  return (
    <input
      style={{
        ...F.input,
        fontWeight: 590,
        outline: invalid ? '1.5px solid var(--tg-theme-destructive-text-color)' : 'none',
      }}
      inputMode="numeric"
      placeholder={placeholder}
      value={shown}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      onChange={(e) => onChange(digits(e.target.value))}
    />
  );
}

const MAX_POINT_PHOTOS = 10;

function PhotoPicker({
  title,
  hint,
  invalid,
  onChange,
}: {
  title: string;
  hint: string;
  invalid: boolean;
  onChange: (keys: string[]) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [photos, setPhotos] = useState<{ key: string; preview: string }[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const report = (next: { key: string; preview: string }[]) => {
    setPhotos(next);
    onChange(next.map((p) => p.key));
  };

  const upload = async (files: FileList | null) => {
    if (!files?.length || busy) return;
    setBusy(true);
    setError(null);
    try {
      const room = MAX_POINT_PHOTOS - photos.length;
      const next = [...photos];
      for (const file of Array.from(files).slice(0, room)) {
        const { photo_key } = await uploadPointPhoto(file);
        next.push({ key: photo_key, preview: URL.createObjectURL(file) });
      }
      report(next);
      if (files.length > room) setError(`Больше ${MAX_POINT_PHOTOS} фото не выйдет`);
    } catch (e: unknown) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
      if (input.current) input.current.value = '';
    }
  };

  const remove = (key: string) => {
    const gone = photos.find((p) => p.key === key);
    if (gone) URL.revokeObjectURL(gone.preview);
    report(photos.filter((p) => p.key !== key));
  };

  return (
    <div style={F.field}>
      <span style={F.label}>{title}</span>
      <div className="no-scrollbar" style={{ display: 'flex', gap: 8, overflowX: 'auto' }}>
        {photos.map((p) => (
          <div key={p.key} style={{ position: 'relative', flex: '0 0 auto' }}>
            <img
              src={p.preview}
              alt=""
              style={{ width: 96, height: 72, objectFit: 'cover', borderRadius: 10, display: 'block' }}
            />
            <button
              type="button"
              aria-label="Убрать фото"
              onClick={() => remove(p.key)}
              style={{
                position: 'absolute',
                top: 4,
                right: 4,
                width: 20,
                height: 20,
                border: 'none',
                borderRadius: '50%',
                background: 'rgba(0,0,0,0.55)',
                color: '#fff',
                fontSize: 12,
                lineHeight: 1,
                cursor: 'pointer',
              }}
            >
              ×
            </button>
          </div>
        ))}
        {photos.length < MAX_POINT_PHOTOS && (
          <div
            onClick={() => input.current?.click()}
            style={{
              ...F.dropzone,
              width: photos.length ? 96 : '100%',
              height: photos.length ? 72 : 140,
              aspectRatio: 'auto',
              flex: '0 0 auto',
              borderColor: invalid
                ? 'var(--tg-theme-destructive-text-color)'
                : 'color-mix(in srgb, var(--tg-theme-hint-color) 50%, transparent)',
            }}
          >
            <span style={{ fontSize: 20, color: 'var(--tg-theme-accent-text-color)', lineHeight: 1 }}>
              ＋
            </span>
            {!photos.length && (
              <>
                <span
                  style={{ fontSize: 13, fontWeight: 510, color: 'var(--tg-theme-accent-text-color)' }}
                >
                  {busy ? 'Загружаю…' : 'Добавить фото'}
                </span>
                <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>{hint}</span>
              </>
            )}
          </div>
        )}
      </div>
      <input
        ref={input}
        type="file"
        accept="image/*"
        multiple
        style={{ display: 'none' }}
        onChange={(e) => void upload(e.target.files)}
      />
      {error && <span style={{ ...F.error, textAlign: 'left' }}>{error}</span>}
    </div>
  );
}

/** Отправка формы и отказы сервера. */
function usePointForm(onCreated: (point: PointDTO) => void) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [invalid, setInvalid] = useState<string[]>([]);

  const submit = async (body: CreatePointBody) => {
    if (busy) return;
    setBusy(true);
    setError(null);
    setInvalid([]);
    try {
      onCreated(await postPoint(body));
    } catch (e: unknown) {
      if (e instanceof ApiError && e.code === 'invalid_field') {
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

  return { busy, error, invalid, submit };
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
  const [stage, setStage] = useState<StageChoice>(hasLivePoint ? 'moving_in' : 'live');
  const [zoneId, setZoneId] = useState<string | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [price, setPrice] = useState('');
  const [deposit, setDeposit] = useState('');
  const [isolated, setIsolated] = useState<boolean | null>(null);
  const [moveIn, setMoveIn] = useState('');
  const [restrictions, setRestrictions] = useState<Restriction[]>([]);
  const [photoKeys, setPhotoKeys] = useState<string[]>([]);

  const form = usePointForm(onCreated);

  const zone = zoneId ? ((zones ?? []).find((z) => z.id === zoneId) ?? null) : null;
  const priceShare = Number(price) || 0;
  const bad = (field: string) => form.invalid.includes(field);
  const renting = stage === 'renting';
  const ready =
    !renting &&
    Boolean(zoneId && priceShare > 0 && photoKeys.length && (stage === 'live' || moveIn)) &&
    !form.busy;

  const send = () => {
    // ready несёт в себе !renting, поэтому дальше stage — настоящая стадия.
    if (!ready) return;
    void form.submit({
      type: 'intent_home',
      stage,
      zone_id: zoneId ?? '',
      price_share: priceShare,
      isolated,
      restrictions,
      photo_keys: photoKeys,
      move_in: moveIn || null,
      deposit: deposit ? Number(deposit) : null,
    });
  };

  const option = (value: StageChoice, title: string, sub: string, badge?: string) => {
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
            <HomeStageGate
              moving={stage === 'moving_in'}
              onPickMoving={() => setStage('moving_in')}
            />
          ) : (
            <>
              {option('live', 'Уже живу здесь', 'комната освободилась, ищу к себе', 'живёт здесь')}
              {option(
                'moving_in',
                'Договорился и заезжаю',
                'квартира есть, ищу соседа со старта',
                'заезжает · ищет со-съёмщика со старта',
              )}
            </>
          )}
          {/* Третий ответ гейта §2.2: сдача — не наш кейс, и сказать это надо
              до полей, а не отказом сервера. */}
          {option('renting', 'Сдаю квартиру', 'ищу, кому сдать, а не с кем жить')}
        </div>

        {renting && (
          <div style={F.group}>
            <span style={{ fontSize: 17, fontWeight: 640, letterSpacing: '-0.3px' }}>
              Это не наш кейс
            </span>
            <span
              style={{ fontSize: 14, lineHeight: 1.45, color: 'var(--tg-theme-subtitle-text-color)' }}
            >
              Мы ищем со-съёмщика — того, с кем делят квартиру и счета. Для сдачи есть
              классифайды: Авито и Циан.
            </span>
            <span style={{ fontSize: 13, lineHeight: 1.4, color: 'var(--tg-theme-hint-color)' }}>
              Живёшь в квартире сам или заезжаешь — выбери стадию выше.
            </span>
          </div>
        )}

        {!renting && (
          <div style={F.group}>
            <span style={S.blockTitle}>Квартира и комната</span>

            <Field label="Метро">
              <ZoneField zone={zone} invalid={bad('zone_id')} onOpen={() => setPickerOpen(true)} />
            </Field>

            <div style={{ display: 'flex', gap: 8 }}>
              <div style={{ flex: '1 1 0', minWidth: 0 }}>
                <Field label="Цена доли">
                  <MoneyInput
                    value={price}
                    invalid={bad('price_share')}
                    placeholder="28 000 ₽"
                    onChange={setPrice}
                  />
                </Field>
              </div>
              <div style={{ flex: '1 1 0', minWidth: 0 }}>
                <Field label="Депозит">
                  <MoneyInput
                    value={deposit}
                    invalid={bad('deposit')}
                    placeholder="необязательно"
                    onChange={setDeposit}
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
              invalid={bad('photo_keys')}
              onChange={setPhotoKeys}
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
        )}
      </div>

      {!renting && (
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
            <span style={F.footnote}>
              Появится в ленте после быстрой проверки — тебе доступна сразу
            </span>
          )}
        </div>
      )}

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
