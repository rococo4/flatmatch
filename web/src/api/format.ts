/* Презентация ответов API: склейка меток, плюрализация, схлопывание вердиктов.

   Отдельно от client.ts намеренно: там транспорт и типы провода, без единой
   русской строки. И один модуль на обоих потребителей — ленту и пул: иначе
   метки чипов разъедутся между экранами. Ни один экран не маппит вердикты
   у себя.

   Чистые функции, React здесь не живёт. */
import type {
  CloseReason,
  ParamOverlap,
  PoolChip,
  PoolPersonDTO,
  PoolSectionDTO,
  Questionnaire,
  SearchParamsNoZone,
  Term,
} from './client';
import type { Verdict } from '../ui/primitives';

/**
 * Четыре вердикта провода в три цвета UI.
 *
 * `grey` и `none` схлопываются в серый, но текст у них разный: `grey` —
 * значение параметра («60–80к»), `none` — «не указана» (§6). Цвет один,
 * смысл разный, поэтому маппинг цвета и сборка метки разведены.
 */
export const VERDICT_UI: Record<PoolChip['verdict'], Verdict> = {
  green: 'ok',
  edge: 'edge',
  grey: 'no',
  none: 'no',
};

/** Род слова разный — обобщать нельзя. */
const NOT_SET: Record<PoolChip['param'], string> = {
  price: 'цена не указана',
  rooms: 'комнатность не указана',
  move_in: 'заезд не указан',
  term: 'срок не указан',
};

const TERM_LABEL: Record<Term, string> = {
  half_year: 'полгода+',
  year: 'год+',
  open: 'бессрочно',
};

const SCHEDULE_LABEL: Record<PoolPersonDTO['schedule'], string> = {
  owl: 'сова',
  lark: 'жаворонок',
  floating: 'плавает',
};

const SMOKING_LABEL: Record<PoolPersonDTO['smoking'], string> = {
  no: 'не курю',
  outside: 'балкон',
  home: 'курю дома',
};

const PETS_OWN_LABEL: Record<PoolPersonDTO['pets_own'], string> = {
  none: 'без животных',
  cat: 'кот',
  dog: 'собака',
  other: 'есть питомец',
};

const PETS_OK_LABEL: Record<NonNullable<Questionnaire['pets_ok']>, string> = {
  ok: 'к чужим ок',
  allergy: 'аллергия',
  against: 'против чужих животных',
};

const NOISE_LABEL: Record<NonNullable<Questionnaire['noise']>, string> = {
  silence: 'нужна тишина',
  normal: 'шум нормально',
  loud: 'сам шумный',
};

const GUESTS_LABEL: Record<NonNullable<Questionnaire['guests']>, string> = {
  rare: 'гости редко',
  sometimes: 'гости иногда',
  often: 'гости часто',
  partner: 'партнёр регулярно ночует',
};

const CLEANING_LABEL: Record<NonNullable<Questionnaire['cleaning']>, string> = {
  schedule: 'убираю по расписанию',
  when_dirty: 'убираю, когда грязно',
  creative_mess: 'творческий беспорядок',
};

const ALCOHOL_LABEL: Record<NonNullable<Questionnaire['alcohol']>, string> = {
  none: 'не пью',
  with_neighbor: 'с соседом за сериалом',
  parties: 'тусовки — норм',
};

const EXPENSES_LABEL: Record<NonNullable<Questionnaire['expenses']>, string> = {
  split_equally: 'поровну и сразу',
  tracked: 'веду учёт трат',
  as_we_go: 'разберёмся по ходу',
};

/**
 * Девять привычек анкеты (§4) по ключам полей, как их шлёт сервер в `shared`.
 *
 * Первые три карты — те же, что рисуют строку привычек в пуле: одна метка на
 * все экраны, иначе один и тот же человек в пуле «сова», а в профиле «ночной».
 */
