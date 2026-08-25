/* Экран анкеты — §6 «Профиль свой», правка списком.

   Проход по двенадцати шагам остался только первому заполнению (draft): менять
   один ответ, листая всю анкету, — не правка, а её повторное прохождение.
   Здесь строки «вопрос → ответ ›», каждая открывает шторку с вариантами того
   же вопроса; выбор уходит на сервер сразу и молча (§5: привычки не
   модерируются), а фото и «о себе» ждут кнопки «Отправить на проверку» —
   модератор смотрит ровно их, и уходят они вместе. */
import { useEffect, useRef, useState } from 'react';
import type { CSSProperties } from 'react';
import {
  backButton,
  hideBackButton,
  onBackButtonClick,
  showBackButton,
} from '@telegram-apps/sdk-react';

import { ApiError, patchProfile, submitProfile } from '../api/client';
import type { Profile, Questionnaire } from '../api/client';
import { ABOUT_REJECTED, PHOTO_REJECTED } from '../api/format';
import { PhotoGrid } from '../ui/PhotoGrid';
import { LimitChip, S } from '../ui/primitives';
import { AboutSheet, AgeSheet, ChoiceSheet } from './EdgeStates';
import {
  GENDER_CHOICES,
  GENDER_PREF_CHOICES,
  GENDER_PREF_HINT,
  OTHER_PETS,
  OWN_PETS,
  PETS_WHY,
  QUESTION_STEPS,
  type Choice,
  type QuestionField,
} from './OnboardingScreens';

/** Строка списка. Ключ — он же имя поля анкеты в API: патч собирается из него. */
type RowKey = 'age' | 'gender' | 'gender_pref' | 'pets_own' | 'pets_ok' | QuestionField;

/** Что открыто поверх экрана. Ключ строки — её же шторка; «о себе» стоит особняком. */
type SheetKey = RowKey | 'about';

/** Куда ходит запрос: у отказа должно быть место на экране, а не одно на всех.
 *  Фото сюда не входит — набором и его отказами владеет сетка. */
type Where = 'sheet' | 'submit';

const NOT_SET = 'Не указано';

/**
 * Лейблы строк — короткие: вопрос целиком («Как часто у тебя гости?») в
 * строку списка не влезает и читается там как допрос. Полная формулировка
 * живёт в шторке, дословно из §4.
 */
const SHORT_LABEL: Record<QuestionField, string> = {
  occupation: 'Занятость',
  schedule: 'Активен',
  smoking: 'Курение',
  noise: 'Звук дома',
  guests: 'Гости',
  cleaning: 'Уборка',
  alcohol: 'Алкоголь',
  expenses: 'Общие траты',
};

type Field = {
  key: RowKey;
  label: string;
  title: string;
  why?: string;
  /** Пусто только у возраста: там число, а не список. */
  choices?: Choice[];
};

function fromQuestion(q: (typeof QUESTION_STEPS)[number]): Field {
  return { key: q.field, label: SHORT_LABEL[q.field], title: q.title, why: q.why, choices: q.choices };
}

