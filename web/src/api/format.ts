/* Презентация ответов API: склейка меток, плюрализация, схлопывание вердиктов.

   Отдельно от client.ts намеренно: там транспорт и типы провода, без единой
   русской строки. И один модуль на обоих потребителей — ленту и пул: иначе
   метки чипов разъедутся между экранами. Ни один экран не маппит вердикты
   у себя.

   Чистые функции, React здесь не живёт. */
import type {
  Bathrooms,
  CloseReason,
  ParamOverlap,
  PointDTO,
  PointStage,
  PointStatus,
  PointType,
  PoolChip,
  PoolPersonDTO,
  PoolSectionDTO,
  Questionnaire,
  Restriction,
  SearchParamsNoZone,
  Term,
  Utilities,
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

/** Бейдж стадии «к себе» — дословно §2.2, обе формулировки от лица автора. */
const STAGE_BADGE: Record<PointStage, string> = {
  live: 'живёт здесь',
  moving_in: 'заезжает · ищет со-съёмщика со старта',
};

/**
 * Строка статуса своей точки.
 *
 * У активной строки нет: «в ленте» — это норма, а не новость. Закрытую «Мои
 * поиски» не показывают вовсе, и метка ей нужна только на случай, если сервер
 * когда-нибудь её отдаст.
 */
const POINT_STATUS_LABEL: Record<PointStatus, string> = {
  unknown: '',
  pending: 'на проверке',
  active: '',
  paused: 'скрыта',
  closed: 'закрыта',
  rejected: 'отклонена',
};

/** Развёрнуто — для строки фактов карточки. */
const BATHROOMS_LABEL: Record<Bathrooms, string> = {
  combined: '1 совмещённый',
  separate: '1 раздельный',
  two: '2',
};

/** Коротко — для чипов формы, где три варианта стоят в ряд. */
const BATHROOMS_SHORT: Record<Bathrooms, string> = {
  combined: '1 совм.',
  separate: '1 разд.',
  two: '2',
};

const UTILITIES_LABEL: Record<Utilities, string> = {
  included: 'включена',
  meters: 'по счётчикам',
};

/**
 * Одно ограничение — две формулировки: у квартиры с рынка запрещает
 * собственник, у «к себе» рассказывает тот, кто там живёт (§2.2, §6).
 */
const RESTRICTION_LABEL: Record<PointType, Record<Restriction, string>> = {
  flat: {
    no_pets: 'без животных',
    no_couples: 'не сдают парам',
    no_smoking: 'курить нельзя',
  },
  intent_home: {
    no_pets: 'нельзя с животными',
    no_couples: 'хозяин против пар',
    no_smoking: 'курить нельзя',
  },
};

/** Порядок чипов ограничений в форме — он же порядок на карточке. */
const RESTRICTIONS: Restriction[] = ['no_pets', 'no_couples', 'no_smoking'];

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

const MINUTE_MS = 60_000;
const HOUR_MS = 3_600_000;

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
  return habitChips(p).join(' · ');
}

