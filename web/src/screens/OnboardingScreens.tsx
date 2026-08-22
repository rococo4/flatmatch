/* Онбординг — §4. 12 шагов, ~2 минуты. Анкета выверена по ресерчу предикторов
   конфликтов сожителей: чистота, сон, шум, гости/партнёр, деньги — сильные;
   хобби, психотипы, температура — не предсказывают и в анкету не входят.
   В анкете нет бюджета/района/дат — это параметры поиска, не личности.
   У каждого вопроса подпись «зачем спрашиваем». */

import { useEffect, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';

export type Choice = { id: string; label: string };

export const TOTAL_STEPS = 12;

const O = {
  screen: {
    display: 'flex',
    flexDirection: 'column',
    minHeight: '100%',
    background: 'var(--app-bg)',
    color: 'var(--tg-theme-text-color)',
    padding: '14px 12px 20px',
    gap: 14,
  } as CSSProperties,
  card: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: '18px 16px 20px',
    borderRadius: 18,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  stepLine: { fontSize: 12, fontWeight: 590, color: 'var(--tg-theme-hint-color)' } as CSSProperties,
  track: {
    height: 3,
    borderRadius: 2,
    background: 'var(--chip-bg)',
    overflow: 'hidden',
  } as CSSProperties,
  h1: { fontSize: 23, fontWeight: 640, letterSpacing: '-0.5px' } as CSSProperties,
  why: {
    fontSize: 14,
    color: 'var(--tg-theme-subtitle-text-color)',
    textWrap: 'pretty',
  } as CSSProperties,
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
  next: {
    width: '100%',
    padding: 13,
    border: 'none',
    borderRadius: 11,
    background: 'var(--tg-theme-button-color)',
    font: 'inherit',
    fontSize: 16,
    fontWeight: 590,
    color: 'var(--tg-theme-button-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
};

export function StepHeader({ step, total, aside }: { step: number; total: number; aside?: string }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 8 }}>
        <span style={O.stepLine}>
          Шаг {step} из {total}
        </span>
        {aside && <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>{aside}</span>}
      </div>
      <div style={O.track}>
        <div
          style={{
            width: `${Math.round((step / total) * 100)}%`,
            height: '100%',
            borderRadius: 2,
            background: 'var(--tg-theme-button-color)',
          }}
        />
      </div>
    </div>
  );
}

/** Радио-строка. Используется для «с кем готов жить» и гейтов. */
function RadioRow({
  label,
  active,
  onClick,
  children,
}: {
  label: string;
  active: boolean;
  onClick?: () => void;
  children?: ReactNode;
}) {
  return (
    <div
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 11,
        padding: 12,
        borderRadius: 11,
        background: 'var(--card-bg)',
        border: active ? '1.5px solid var(--tg-theme-button-color)' : '1px solid var(--sep)',
        cursor: 'pointer',
      }}
    >
      <span
        style={{
          flex: '0 0 auto',
          width: 19,
          height: 19,
          borderRadius: '50%',
          boxSizing: 'border-box',
          border: active
            ? '6px solid var(--tg-theme-button-color)'
            : '1.5px solid color-mix(in srgb, var(--tg-theme-hint-color) 70%, transparent)',
          background: active ? 'var(--card-bg)' : 'transparent',
        }}
      />
      <span style={{ fontSize: 15, fontWeight: 510 }}>{label}</span>
      {children}
    </div>
  );
}

/** Шаг 0: возраст, пол, предпочтение пола соседа (единственное обоюдное правило видимости). */
export type Basics = {
  age: string;
  gender: 'f' | 'm' | null;
  pref: 'any' | 'same' | null;
};

