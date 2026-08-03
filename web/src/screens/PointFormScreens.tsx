/* Формы точек — §6. Создание точки не лайк, поэтому кнопки secondary:
   filled в системе зарезервирован под дорогой лайк. */
import { useMemo, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import { S } from '../ui/primitives';

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
};

function Field({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div style={F.field}>
      <span style={F.label}>{label}</span>
      {children}
    </div>
  );
}

const BATHROOMS = [
  { id: '1joint', label: '1 совм.' },
  { id: '1sep', label: '1 разд.' },
  { id: '2', label: '2' },
];

const UTILITIES = [
  { id: 'included', label: 'включена' },
  { id: 'meters', label: 'по счётчикам' },
];

const OWNER_LIMITS = ['не сдают парам', 'без животных', 'без детей'];

/** Для «к себе» формулировки от лица живущего. */
const HOME_LIMITS = ['нельзя с животными', 'хозяин против пар', 'курить нельзя'];

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

function Dropzone({ title, hint }: { title: string; hint: string }) {
  return (
    <div style={F.dropzone}>
      <span style={{ fontSize: 20, color: 'var(--tg-theme-accent-text-color)', lineHeight: 1 }}>＋</span>
      <span style={{ fontSize: 13, fontWeight: 510, color: 'var(--tg-theme-accent-text-color)' }}>
        {title}
      </span>
      <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>{hint}</span>
    </div>
  );
}

const SOURCES: { host: string; label: string }[] = [
  { host: 'avito.ru', label: 'Авито' },
  { host: 'cian.ru', label: 'Циан' },
];

/** Квартира по ссылке: валидация источника + живой калькулятор доли. */
export function FlatLinkFormScreen({
  onSubmit,
  onBack,
}: {
  onSubmit?: () => void;
  onBack?: () => void;
}) {
  const [url, setUrl] = useState('avito.ru/moskva/kvartiry/2-k...');
  const [price, setPrice] = useState(90000);
  const [rooms, setRooms] = useState(2);
  const [bath, setBath] = useState('1sep');
  const [utils, setUtils] = useState('meters');
  const [limits, setLimits] = useState<string[]>(['без животных']);

  const source = useMemo(() => SOURCES.find((s) => url.includes(s.host)), [url]);
  /** Пока собираем только пары — доля всегда половина, не делим на комнаты. */
  const perPerson = Math.round(price / 2);

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
              border: `1px solid ${source ? 'var(--verdict-ok-fg)' : 'var(--sep)'}`,
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

          <Field label="Метро и время пешком">
            <input style={F.input} defaultValue="Фрунзенская · 12 мин" />
          </Field>

          <div style={{ display: 'flex', gap: 8 }}>
            <div style={{ flex: '1 1 0', minWidth: 0 }}>
              <Field label="Цена за квартиру">
                <input
                  style={{ ...F.input, fontWeight: 590 }}
                  value={`${price.toLocaleString('ru-RU')} ₽`}
                  onChange={(e) => setPrice(Number(e.target.value.replace(/\D/g, '')) || 0)}
                />
              </Field>
            </div>
            <div style={{ flex: '0 0 108px' }}>
              <Field label="Комнаты">
                <div style={{ ...F.select, fontWeight: 590 }} onClick={() => setRooms((r) => (r % 4) + 1)}>
                  {rooms}
                  <span style={{ color: 'var(--tg-theme-hint-color)', fontSize: 12 }}>▾</span>
                </div>
              </Field>
            </div>
          </div>

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
                {perPerson.toLocaleString('ru-RU')} ₽
              </span>
            </div>
            <span
              style={{
                fontSize: 12,
                color: 'color-mix(in srgb, var(--verdict-ok-fg) 78%, transparent)',
              }}
            >
              делим на двоих — пока собираем только пары
            </span>
          </div>

          <Field label="Фото-скрин квартиры">
            <Dropzone title="Добавить скрин" hint="чтобы карточку не листали мимо" />
          </Field>

          <Field label="Заезд">
            <div style={F.select}>
              с 15 августа
              <span style={{ color: 'var(--tg-theme-hint-color)', fontSize: 12 }}>▾</span>
            </div>
          </Field>
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
                {BATHROOMS.map((b) => (
                  <SegChip key={b.id} active={bath === b.id} onClick={() => setBath(b.id)}>
                    {b.label}
                  </SegChip>
                ))}
              </div>
            </Field>
            <Field label="Коммуналка">
              <div style={{ display: 'flex', gap: 5 }}>
                {UTILITIES.map((u) => (
                  <SegChip key={u.id} active={utils === u.id} onClick={() => setUtils(u.id)}>
                    {u.label}
                  </SegChip>
                ))}
              </div>
            </Field>
          </div>

          <Field label="Ограничения собственника">
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {OWNER_LIMITS.map((l) => {
                const active = limits.includes(l);
                return (
                  <span
                    key={l}
                    onClick={() =>
                      setLimits((prev) => (active ? prev.filter((x) => x !== l) : [...prev, l]))
                    }
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
                    {l}
                  </span>
                );
              })}
            </div>
            <span style={{ fontSize: 12, lineHeight: 1.4, color: 'var(--tg-theme-hint-color)' }}>
              Покажем на карточке — чтобы не выяснялось после мэтча
            </span>
          </Field>
        </div>
      </div>

      <div style={F.fixed}>
        <button type="button" style={F.submit} onClick={onSubmit}>
          Создать точку
        </button>
        <span style={F.footnote}>Появится в ленте после быстрой проверки — тебе доступна сразу</span>
      </div>
    </div>
  );
}