/** Те же три привычки чипами — блоком «Привычки» на карточке с лицом. */
export function habitChips(p: Pick<PoolPersonDTO, 'schedule' | 'smoking' | 'pets_own'>): string[] {
  return [SCHEDULE_LABEL[p.schedule], SMOKING_LABEL[p.smoking], PETS_OWN_LABEL[p.pets_own]].filter(
    Boolean,
  );
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

/**
 * Сколько ждать возврата лайка: «3 ч 20 мин», «40 мин», «минуту» (§2.3).
 *
 * Относительное, а не «в 15:00»: абсолютный час без даты двусмыслен —
 * потратил в 15:00, «освободится в 15:00» читается как «прямо сейчас»,
 * хотя значит «завтра».
 */
export function untilTime(iso: string): string {
  const ms = new Date(iso).getTime() - Date.now();
  if (ms < MINUTE_MS) return 'минуту';
  const h = Math.floor(ms / HOUR_MS);
  const m = Math.floor((ms % HOUR_MS) / MINUTE_MS);
  if (h === 0) return `${m} мин`;
  if (m === 0) return `${h} ч`;
  return `${h} ч ${m} мин`;
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

/* ── Точки (§2.2, §6) ───────────────────────────────────────────────────── */

/**
 * Заголовок карточки точки: «2-комнатная · м. Фрунзенская», «Комната · м. Сокол».
 *
 * Точка всегда стоит на станции (§5), поэтому «м.» — не догадка. Зона, которой
 * справочник больше не знает, оставляет один тип: «м.» без названия хуже.
 */
export function pointTitle(p: Pick<PointDTO, 'type' | 'rooms' | 'zone'>): string {
  const kind =
    p.type === 'intent_home' ? 'Комната' : p.rooms ? `${p.rooms}-комнатная` : 'Квартира';
  return p.zone.name ? `${kind} · м. ${p.zone.name}` : kind;
}

/**
 * Название точки в родительном: «2-комнатной у м. Фрунзенская».
 *
 * Нужно ровно одной строке — вопросу шторки «Выйти из пула {чего}?»: там
 * именительный из pointTitle читался бы поломанным.
 */
export function pointTitleGen(p: Pick<PointDTO, 'type' | 'rooms' | 'zone'>): string {
  const kind =
    p.type === 'intent_home' ? 'комнаты' : p.rooms ? `${p.rooms}-комнатной` : 'квартиры';
  return p.zone.name ? `${kind} у м. ${p.zone.name}` : kind;
}

/**
 * Подпись ссылки на оригинал: «Открыть на Авито».
 *
 * Источник берётся из самой ссылки, а не из поля: доменов всего два (§5), и
 * отдельного `source` карточка не несёт. Незнакомый хост — «Открыть оригинал»:
 * врать про площадку хуже, чем не называть её.
 */
export function openOriginalLabel(url: string): string {
  if (url.includes('avito.')) return 'Открыть на Авито';
  if (url.includes('cian.')) return 'Открыть на Циан';
  return 'Открыть оригинал';
}

/** «живёт здесь» / «заезжает · ищет со-съёмщика со старта». */
export function stageBadge(stage: PointStage): string {
  return STAGE_BADGE[stage] ?? '';
}

/** «на проверке» / «скрыта». У активной точки строки нет — это норма. */
export function pointStatusLabel(status: PointStatus): string {
  return POINT_STATUS_LABEL[status] ?? '';
}

/** «добавлена вчера» / «добавлена 5 дней назад». Точность до дня (§8). */
export function addedLabel(daysAgo: number): string {
  if (daysAgo <= 0) return 'добавлена сегодня';
  if (daysAgo === 1) return 'добавлена вчера';
  return `добавлена ${daysAgo} ${plural(daysAgo, 'день', 'дня', 'дней')} назад`;
}

/**
 * Когда автор «к себе» заходил: «в сети вчера».
 *
 * Без рода: «была вчера» из §6 — про Иру из макета, а пол автора карточка не
 * знает и знать не должна (§4 — это правило видимости, не поле витрины).
 */
export function lastSeenLabel(iso: string): string {
  return `в сети ${relativeDay(iso)}`;
}

/**
 * Цена доли рыночной квартиры: аренда, делённая на комнаты.
 *
 * Без комнатности доли нет: делить «на двоих» наугад — это обещание, которого
 * объявление не давало. Округляем до тысяч, чтобы не показывать 31 666 ₽.
 */
export function pricePerPerson(total?: number, rooms?: number): number | null {
  if (total == null || rooms == null || rooms < 1) return null;
  return Math.round(total / rooms / 1000) * 1000;
}

/**
 * Факты квартиры с рынка — одни и те же в ленте и на карточке в пуле.
 *
 * Неназванное объявлением просто отсутствует: строка «Санузлы — не указано»
 * рассказывает про наши поля, а не про квартиру.
 */
export function flatFacts(p: PointDTO): { label: string; value: string }[] {
  const facts: { label: string; value: string }[] = [];
  if (p.zone.name) facts.push({ label: 'Метро', value: p.zone.name });
  if (p.rooms) {
    facts.push({
      label: 'Комнаты',
      value:
        p.isolated == null ? String(p.rooms) : `${p.rooms}, ${p.isolated ? 'изолированные' : 'проходные'}`,
    });
  }
  if (p.bathrooms) facts.push({ label: 'Санузлы', value: bathroomsLabel(p.bathrooms) });
  if (p.utilities) facts.push({ label: 'Коммуналка', value: utilitiesLabel(p.utilities) });
  if (p.deposit) facts.push({ label: 'Депозит', value: money(p.deposit) });
  return facts;
}

export function bathroomsLabel(value: Bathrooms): string {
  return BATHROOMS_LABEL[value] ?? '';
}

export function utilitiesLabel(value: Utilities): string {
  return UTILITIES_LABEL[value] ?? '';
}

/** Метки ограничений для карточки: формулировка зависит от типа точки. */
export function restrictionLabels(codes: Restriction[] | undefined, type: PointType): string[] {
  return (codes ?? []).map((code) => RESTRICTION_LABEL[type][code] ?? code);
}

/** Чипы формы: код едет на сервер, метка стоит на экране. */
export function restrictionOptions(type: PointType): { value: Restriction; label: string }[] {
  return RESTRICTIONS.map((value) => ({ value, label: RESTRICTION_LABEL[type][value] }));
}

export function bathroomsOptions(): { value: Bathrooms; label: string }[] {
  return (['combined', 'separate', 'two'] as Bathrooms[]).map((value) => ({
    value,
    label: BATHROOMS_SHORT[value],
  }));
}

export function utilitiesOptions(): { value: Utilities; label: string }[] {
  return (['included', 'meters'] as Utilities[]).map((value) => ({
    value,
    label: UTILITIES_LABEL[value],
  }));
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