export function BasicsStep({
  value,
  onChange,
  onNext,
  canGoNext,
}: {
  value: Basics;
  onChange: (next: Basics) => void;
  onNext?: () => void;
  canGoNext: boolean;
}) {
  const { gender, pref } = value;
  const setGender = (g: 'f' | 'm') => onChange({ ...value, gender: g });
  const setPref = (p: 'any' | 'same') => onChange({ ...value, pref: p });

  const genderChip = (v: 'f' | 'm', label: string) => (
    <span
      onClick={() => setGender(v)}
      style={{
        flex: '1 1 0',
        padding: '11px 6px',
        borderRadius: 10,
        background:
          gender === v
            ? 'color-mix(in srgb, var(--tg-theme-button-color) 14%, transparent)'
            : 'var(--app-bg)',
        color: gender === v ? 'var(--tg-theme-accent-text-color)' : 'var(--tg-theme-subtitle-text-color)',
        fontSize: 14,
        fontWeight: gender === v ? 590 : 400,
        textAlign: 'center',
        cursor: 'pointer',
      }}
    >
      {label}
    </span>
  );

  return (
    <div style={O.card}>
      <StepHeader step={1} total={TOTAL_STEPS} aside="≈2 минуты" />
      <span style={O.h1}>Базовое</span>

      <div style={{ display: 'flex', gap: 8 }}>
        <div style={{ flex: '1 1 0', display: 'flex', flexDirection: 'column', gap: 5, minWidth: 0 }}>
          <span style={O.why}>Возраст</span>
          <input
            style={O.input}
            inputMode="numeric"
            placeholder="23"
            value={value.age}
            onChange={(e) => onChange({ ...value, age: e.target.value.replace(/\D/g, '').slice(0, 3) })}
          />
        </div>
        <div style={{ flex: '1 1 0', display: 'flex', flexDirection: 'column', gap: 5, minWidth: 0 }}>
          <span style={O.why}>Пол</span>
          <div style={{ display: 'flex', gap: 6 }}>
            {genderChip('f', 'Ж')}
            {genderChip('m', 'М')}
          </div>
        </div>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
        <span style={{ fontSize: 15, fontWeight: 590 }}>С кем готов жить?</span>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
          {GENDER_PREF_CHOICES.map((c) => (
            <RadioRow
              key={c.id}
              label={c.label}
              active={pref === c.id}
              onClick={() => setPref(c.id as 'any' | 'same')}
            />
          ))}
        </div>
        <span style={O.hint}>{GENDER_PREF_HINT}</span>
      </div>

      <button
        type="button"
        style={{ ...O.next, opacity: canGoNext ? 1 : 0.4 }}
        disabled={!canGoNext}
        onClick={onNext}
      >
        Дальше
      </button>
    </div>
  );
}

/** Вопрос анкеты: тап выбирает, «Дальше» уводит.
 *
 *  Разделение намеренное. Пока выбор сам листал экран, ошибиться было нельзя —
 *  случайное касание уже улетало на сервер и уносило вперёд. Теперь ответ можно
 *  переменить, а запрос уходит один на экран, вместе с номером шага.
 */
export function ChoiceStep({
  step,
  title,
  why,
  choices,
  value,
  busy,
  onNext,
}: {
  step: number;
  title: string;
  why: string;
  choices: Choice[];
  /** Ответ с сервера — чтобы вернувшийся увидел свой выбор. null — ещё не отвечал. */
  value: string | null;
  busy?: boolean;
  onNext: (id: string) => void;
}) {
  const [selected, setSelected] = useState<string | null>(value);

  // Экран переиспользуется под все восемь вопросов, поэтому при смене шага
  // выбор надо перечитать заново — иначе на следующий вопрос переедет ответ
  // с предыдущего.
  useEffect(() => setSelected(value), [step, value]);

  const picked = selected;
  const pick = (id: string) => setSelected(id);

  return (
    <div style={O.card}>
      <StepHeader step={step} total={TOTAL_STEPS} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
        <span style={O.h1}>{title}</span>
        <span style={O.why}>{why}</span>
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
        {choices.map((c) => {
          const active = picked === c.id;
          return (
            <div
              key={c.id}
              onClick={() => pick(c.id)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 10,
                padding: '17px 14px',
                borderRadius: 12,
                background: active
                  ? 'color-mix(in srgb, var(--tg-theme-button-color) 12%, transparent)'
                  : 'var(--app-bg)',
                border: active ? '1.5px solid var(--tg-theme-button-color)' : '1.5px solid transparent',
                fontSize: 16,
                fontWeight: active ? 590 : 400,
                color: active ? 'var(--tg-theme-accent-text-color)' : 'var(--tg-theme-text-color)',
                cursor: 'pointer',
                transition: 'background 140ms ease, border-color 140ms ease, transform 140ms ease',
                transform: active ? 'scale(1.015)' : 'none',
              }}
            >
              {c.label}
              <span
                style={{
                  marginLeft: 'auto',
                  fontSize: 15,
                  fontWeight: 700,
                  opacity: active ? 1 : 0,
                  transition: 'opacity 140ms ease',
                }}
              >
                ✓
              </span>
            </div>
          );
        })}
      </div>

      <button
        type="button"
        style={{ ...O.next, opacity: picked && !busy ? 1 : 0.4 }}
        disabled={!picked || busy}
        onClick={() => picked && onNext(picked)}
      >
        Дальше
      </button>
    </div>
  );
}