const FIELD_LABEL: Record<string, Record<string, string>> = {
  schedule: SCHEDULE_LABEL,
  smoking: SMOKING_LABEL,
  pets_own: PETS_OWN_LABEL,
  pets_ok: PETS_OK_LABEL,
  noise: NOISE_LABEL,
  guests: GUESTS_LABEL,
  cleaning: CLEANING_LABEL,
  alcohol: ALCOHOL_LABEL,
  expenses: EXPENSES_LABEL,
};

/** Занятость — подпись под именем. Формулировки §4, дословно. */
const OCCUPATION_LABEL: Record<string, string> = {
  study: 'Учусь',
  office: 'Офис',
  hybrid: 'Гибрид',
  remote: 'Удалёнка',
  shift: 'Сменный график',
};

/** Метка причины у карточки в архиве. Строчные — это подпись, а не кнопка. */
const CLOSE_REASON_LABEL: Record<CloseReason, string> = {
  found_here: 'нашёл здесь',
  found_elsewhere: 'нашёл в другом месте',
  changed_mind: 'передумал',
};

/** Подписи строк блока «ПО ПАРАМЕТРАМ» на экране мэтча (§6). */
const OVERLAP_LABEL: Record<ParamOverlap['param'], string> = {
  price: 'Бюджет',
  rooms: 'Комнаты',
  move_in: 'Заезд',
  term: 'Срок',
};

const SECTION_LABEL: Record<PoolSectionDTO['id'], string> = {
  all: 'СОВПАЛИ ПО ВСЕМ ПАРАМЕТРАМ',
  partial: 'ПЕРЕСЕКАЮТСЯ ЧАСТИЧНО',
  none: 'НЕ СОВПАЛИ',
};

/** Предложный падеж для «заезд в сентябре»: Intl отдаёт только именительный. */
const MONTHS_IN = [
  'январе',
  'феврале',
  'марте',
  'апреле',
  'мае',
  'июне',
  'июле',
  'августе',
  'сентябре',
  'октябре',
  'ноябре',
  'декабре',
];

const MONTH_FMT = new Intl.DateTimeFormat('ru-RU', { month: 'long' });
const DAY_MONTH_FMT = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long' });

const DAY_MS = 86_400_000;

/**
 * Часы возврата лимита лайков — всегда московские, а не телефонные.
 *
 * Сутки лимита сервер считает по Москве (продукт московский, §1), и «следующий
 * в 00:00» на телефоне с другой зоной назвал бы час, которого никто не ждёт.
 */
const MSK_TIME_FMT = new Intl.DateTimeFormat('ru-RU', {
  timeZone: 'Europe/Moscow',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
});

/**
 * Метка чипа параметра у человека в пуле.
 *
 * Сервер шлёт только вердикт — текст собирается здесь из параметров самого
 * человека, потому что чип показывает ЕГО значение, а цвет говорит, как оно
 * легло на моё.
 */
export function chipLabel(chip: PoolChip, params: SearchParamsNoZone): string {
  if (chip.verdict === 'none') return NOT_SET[chip.param];
  switch (chip.param) {
    case 'price':
      return priceShort(params);
    case 'rooms':
      return roomsShort(params.rooms_min, params.rooms_max) ?? NOT_SET.rooms;
    case 'move_in':
      return params.move_in ? monthName(params.move_in) : NOT_SET.move_in;
    case 'term':
      return params.term ? TERM_LABEL[params.term] : NOT_SET.term;
  }
}

/**
 * Чипы фильтров для шапки выдачи и карточки «Моих поисков».
 *
 * Месяц — «заезд в сентябре», а не «с 1 сентября»: дня в модели не
 * существует (в БД первое число месяца, на проводе «2026-09»), обещать его
 * нельзя. Отсутствующее просто пропускается.
 */
export function paramChips(p: SearchParamsNoZone, opts?: { zoneName?: string }): string[] {
  const out: string[] = [];
  if (opts?.zoneName) out.push(opts.zoneName);
  out.push(`${priceShort(p)} с человека`);

  const rooms = roomsLong(p);
  if (rooms) out.push(rooms);
  if (p.move_in) out.push(`заезд в ${monthIn(p.move_in)}`);
  if (p.term) out.push(TERM_LABEL[p.term]);
  return out;
}