/** Порядок — как в онбординге (§4): человек уже видел анкету в нём. */
const FIELDS: Field[] = [
  { key: 'age', label: 'Возраст', title: 'Возраст' },
  { key: 'gender', label: 'Пол', title: 'Пол', choices: GENDER_CHOICES },
  {
    key: 'gender_pref',
    label: 'С кем готов жить',
    title: 'С кем готов жить?',
    why: GENDER_PREF_HINT,
    choices: GENDER_PREF_CHOICES,
  },
  ...QUESTION_STEPS.slice(0, 3).map(fromQuestion),
  { key: 'pets_own', label: 'Свои животные', title: 'Свои животные?', why: PETS_WHY, choices: OWN_PETS },
  { key: 'pets_ok', label: 'Чужие животные', title: 'Как к чужим?', why: PETS_WHY, choices: OTHER_PETS },
  ...QUESTION_STEPS.slice(3).map(fromQuestion),
];

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
    gap: 14,
    padding: '14px 12px 20px',
  } as CSSProperties,
  block: {
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    padding: 12,
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  row: {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    padding: '11px 2px',
    cursor: 'pointer',
  } as CSSProperties,
  rowLabel: { fontSize: 15, color: 'var(--tg-theme-subtitle-text-color)' } as CSSProperties,
  rowValue: {
    marginLeft: 'auto',
    fontSize: 15,
    fontWeight: 510,
    textAlign: 'right',
    minWidth: 0,
  } as CSSProperties,
  sep: { height: 1, background: 'var(--sep)' } as CSSProperties,
  warning: {
    padding: '9px 11px',
    borderRadius: 10,
    background: 'var(--warning-bg)',
    color: 'var(--warning-fg)',
    fontSize: 13,
    fontWeight: 590,
    lineHeight: 1.35,
  } as CSSProperties,
  error: { fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' } as CSSProperties,
};

export function ProfileEditScreen({
  profile,
  autoOpen,
  onProfile,
  onBack,
}: {
  profile: Profile;
  /** Открыть нужное сразу: строка реджекта и кнопка пуша зовут чинить одно (§5). */
  autoOpen?: 'photo' | 'about';
  /** Ответ сервера — истина: локальных правок анкеты здесь нет. */
  onProfile: (next: Profile) => void;
  onBack: () => void;
}) {
  const [sheet, setSheet] = useState<SheetKey | null>(autoOpen === 'about' ? 'about' : null);
  const [busy, setBusy] = useState<Where | null>(null);
  const [fail, setFail] = useState<{ at: Where; text: string } | null>(null);
  /** Что улетело из шторки вариантов: галочка стоит на нём, пока сервер молчит. */
  const [pending, setPending] = useState<string | null>(null);
  /**
   * Отправлено в этой сессии экрана.
   *
   * Статусы после отправки не меняются: у одобренной анкеты фото и «о себе»
   * остаются pending до вердикта (§5), и без этой отметки кнопка звала бы
   * отправить второй раз — модератору ушла бы вторая карточка на то же фото.
   */
  const [sent, setSent] = useState(false);

  // Свежий onBack в ref: оболочка передаёт inline-стрелку, и подписка иначе
  // пересобиралась бы на каждый рендер — то есть на каждый ответ сервера.
  const onBackRef = useRef(onBack);
  useEffect(() => {
    onBackRef.current = onBack;
  }, [onBack]);

  // Родная кнопка «назад» Telegram: экран открывают из профиля и из пуша, и
  // уводить с него должна та же стрелка, что и отовсюду. При открытой шторке
  // она сначала закрывает её — иначе тап по системной стрелке уносил бы с
  // экрана вместе с недописанным «о себе»; пока запрос в пути, не делает
  // ничего: шторка не должна уезжать из-под улетевшего ответа.
  useEffect(() => {
    if (!backButton.isMounted() || !showBackButton.isAvailable()) return undefined;
    showBackButton();
    return onBackButtonClick(() => {
      if (busy) return;
      if (sheet) {
        setFail(null);
        setPending(null);
        setSheet(null);
        return;
      }
      onBackRef.current();
    });
  }, [sheet, busy]);

  // Гасим стрелку при уходе с экрана — отдельным эффектом на монтирование, а
  // не в cleanup подписки выше: та пересобирается на каждое открытие шторки,
  // и стрелка мигала бы.
  useEffect(
    () => () => {
      if (hideBackButton.isAvailable()) hideBackButton();
    },
    [],
  );

  const field = sheet && sheet !== 'about' ? FIELDS.find((f) => f.key === sheet) : undefined;
  const sheetError = fail?.at === 'sheet' ? fail.text : null;
  const about = profile.about ?? '';
  // Кнопка отправки — у того, чей контент ещё не в очереди: у pending анкета
  // уже там, и перезалитое фото модератор увидит по /queue (§5).
  const submittable =
    (profile.status === 'active' || profile.status === 'draft') &&
    (profile.photo_status === 'pending' || profile.about_status === 'pending');
  // «Уйдёт» — пока карточка не в очереди; у отправленной анкеты она уже там,
  // и обещать будущее время нельзя. У набора фото та же метка во множественном
  // числе: проверяют его целиком.
  const inQueue = profile.status === 'pending' || sent;
  const reviewChip = inQueue ? 'на проверке' : 'уйдёт на проверку';

  return (
    <div style={L.screen}>
      <div style={L.header}>
        <span
          style={{
            fontSize: 22,
            lineHeight: 1,
            color: 'var(--tg-theme-accent-text-color)',
            cursor: 'pointer',
          }}
          onClick={onBack}
        >
          ‹
        </span>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Анкета</span>
      </div>

      <div style={L.body}>
        <div style={L.block}>
          <span style={{ fontSize: 20, fontWeight: 640, letterSpacing: '-0.4px' }}>
            {profile.name}
            {profile.age ? `, ${profile.age}` : ''}
          </span>

          {/* Свой набор виден всегда, даже до проверки: иначе не решить, менять
              его или нет (§5). Отправку зовёт кнопка внизу — сетка только
              собирает набор. */}
          <PhotoGrid
            profile={profile}
            onProfile={onProfile}
            onAdded={() => setSent(false)}
            reviewLabel={inQueue ? 'на проверке' : 'уйдут на проверку'}
          />

          {profile.photo_status === 'rejected' && <span style={L.warning}>{PHOTO_REJECTED}</span>}

          <div style={L.sep} />

          <div style={L.row} onClick={() => open('about')}>
            <span style={L.rowLabel}>О себе</span>
            <span
              style={{
                ...L.rowValue,
                color: about ? 'var(--tg-theme-text-color)' : 'var(--tg-theme-hint-color)',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {about || 'Не заполнено'}
            </span>
            <span style={{ flex: '0 0 auto', fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
              {about.length}/200
            </span>
            <span style={{ flex: '0 0 auto', fontSize: 17, color: 'var(--tg-theme-hint-color)' }}>
              ›
            </span>
          </div>
          {profile.about_status === 'rejected' && <span style={L.warning}>{ABOUT_REJECTED}</span>}
          {profile.about_status === 'pending' && about !== '' && (
            <span style={{ alignSelf: 'flex-start' }}>
              <LimitChip small>{reviewChip}</LimitChip>
            </span>
          )}
        </div>

        <div style={L.block}>
          <span style={S.blockTitle}>Анкета</span>

          {/* Имя приходит из Telegram и меняется там же — строка без стрелки. */}
          <div style={{ ...L.row, cursor: 'default' }}>
            <span style={L.rowLabel}>Имя</span>
            <span style={L.rowValue}>{profile.name}</span>
            <span style={{ flex: '0 0 auto', fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
              из Telegram
            </span>
          </div>

          {FIELDS.map((f) => (
            <div key={f.key}>
              <div style={L.sep} />
              <div style={L.row} onClick={() => open(f.key)}>
                <span style={L.rowLabel}>{f.label}</span>
                <span
                  style={{
                    ...L.rowValue,
                    color: valueOf(profile, f) === NOT_SET ? 'var(--tg-theme-hint-color)' : undefined,
                  }}
                >
                  {valueOf(profile, f)}
                </span>
                <span style={{ flex: '0 0 auto', fontSize: 17, color: 'var(--tg-theme-hint-color)' }}>
                  ›
                </span>
              </div>
            </div>
          ))}
        </div>

        {submittable && !sent && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {fail?.at === 'submit' && <span style={L.error}>{fail.text}</span>}
            <button
              type="button"
              style={{ ...S.filledButton, opacity: busy === 'submit' ? 0.6 : 1 }}
              disabled={busy === 'submit'}
              onClick={() => void send()}
            >
              Отправить на проверку
            </button>
            <span style={S.caption}>
              Фото и „о себе" уйдут модератору вместе; привычки уже сохранены
            </span>
          </div>
        )}
      </div>

      {sheet === 'about' && (
        <AboutSheet
          text={about}
          busy={busy === 'sheet'}
          error={sheetError}
          onSave={(text) => void save({ about: text })}
          onCancel={close}
        />
      )}

      {sheet === 'age' && (
        <AgeSheet
          age={profile.age}
          busy={busy === 'sheet'}
          error={sheetError}
          onSave={(age) => (age === profile.age ? close() : void save({ age }))}
          onCancel={close}
        />
      )}

      {field?.choices && (
        <ChoiceSheet
          title={field.title}
          why={field.why}
          choices={field.choices}
          value={answerOf(profile, field.key)}
          pending={pending}
          error={sheetError}
          onPick={(id) => {
            // Тот же ответ — не запрос: сервер записал бы то же самое, а шторку
            // и без него закрывает выбор.
            if (id === answerOf(profile, field.key)) {
              close();
              return;
            }
            setPending(id);
            void save({ [field.key]: id } as Questionnaire);
          }}
          onCancel={close}
        />
      )}
    </div>
  );

  function open(key: SheetKey) {
    setFail(null);
    setSheet(key);
  }

  function close() {
    setFail(null);
    setPending(null);
    setSheet(null);
  }

  async function call(at: Where, request: () => Promise<Profile>): Promise<boolean> {
    setBusy(at);
    setFail(null);
    try {
      onProfile(await request());
      return true;
    } catch (e) {
      setFail({ at, text: messageOf(e) });
      return false;
    } finally {
      setBusy(null);
      setPending(null);
    }
  }

  /** Ответ уходит на сервер сразу; шторку закрывает он, а не тап (§6). */
  async function save(patch: Questionnaire) {
    // Прежний текст «о себе» на сервер не отправляем: about_status он сбрасывает
    // по одному наличию поля, и блок пропал бы у всех до нового вердикта (§5) —
    // за то, что человек открыл шторку и передумал.
    if ('about' in patch && patch.about === about) {
      setSheet(null);
      return;
    }
    if (!(await call('sheet', () => patchProfile(patch)))) return;
    // Переписанный текст ушёл только в базу: модератору его отправляет кнопка.
    if ('about' in patch) setSent(false);
    setSheet(null);
  }

  async function send() {
    if (await call('submit', submitProfile)) setSent(true);
  }
}

/** Ответ анкеты как его хранит сервер. null — на вопрос ещё не отвечали. */
function answerOf(profile: Profile, key: RowKey): string | null {
  return (profile[key] as string | undefined) ?? null;
}

/** Ответ строки: лейбл выбранного варианта, иначе честное «Не указано». */
function valueOf(profile: Profile, f: Field): string {
  if (f.key === 'age') return profile.age ? String(profile.age) : NOT_SET;
  const answer = answerOf(profile, f.key);
  return f.choices?.find((c) => c.id === answer)?.label ?? NOT_SET;
}

function messageOf(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return 'Не получилось сохранить — проверь связь';
}