/** Шаг 5: животные — два подвопроса на одном экране. */
export function PetsStep({
  own,
  others,
  onChange,
  onNext,
  canGoNext,
}: {
  own: string | null;
  others: string | null;
  onChange: (next: { own: string | null; others: string | null }) => void;
  onNext?: () => void;
  canGoNext: boolean;
}) {
  const setOwn = (v: string) => onChange({ own: v, others });
  const setOthers = (v: string) => onChange({ own, others: v });

  const chip = (active: boolean, label: string, onClick: () => void) => (
    <span
      key={label}
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

  return (
    <div style={O.card}>
      <StepHeader step={5} total={TOTAL_STEPS} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
        <span style={O.h1}>Животные</span>
        <span style={O.why}>{PETS_WHY}</span>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <span style={{ fontSize: 15, fontWeight: 590 }}>Свои животные?</span>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 7 }}>
          {OWN_PETS.map((c) => chip(own === c.id, c.label, () => setOwn(c.id)))}
        </div>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <span style={{ fontSize: 15, fontWeight: 590 }}>Как к чужим?</span>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 7 }}>
          {OTHER_PETS.map((c) => chip(others === c.id, c.label, () => setOthers(c.id)))}
        </div>
      </div>

      <button
        type="button"
        style={{ ...O.next, opacity: canGoNext ? 1 : 0.4 }}
        disabled={!canGoNext}
        onClick={onNext}
      >
        Дальше
      </button>
    </div>
  );
}

/** Шаг 11: «О себе» — единственный свободный текст во всём продукте, ≤200 знаков. */
export function AboutStep({
  text,
  onChange,
  max = 200,
}: {
  text: string;
  onChange: (next: string) => void;
  max?: number;
}) {
  const setText = onChange;
  return (
    <div style={O.card}>
      <StepHeader step={11} total={TOTAL_STEPS} />
      <span style={O.h1}>О себе</span>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <textarea
          value={text}
          maxLength={max}
          onChange={(e) => setText(e.target.value)}
          placeholder="Диета, религия, привычки — если важно"

          style={{
            minHeight: 104,
            padding: 12,
            borderRadius: 11,
            border: 'none',
            background: 'var(--app-bg)',
            font: 'inherit',
            fontSize: 15,
            lineHeight: 1.45,
            color: 'var(--tg-theme-text-color)',
            resize: 'none',
            outline: 'none',
          }}
        />
        <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10 }}>
          <span style={{ ...O.hint, flex: '1 1 auto', textWrap: 'pretty', minWidth: 0 }}>
            Диета, религия, температура в комнате — если важно, напиши здесь. Пройдёт проверку вместе
            с фото
          </span>
          <span
            style={{
              flex: '0 0 auto',
              fontSize: 12,
              color: 'var(--tg-theme-hint-color)',
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            {text.length}/{max}
          </span>
        </div>
      </div>
    </div>
  );
}

/** Шаг 12: фото с лицом. Обязательное, проверяется глазами модератора (§4, §5).
 *  Аватар из Telegram не подставляем: он может быть скрыт настройками
 *  приватности, и путь «оставить как есть» работал бы не у всех. */
export function PhotoStep({
  previewUrl,
  uploading,
  error,
  onPick,
}: {
  /** Уже загруженное фото — своё человек видит всегда, даже до проверки. */
  previewUrl: string | null;
  uploading: boolean;
  error: string | null;
  onPick: (file: File) => void;
}) {
  return (
    <div style={O.card}>
      <StepHeader step={12} total={TOTAL_STEPS} />
      <span style={O.h1}>Фото с лицом</span>

      <label
        style={{
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 10,
          minHeight: 190,
          padding: 16,
          borderRadius: 14,
          border: previewUrl
            ? '1px solid var(--sep)'
            : '1.5px dashed color-mix(in srgb, var(--tg-theme-button-color) 55%, transparent)',
          background: 'var(--app-bg)',
          cursor: uploading ? 'progress' : 'pointer',
          overflow: 'hidden',
        }}
      >
        <input
          type="file"
          accept="image/jpeg,image/png,image/webp"
          disabled={uploading}
          style={{ display: 'none' }}
          onChange={(e) => {
            const file = e.target.files?.[0];
            // Сбрасываем значение: иначе повторный выбор того же файла после
            // отклонённой загрузки не даст события вовсе.
            e.target.value = '';
            if (file) onPick(file);
          }}
        />
        {previewUrl ? (
          <img
            src={previewUrl}
            alt="Твоё фото"
            style={{ width: 150, height: 150, borderRadius: '50%', objectFit: 'cover' }}
          />
        ) : (
          <span style={{ fontSize: 38, lineHeight: 1, color: 'var(--tg-theme-hint-color)' }}>+</span>
        )}
        <span
          style={{
            fontSize: 15,
            fontWeight: 590,
            color: 'var(--tg-theme-accent-text-color)',
            textAlign: 'center',
          }}
        >
          {uploading ? 'Загружаю…' : previewUrl ? 'Выбрать другое' : 'Загрузить фото'}
        </span>
      </label>

      {error && (
        <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>{error}</span>
      )}

      <span style={O.hint}>
        Лицо должно быть видно — по фото решают, писать ли тебе. Проверим вместе с «о себе»
      </span>
    </div>
  );
}