/**
 * Строка блока «ПО ПАРАМЕТРАМ» мэтч-экрана: «Бюджет — у обоих 40–45к» (§6).
 *
 * Сервер шлёт только пересечение и только по тому, что назвали ОБА, поэтому
 * ветки «не указано» здесь нет: нечего сказать — строки не будет вовсе (null).
 * Вырожденная вилка печатается одним числом («у обоих 45к»), комнаты без
 * верхней границы — «от 2 комн.»: её не назвал никто из двоих.
 */
export function overlapRow(o: ParamOverlap): { label: string; value: string } | null {
  switch (o.param) {
    case 'price':
      return o.max != null
        ? { label: OVERLAP_LABEL.price, value: `у обоих ${priceRange(o.min ?? 0, o.max)}` }
        : null;
    case 'rooms': {
      const rooms = roomsShort(o.min, o.max);
      return rooms ? { label: OVERLAP_LABEL.rooms, value: `у обоих ${rooms}` } : null;
    }
    case 'move_in':
      return o.month
        ? { label: OVERLAP_LABEL.move_in, value: `${monthName(o.month)} у обоих` }
        : null;
    case 'term':
      return o.term ? { label: OVERLAP_LABEL.term, value: `${TERM_LABEL[o.term]} у обоих` } : null;
  }
}

/**
 * Строка привычек мини-карточки: «сова · не курю · без животных».
 *
 * Берёт три поля, а не человека пула целиком: ровно их же шлёт карточка
 * входящего лайка, и одна метка должна обслуживать оба экрана.
 */
export function habitsLine(p: Pick<PoolPersonDTO, 'schedule' | 'smoking' | 'pets_own'>): string {
  return [SCHEDULE_LABEL[p.schedule], SMOKING_LABEL[p.smoking], PETS_OWN_LABEL[p.pets_own]]
    .filter(Boolean)
    .join(' · ');
}

/**
 * Метка ответа анкеты: «сова», «не курю», «убираю по расписанию».
 *
 * Незнакомая пара поле-значение (схема ушла вперёд бинаря) отдаётся как есть:
 * сырое значение честнее пустой строки.
 */
export function fieldLabel(field: string, value: string): string {
  return FIELD_LABEL[field]?.[value] ?? value;
}

/** Занятость человека: «Учусь», «Офис». Незаполненная — пустая строка. */
export function occupationLabel(value?: string): string {
  return value ? (OCCUPATION_LABEL[value] ?? '') : '';
}

/** Час по Москве, «14:30»: когда вернётся истраченный лайк (§2.3). */
export function moscowTime(iso: string): string {
  return MSK_TIME_FMT.format(new Date(iso));
}

/** Заголовок секции пула: «СОВПАЛИ ПО ВСЕМ ПАРАМЕТРАМ · 3». */
export function sectionTitle(id: PoolSectionDTO['id'], n: number): string {
  return `${SECTION_LABEL[id]} · ${n}`;
}

/**
 * Почему поиск в архиве: «нашёл здесь» / «нашёл в другом месте» / «передумал».
 *
 * Пустая строка у поиска без причины — это пауза по молчанию (§3), а не
 * закрытие: выдумывать ей причину нельзя.
 */
export function closeReasonLabel(reason?: CloseReason): string {
  return reason ? CLOSE_REASON_LABEL[reason] : '';
}

/** Русская плюрализация счётчиков: 1 человек / 2 человека / 5 человек. */
export function plural(n: number, one: string, few: string, many: string): string {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}

/** Деньги на карточках точек: «90 000 ₽». */
export function money(n: number): string {
  return `${n.toLocaleString('ru-RU')} ₽`;
}

/** Дата без года: «12 июля». Точнее дня свежесть не обещаем (§8). */
export function dayMonth(iso: string): string {
  return DAY_MONTH_FMT.format(new Date(iso));
}

/**
 * Когда это было: «сегодня» / «вчера» / «26 июля».
 *
 * Считается от полуночи, а не «за последние 24 часа»: вчерашний вечер — это
 * вчера, даже если прошло меньше суток.
 */