export type HomeStage = 'living' | 'moving';

/** «К себе»: гейт стадии решает, обязательна ли дата заезда и что за фото просим. */
export function HomePointFormScreen({
  onSubmit,
  onBack,
}: {
  onSubmit?: () => void;
  onBack?: () => void;
}) {
  const [stage, setStage] = useState<HomeStage>('living');
  const [limits, setLimits] = useState<string[]>(['нельзя с животными']);

  const option = (value: HomeStage, title: string, sub: string, badge: string) => {
    const active = stage === value;
    return (
      <div
        onClick={() => setStage(value)}
        style={{
          display: 'flex',
          alignItems: value === 'moving' ? 'flex-start' : 'center',
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
            marginTop: value === 'moving' ? 1 : 0,
          }}
        />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
          <span style={{ fontSize: 15, fontWeight: 590 }}>{title}</span>
          <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>{sub}</span>
          {value === 'moving' && (
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
        {value === 'living' && (
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
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {option('living', 'Уже живу здесь', 'комната освободилась, ищу к себе', 'живёт здесь')}
            {option(
              'moving',
              'Договорился и заезжаю',
              'квартира есть, ищу соседа со старта',
              'заезжает · ищет со-съёмщика со старта',
            )}
          </div>
        </div>

        <div style={F.group}>
          <span style={S.blockTitle}>Квартира и комната</span>

          <Field label="Метро и время пешком">
            <input style={F.input} defaultValue="Сокол · 8 мин" />
          </Field>

          <div style={{ display: 'flex', gap: 8 }}>
            <div style={{ flex: '1 1 0', minWidth: 0 }}>
              <Field label="Цена доли">
                <input style={{ ...F.input, fontWeight: 590 }} defaultValue="28 000 ₽" />
              </Field>
            </div>
            <div style={{ flex: '1 1 0', minWidth: 0 }}>
              <Field label="Депозит">
                <input style={{ ...F.input, fontWeight: 590 }} defaultValue="28 000 ₽" />
              </Field>
            </div>
          </div>

          <Field label="Комната">
            <div style={{ display: 'flex', gap: 6 }}>
              <span
                style={{
                  flex: '1 1 0',
                  padding: 10,
                  borderRadius: 10,
                  background: 'color-mix(in srgb, var(--tg-theme-button-color) 14%, transparent)',
                  color: 'var(--tg-theme-accent-text-color)',
                  fontSize: 14,
                  fontWeight: 590,
                  textAlign: 'center',
                  cursor: 'pointer',
                }}
              >
                изолированная
              </span>
              <span
                style={{
                  flex: '1 1 0',
                  padding: 10,
                  borderRadius: 10,
                  background: 'var(--app-bg)',
                  color: 'var(--tg-theme-subtitle-text-color)',
                  fontSize: 14,
                  textAlign: 'center',
                  cursor: 'pointer',
                }}
              >
                проходная
              </span>
              <input
                style={{ ...F.input, flex: '0 0 84px', width: 84, textAlign: 'center' }}
                defaultValue="14 м²"
              />
            </div>
          </Field>

          <Field
            label={
              <>
                Дата заезда{' '}
                {stage === 'moving' && (
                  <span style={{ color: 'var(--tg-theme-destructive-text-color)' }}>· обязательно</span>
                )}
              </>
            }
          >
            <div style={F.select}>
              с 15 августа
              <span style={{ color: 'var(--tg-theme-hint-color)', fontSize: 12 }}>▾</span>
            </div>
          </Field>

          <Field label="Фото комнаты">
            <Dropzone
              title="Добавить фото"
              hint={
                stage === 'moving' ? 'фото из квартиры, не скрин объявления' : 'как комната выглядит сейчас'
              }
            />
          </Field>

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
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {HOME_LIMITS.map((l) => {
                const active = limits.includes(l);
                return (
                  <span
                    key={l}
                    onClick={() =>
                      setLimits((prev) => (active ? prev.filter((x) => x !== l) : [...prev, l]))
                    }
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
                    {l}
                  </span>
                );
              })}
            </div>
            <span style={{ fontSize: 12, lineHeight: 1.4, color: 'var(--tg-theme-hint-color)' }}>
              Покажем на карточке до лайка — чтобы никто не тратил его зря
            </span>
          </Field>
        </div>
      </div>

      <div style={F.fixed}>
        <button type="button" style={F.submit} onClick={onSubmit}>
          Создать точку
        </button>
        <span style={F.footnote}>Появится в ленте после быстрой проверки — тебе доступна сразу</span>
      </div>
    </div>
  );
}