/** Финал: в pending можно смотреть ленту и сохранять поиски, нельзя быть видимым.
 *  Развилка — две карточки-пути в цветах своих типов точек. */
export function OnboardingDone({
  onSetupSearch,
  onJustLook,
  onHostFlow,
}: {
  onSetupSearch?: () => void;
  onJustLook?: () => void;
  /** → форма «к себе», сразу на гейте стадии */
  onHostFlow?: () => void;
}) {
  return (
    <div style={{ ...O.card, padding: '18px 12px 16px', gap: 16 }}>
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 10 }}>
        <span
          style={{
            width: 52,
            height: 52,
            borderRadius: '50%',
            background: 'var(--verdict-ok-bg)',
            color: 'var(--verdict-ok-fg)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            fontSize: 24,
            fontWeight: 700,
          }}
        >
          ✓
        </span>
        <span style={{ fontSize: 22, fontWeight: 640, letterSpacing: '-0.5px' }}>Анкета готова</span>
      </div>

      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          gap: 3,
          padding: '10px 12px',
          borderRadius: 11,
          background: 'var(--warning-bg)',
          color: 'var(--warning-fg)',
        }}
      >
        <span style={{ fontSize: 13, fontWeight: 590, lineHeight: 1.35 }}>
          Уйдёт на быструю проверку — виден станешь после
        </span>
        <span
          style={{
            fontSize: 12,
            lineHeight: 1.35,
            color: 'color-mix(in srgb, var(--warning-fg) 72%, transparent)',
          }}
        >
          пока можешь смотреть и сохранять поиски, лайки сохранятся
        </span>
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        <div
          onClick={onSetupSearch}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            padding: '18px 14px',
            borderRadius: 14,
            background: 'var(--type-search-bg)',
            color: 'var(--type-search-fg)',
            cursor: 'pointer',
          }}
        >
          <div style={{ flex: '1 1 auto', display: 'flex', flexDirection: 'column', gap: 5, minWidth: 0 }}>
            <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px' }}>Ищу, куда съехать</span>
            <span style={{ fontSize: 14, lineHeight: 1.35, opacity: 0.85, textWrap: 'pretty' }}>
              Настрой поиск — 20 секунд, покажем людей и квартиры
            </span>
          </div>
          <span style={{ flex: '0 0 auto', fontSize: 22, lineHeight: 1, opacity: 0.6 }}>›</span>
        </div>

        <div
          onClick={onHostFlow}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            padding: 14,
            borderRadius: 14,
            background: 'var(--type-home-bg)',
            color: 'var(--type-home-fg)',
            cursor: 'pointer',
          }}
        >
          <div style={{ flex: '1 1 auto', display: 'flex', flexDirection: 'column', gap: 4, minWidth: 0 }}>
            <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.3px' }}>Зову соседа к себе</span>
            <span style={{ fontSize: 13, lineHeight: 1.35, opacity: 0.85, textWrap: 'pretty' }}>
              Уже живёшь в квартире или заезжаешь — найди, кто въедет
            </span>
          </div>
          <span style={{ flex: '0 0 auto', fontSize: 22, lineHeight: 1, opacity: 0.6 }}>›</span>
        </div>
      </div>

      <span
        onClick={onJustLook}
        style={{
          alignSelf: 'center',
          fontSize: 14,
          fontWeight: 510,
          color: 'var(--tg-theme-accent-text-color)',
          cursor: 'pointer',
        }}
      >
        просто посмотреть ленту
      </span>
    </div>
  );
}

export const OWN_PETS: Choice[] = [
  { id: 'none', label: 'Нет' },
  { id: 'cat', label: 'Кот' },
  { id: 'dog', label: 'Собака' },
  { id: 'other', label: 'Другое' },
];

export const OTHER_PETS: Choice[] = [
  { id: 'ok', label: 'Ок' },
  { id: 'allergy', label: 'Аллергия' },
  { id: 'against', label: 'Против' },
];

/** Зачем спрашиваем про животных — подпись экрана: подвопроса два, причина одна. */
export const PETS_WHY = 'Аллергия выясняется на переезде — лучше сейчас';