export function relativeDay(iso: string): string {
  const midnight = new Date();
  midnight.setHours(0, 0, 0, 0);
  const ago = midnight.getTime() - new Date(iso).getTime();
  if (ago <= 0) return 'сегодня';
  if (ago <= DAY_MS) return 'вчера';
  return dayMonth(iso);
}

/**
 * Ближайшие месяцы для чипов заезда, начиная с текущего.
 *
 * Готовый список, а не `<input type="month">`: нативный пикер месяца в
 * WebView Telegram ведёт себя по-разному, а библиотеку добавлять нельзя.
 */
export function monthOptions(count = 6): { value: string; label: string }[] {
  const now = new Date();
  const out: { value: string; label: string }[] = [];
  for (let i = 0; i < count; i++) {
    const d = new Date(now.getFullYear(), now.getMonth() + i, 1);
    const month = String(d.getMonth() + 1).padStart(2, '0');
    out.push({ value: `${d.getFullYear()}-${month}`, label: withYear(MONTH_FMT.format(d), d) });
  }
  return out;
}

/** «35–45к» или «до 45к»: снизу вилка открыта чаще, чем сверху. */
function priceShort(p: SearchParamsNoZone): string {
  return p.price_min != null ? `${thousands(p.price_min)}–${k(p.price_max)}` : `до ${k(p.price_max)}`;
}

/** Вилка пересечения: «40–45к», «45к» у вырожденной, «до 45к» без нижней границы. */
function priceRange(min: number, max: number): string {
  if (min >= max) return k(max);
  if (min <= 0) return `до ${k(max)}`;
  return `${thousands(min)}–${k(max)}`;
}

/** Деньги коротко: 45000 → «45к». Буква одна на вилку, отсюда и thousands(). */
function k(n: number): string {
  return `${thousands(n)}к`;
}

function thousands(n: number): number {
  return Math.round(n / 1000);
}

/** Короткая комнатность для вердикт-чипа: «2 комн.» / «2–3 комн.» / «от 3 комн.». */
function roomsShort(min?: number | null, max?: number | null): string | null {
  if (min != null && max != null) return min === max ? `${min} комн.` : `${min}–${max} комн.`;
  if (min != null) return `от ${min} комн.`;
  if (max != null) return `до ${max} комн.`;
  return null;
}

/** Развёрнутая комнатность для чипов фильтров: «2 комнаты» / «от 4 комнат». */
function roomsLong(p: SearchParamsNoZone): string | null {
  const min = p.rooms_min ?? null;
  const max = p.rooms_max ?? null;
  const nominative = (n: number) => `${n} ${plural(n, 'комната', 'комнаты', 'комнат')}`;
  // «от» и «до» требуют родительного: «от 1 комнаты», «от 4 комнат».
  const genitive = (n: number) => `${n} ${plural(n, 'комнаты', 'комнат', 'комнат')}`;

  if (min != null && max != null) {
    return min === max ? nominative(min) : `${min}–${max} ${plural(max, 'комната', 'комнаты', 'комнат')}`;
  }
  if (min != null) return `от ${genitive(min)}`;
  if (max != null) return `до ${genitive(max)}`;
  return null;
}

/** «сентябрь», с годом — если он не текущий. */
function monthName(value: string): string {
  const d = monthDate(value);
  return withYear(MONTH_FMT.format(d), d);
}

/** «сентябре» — предложный, для строки «заезд в …». */
function monthIn(value: string): string {
  const d = monthDate(value);
  return withYear(MONTHS_IN[d.getMonth()], d);
}

/** Год дописывается только когда он не текущий: иначе это шум в каждом чипе. */
function withYear(label: string, d: Date): string {
  return d.getFullYear() === new Date().getFullYear() ? label : `${label} ${d.getFullYear()}`;
}

/** «2026-09» → первое число этого месяца. Дня в модели нет. */
function monthDate(value: string): Date {
  const [year, month] = value.split('-').map(Number);
  return new Date(year, month - 1, 1);
}