/* Шаг 0 отдельными константами: тем же вопросам нужен второй дом — список
   правки анкеты (§6). Лейблы полные, «Ж»/«М» на чипах формы — сокращение
   ради двух колонок, словами их читают в шторке. Значения — как в API. */
export const GENDER_CHOICES: Choice[] = [
  { id: 'female', label: 'Женский' },
  { id: 'male', label: 'Мужской' },
];

export const GENDER_PREF_CHOICES: Choice[] = [
  { id: 'any', label: 'С любым полом' },
  { id: 'same', label: 'Только свой пол' },
];

export const GENDER_PREF_HINT =
  'Это взаимно: если ты «только свой пол», другой пол тебя не увидит нигде';

/** Вопросы-тапы. Шаг 1 — BasicsStep, 5 — PetsStep, 11 — AboutStep, 12 — PhotoStep. */
/** Поле анкеты, в которое пишет экран-вопрос. Совпадает с ключом в API. */
export type QuestionField =
  | 'occupation'
  | 'schedule'
  | 'smoking'
  | 'noise'
  | 'guests'
  | 'cleaning'
  | 'alcohol'
  | 'expenses';

export const QUESTION_STEPS: {
  step: number;
  field: QuestionField;
  title: string;
  why: string;
  choices: Choice[];
}[] = [
  {
    step: 2,
    field: 'occupation',
    title: 'Чем занимаешься?',
    why: 'График занятости — это про то, когда квартира пустая',
    choices: [
      { id: 'study', label: 'Учусь' },
      { id: 'office', label: 'Офис' },
      { id: 'hybrid', label: 'Гибрид' },
      { id: 'remote', label: 'Удалёнка' },
      { id: 'shift', label: 'Сменный график' },
    ],
  },
  {
    step: 3,
    field: 'schedule',
    title: 'Когда ты активен?',
    why: 'Разный график — конфликт номер один у соседей',
    choices: [
      { id: 'owl', label: 'Сова — поздно ложусь' },
      { id: 'lark', label: 'Жаворонок — рано встаю' },
      { id: 'floating', label: 'Плавает' },
    ],
  },
  {
    step: 4,
    field: 'smoking',
    title: 'Куришь? (вейп и айкос тоже считаются)',
    why: 'Дым в общей квартире — то, о чём договариваются на берегу',
    choices: [
      { id: 'no', label: 'Не курю' },
      { id: 'outside', label: 'На балконе или улице' },
      { id: 'home', label: 'Дома' },
    ],
  },
  {
    step: 6,
    field: 'noise',
    title: 'Какой звук дома норм?',
    why: 'Наушники или колонки — вот про это',
    choices: [
      { id: 'silence', label: 'Нужна тишина' },
      { id: 'normal', label: 'Нормально' },
      { id: 'loud', label: 'Сам шумный' },
    ],
  },
  {
    step: 7,
    field: 'guests',
    title: 'Как часто у тебя гости?',
    why: 'Партнёр, который остаётся ночевать, — отдельный вариант',
    choices: [
      { id: 'rare', label: 'Редко' },
      { id: 'sometimes', label: 'Иногда' },
      { id: 'often', label: 'Часто' },
      { id: 'partner', label: 'Партнёр регулярно ночует' },
    ],
  },
  {
    step: 8,
    field: 'cleaning',
    title: 'Как у тебя с уборкой?',
    why: 'Грязная посуда — причина ссор номер один в общих квартирах',
    choices: [
      { id: 'schedule', label: 'По расписанию' },
      { id: 'when_dirty', label: 'Убираю, когда грязно' },
      { id: 'creative_mess', label: 'Творческий беспорядок' },
    ],
  },
  {
    step: 9,
    field: 'alcohol',
    title: 'Алкоголь дома?',
    why: 'Не про осуждение — про то, каким будет вечер пятницы',
    choices: [
      { id: 'none', label: 'Не пью' },
      { id: 'with_neighbor', label: 'С соседом за сериалом — да' },
      { id: 'parties', label: 'Тусовки — норм' },
    ],
  },
  {
    step: 10,
    field: 'expenses',
    title: 'Как тебе с общими тратами?',
    why: 'Деньги — вторая причина конфликтов соседей после уборки',
    choices: [
      { id: 'split_equally', label: 'Делю поровну и плачу сразу' },
      { id: 'tracked', label: 'Веду учёт (Splitwise и т.п.)' },
      { id: 'as_we_go', label: 'Разберёмся по ходу' },
    ],
  },
];

export const SCHEDULE_CHOICES: Choice[] =
  QUESTION_STEPS.find((q) => q.step === 3)?.choices ?? [];
