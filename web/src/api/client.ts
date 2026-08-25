import { retrieveLaunchParams, retrieveRawInitData } from '@telegram-apps/sdk-react';

/** Ответы анкеты (§4). Каждое поле — из закрытого набора, кроме возраста и «о себе». */
export interface Questionnaire {
  age?: number;
  gender?: 'male' | 'female';
  gender_pref?: 'any' | 'same';
  occupation?: 'study' | 'office' | 'hybrid' | 'remote' | 'shift';
  schedule?: 'owl' | 'lark' | 'floating';
  smoking?: 'no' | 'outside' | 'home';
  pets_own?: 'none' | 'cat' | 'dog' | 'other';
  pets_ok?: 'ok' | 'allergy' | 'against';
  noise?: 'silence' | 'normal' | 'loud';
  guests?: 'rare' | 'sometimes' | 'often' | 'partner';
  cleaning?: 'schedule' | 'when_dirty' | 'creative_mess';
  alcohol?: 'none' | 'with_neighbor' | 'parties';
  expenses?: 'split_equally' | 'tracked' | 'as_we_go';
  about?: string;
}

/**
 * Вердикт модератора по одному куску контента. Фото и «о себе» ходят по
 * очереди независимо: новое фото может ждать проверки, пока текст уже одобрен.
 */
export type ModerationStatus = 'unknown' | 'pending' | 'ok' | 'rejected';

/** Аккаунт FlatMatch, как его отдаёт `GET /api/v1/me`. */
export interface Profile extends Questionnaire {
  id: string;
  tg_id: number;
  name: string;
  tg_username?: string;
  /**
   * Своё фото человек видит всегда, даже до проверки — иначе нельзя решить,
   * менять его или нет. Гейт по photo_status касается чужих глаз (§5).
   */
  photo_url?: string;
  photo_status: ModerationStatus;
  about_status: ModerationStatus;
  /**
   * draft — анкета не дозаполнена, туда же возвращает отклонённое фото, потому
   * что оно обязательно (§4); pending — отправлена, ждёт модератора; active —
   * одобрена и видна; banned (§5).
   *
   * unknown значит, что API прочитал из базы значение, которого не знает.
   * Появляться не должно, перечислено чтобы switch оставался исчерпывающим.
   */
  status: 'unknown' | 'draft' | 'pending' | 'active' | 'banned';
  /** Закладка «на каком шаге бросил», 0..11. Порядком экранов владеет фронт. */
  onboarding_step: number;
  created_at: string;
  /**
   * Бот, в котором открыт Mini App, — из getMe. Пусто, если поллинг выключен
   * или Telegram ещё не ответил: тогда шеринг падает на свою константу.
   */
  bot_username?: string;
}

/* ── Зоны, поиски, пулы (§2.1, §6) ──────────────────────────────────────────
   Имена полей — как в JSON, snake_case. Переименования при разборе тут нет
   намеренно: тип провода должен читаться рядом с ответом сервера, иначе
   расхождение контракта ловится не компилятором, а глазами. Русские строки
   и склейка меток живут в api/format.ts — здесь только транспорт. */

/** Вид зоны. Пересечение зон считает сервер по множествам станций (§8). */
export type ZoneKind = 'district' | 'station' | 'line';

/**
 * Зона справочника.
 *
 * Склонения приходят готовыми, морфологии на фронте нет: `name_in` уже с
 * предлогом («в Хамовниках» / «на красной ветке» / «у метро Сокол»),
 * `name_gen` — родительный («Хамовников»). Дописывать к ним «в» нельзя.
 *
 * `stations` фронту не нужен ни для чего — логику на нём не строить.
 */
export interface Zone {
  id: string;
  kind: ZoneKind;
  name: string;
  name_in: string;
  name_gen: string;
  stations: string[];
}

export interface ZoneStation {
  id: string;
  name: string;
  lat: number;
  lng: number;
}

/** Справочник целиком, как он лежит в репозитории бэкенда: ~450 зон, ~210 КБ. */
export interface ZoneDirectory {
  version: number;
  stations: ZoneStation[];
  zones: Zone[];
}

/**
 * Строка «Где уже ищут» из пустой ленты (§6).
 *
 * `people` — сколько ВИДИМЫХ смотрящему людей с активным поиском пересекают
 * зону, поэтому счётчик персональный, как и все остальные. Зон с нулём в
 * ответе нет: звать в пустую зону из пустой зоны незачем.
 */
export interface TopZoneDTO {
  id: string;
  name: string;
  name_in: string;
  people: number;
}

/**
 * Зона в ответах поисков и пулов — только имена и склонения.
 *
 * Урезана намеренно: множество станций там не нужно, а гонять его в каждом
 * элементе списка поисков — лишние килобайты на каждый вызов.
 */
export interface ZoneRef {
  id: string;
  name: string;
  name_in: string;
  name_gen: string;
}

/** Срок аренды. Мягкий параметр: влияет на секцию пула, но не отсекает (§2.1). */
export type Term = 'half_year' | 'year' | 'open';

/** «нашёл здесь» — ГЛАВНАЯ МЕТРИКА продукта (§3), поэтому причина обязательна. */
export type CloseReason = 'found_here' | 'found_elsewhere' | 'changed_mind';

/** unknown — значение уровня кода: схема ушла вперёд бинаря (как у Profile.status). */
export type SearchStatus = 'unknown' | 'active' | 'paused' | 'closed';

/**
 * Параметры поиска — они же фильтры ленты: поиск это снапшот фильтров (§2.1).
 *
 * Обязательны только зона (граница пула) и «до» по цене; комнатность, срок и
 * заезд — НАБОРЫ, и пустой набор означает «неважно», совпадающее со всем.
 */
export interface SearchParams {
  zone_id: string;
  price_min?: number | null;
  price_max: number;
  /** Комнатность набором: 1–4, где 4 значит «4+». */
  rooms: number[];
  terms: Term[];
  /** Месяцы заезда, «2026-09». Дня в модели нет — обещать его нельзя. */
  move_in: string[];
  has_stuff?: string[];
}

/** Те же параметры там, где зона лежит отдельно: в поиске и у человека в пуле. */
export type SearchParamsNoZone = Omit<SearchParams, 'zone_id'>;

/**
 * Поиск-подписка.
 *
 * Счётчики заполнены только у активных и только в ответе `GET /searches`: в
 * ответах на мутации нули стоят не потому что ноль, а потому что не считали —
 * после любой мутации список надо перечитать.
 */
export interface SearchResponse extends SearchParamsNoZone {
  id: string;
  zone: ZoneRef;
  status: SearchStatus;
  close_reason?: CloseReason;
  created_at: string;
  people: number;
  full_match: number;
  /** «M новых» — гаснет побочным эффектом захода в пул этой зоны. */
  new_count: number;
  /** «M квартир под эти параметры»: активные точки зон, пересекающих зону поиска. */
  points_count: number;
}

/* ── Подбор «Соседи» (§2.1, §6) ─────────────────────────────────────────────
   Одна колода на все активные поиски смотрящего: люди и комнаты «к себе»
   вперемешку, человек в ней один раз. Списками экрана и пулами больше никто
   не ходит — слова «пул» в интерфейсе нет. */

/**
 * Вердикт параметра против поиска смотрящего.
 *
 * На проводе четыре значения, в UI три: `grey` и `none` рисуются одним серым
 * цветом, но разным текстом — `grey` показывает значение параметра, `none`
 * говорит «не указана» (§6). Схлопывание живёт в api/format.ts.
 */
export interface PairChip {
  param: 'price' | 'rooms' | 'move_in' | 'term';
  verdict: 'green' | 'edge' | 'grey' | 'none';
}

/**
 * Три поля анкеты, из которых собирается строка привычек.
 *
 * Отдельный тип, а не Pick с чьей-то карточки: одну и ту же строку рисуют
 * карточка подбора, автор комнаты и входящий лайк, и общий предок у них
 * только этот.
 */
export interface HabitsFields {
  schedule: NonNullable<Questionnaire['schedule']>;
  smoking: NonNullable<Questionnaire['smoking']>;
  pets_own: NonNullable<Questionnaire['pets_own']>;
}

/**
 * Лучшая пара «мой поиск × его поиск» — вся карточка подбора про неё (§2.1).
 *
 * Выбор пары считает сервер: выше секция → больше зелёных чипов → мой более
 * свежий поиск. `chips` и `params` приезжают только у человека: у комнаты
 * сравнивать нечего, кроме цены доли, и она уже в секции.
 */
export interface DeckPair {
  zone: ZoneRef;
  /** Зоны равны или вложены — «твоя зона» на бейдже. */
  zone_match: boolean;
  section: 'all' | 'partial' | 'none';
  chips?: PairChip[];
  params?: SearchParamsNoZone;
}

/** Человек в подборе: анкета целиком, кроме контактов (они живут в мэтче). */
export interface DeckPerson extends HabitsFields {
  user_id: string;
  name: string;
  age: number;
  occupation: NonNullable<Questionnaire['occupation']> | 'none';
  last_seen: string;
  /** Первое — главное. Пустой массив = набор не одобрен, вместо него заглушка (§5). */
  photo_urls: string[];
  pets_ok: NonNullable<Questionnaire['pets_ok']>;
  noise: NonNullable<Questionnaire['noise']>;
  guests: NonNullable<Questionnaire['guests']>;
  cleaning: NonNullable<Questionnaire['cleaning']>;
  alcohol: NonNullable<Questionnaire['alcohol']>;
  /** Пусто, пока «о себе» не одобрено (§5). */
  about: string;
}

/** Автор комнаты — строка внизу её карточки; тап по ней ведёт в его анкету. */
export interface DeckRoomAuthor extends HabitsFields {
  user_id: string;
  name: string;
  age: number;
  occupation: NonNullable<Questionnaire['occupation']> | 'none';
  photo_urls: string[];
  noise: NonNullable<Questionnaire['noise']>;
}

/** Комната «к себе» в подборе (§2.2). Рыночных квартир в колоде нет вовсе. */
export interface DeckRoom {
  id: string;
  stage: PointStage;
  zone: ZoneRef;
  price_share?: number;
  utilities?: Utilities;
  isolated?: boolean;
  rooms?: number;
  bathrooms?: Bathrooms;
  deposit?: number;
  /** День заезда, «2026-09-15»: у точки это дата, а не месяц как у поиска. */
  move_in?: string;
  description?: string;
  restrictions: Restriction[];
  photo_urls: string[];
  responses: number;
  author: DeckRoomAuthor;
}

export type DeckCard =
  | { kind: 'person'; person: DeckPerson; pair: DeckPair }
  | { kind: 'room'; room: DeckRoom; pair: DeckPair };

export interface DeckResponse {
  /** До 50 карточек в порядке подбора: секция пары, внутри — свежие вперёд. */
  cards: DeckCard[];
  /** Активного поиска нет — колоды не существует (§2.1), экран зовёт в форму. */
  no_search: boolean;
  /** Сколько всего подошло: шапка «Соседи · N в твоих зонах». */
  total: number;
  /** Сколько пропущенных ждут возврата: «Показать пропущенных · N». */
  passed: number;
}

/** Кого показывать. Пустой набор = обоих: «пусто = неважно» (§6). */
export type DeckWho = 'people' | 'rooms';

export type DeckPets = 'none' | 'ok_mine';

export type DeckSmoking = 'no' | 'outside_only';

export type DeckSchedule = 'owl' | 'lark';

export type DeckOccupation = 'study' | 'working' | 'none';

/**
 * Фильтры подбора (§6, шторка «Каким должен быть сосед»).
 *
 * Правилом видимости они НЕ являются (§4 — это только пол и блоки): сужают
 * колоду по анкете и живут на клиенте, уходя параметрами запроса.
 */
export interface DeckFilters {
  who: DeckWho[];
  age_min: number | null;
  age_max: number | null;
  pets: DeckPets[];
  smoking: DeckSmoking[];
  schedule: DeckSchedule[];
  occupation: DeckOccupation[];
}

export const NO_DECK_FILTERS: DeckFilters = {
  who: [],
  age_min: null,
  age_max: null,
  pets: [],
  smoking: [],
  schedule: [],
  occupation: [],
};

/** Хоть одно сужение задано: синяя точка на иконке фильтров. */
export function hasDeckFilters(f: DeckFilters): boolean {
  return (
    f.who.length > 0 ||
    f.age_min != null ||
    f.age_max != null ||
    f.pets.length > 0 ||
    f.smoking.length > 0 ||
    f.schedule.length > 0 ||
    f.occupation.length > 0
  );
}

/* ── Точки: квартиры с рынка и «сосед к себе» (§2.2, §6) ────────────────────
   Словарные значения едут кодами — русские метки разные у двух типов карточек
   («без животных» на квартире, «нельзя с животными» у «к себе»), и собирает их
   api/format.ts. */

/** Тип точки решает всё: у `flat` пул и дешёвый вход, у `intent_home` — отклики. */
export type PointType = 'flat' | 'intent_home';

/** Стадия «к себе»: уже живёт или заезжает. У квартиры с рынка стадии нет. */
export type PointStage = 'live' | 'moving_in';

/**
 * Статус точки, как его отдаёт карточка.
 *
 * `paused` — «похоже, сдана» (скрыта голосами, §8), а закрытая приезжает не
 * словом `closed`, а своей причиной: `gone` — сдана, `expired` — истёк срок,
 * `found` — «к себе» нашёл соседа, `rejected` — не прошла модерацию. Голый
 * `closed` остаётся закрытием руками. unknown — значение уровня кода, как у
 * Profile.status.
 */
export type PointStatus =
  | 'unknown'
  | 'pending'
  | 'active'
  | 'paused'
  | 'closed'
  | 'rejected'
  | 'gone'
  | 'expired'
  | 'found';

/** «раздельный» важнее числа: вторая дверь для двух незнакомцев — это про быт. */
export type Bathrooms = 'combined' | 'separate' | 'two';

export type Utilities = 'included' | 'meters';

/** Ограничения собственника — коды, показываются ДО дорогого лайка (§2.2). */
export type Restriction = 'no_pets' | 'no_couples' | 'no_smoking';

/**
 * Автор «к себе» — единственная карточка с лицом (§2.2).
 *
 * Контактов не несёт: @username живёт только в мэтче (§3). Пустой `photo_url` =
 * фото не одобрено, вместо него заглушка (§5).
 */
export interface PointAuthorDTO {
  user_id: string;
  name: string;
  age: number;
  photo_url: string;
  occupation?: NonNullable<Questionnaire['occupation']>;
  schedule: NonNullable<Questionnaire['schedule']>;
  smoking: NonNullable<Questionnaire['smoking']>;
  pets_own: NonNullable<Questionnaire['pets_own']>;
  last_seen: string;
}

/**
 * Мини-карточка точки-контекста: «пересеклись: {точка}» в профиле, контекст
 * входящего лайка, блок ГДЕ на мэтче.
 *
 * `label` приходит готовым к печати и разным у трёх потребителей: адресату
 * отклика сервер пишет «на твою комнату · м. Сокол», смотрящему профиль —
 * «на комнату Кати · м. Сокол». Собирать эту строку на клиенте нельзя.
 */
export interface PointRef {
  id: string;
  kind: PointType;
  label: string;
}

/**
 * Точка, как её показывают все экраны: лента, карточка, оба блока «Моих поисков».
 *
 * `created_by` тут нет и не будет: квартира с рынка обезличена (§2.2). Лицо есть
 * только у «к себе» и приезжает в `author`.
 *
 * Счётчики персональные и по типу разные: `pool_count` — «в пуле N» квартиры,
 * `responses` — «N откликов» у «к себе». `in_pool` — вошёл ли смотрящий.
 */
export interface PointDTO {
  id: string;
  type: PointType;
  stage?: PointStage;
  status: PointStatus;
  zone: ZoneRef;

  price_total?: number;
  price_share?: number;
  rooms?: number;
  isolated?: boolean;
  bathrooms?: Bathrooms;
  utilities?: Utilities;
  restrictions?: Restriction[];
  /** Первое — обложка карточки; остальные листаются на экране точки. */
  photo_urls: string[];
  /** Нормализованная ссылка на оригинал; есть только у квартир с рынка. */
  url?: string;
  /** День заезда, «2026-09-15». У точки это дата, а не месяц как у поиска. */
  move_in?: string;
  deposit?: number;

  /** «добавлена N дней назад» — точность до дня, «20 минут назад» не обещаем (§8). */
  created_days_ago: number;
  /** Последнее подтверждение жизни: создание, голос «живая», восстановление, ответ на пинг (§8). */
  last_seen_alive: string;
  /** Момент реальной проверки; нет поля — точку после создания никто не подтверждал. */
  confirmed_at?: string;
  /** Сколько человек отметили «уже сдана» — строка свежести и плашка паузы. */
  gone_votes: number;
  /** Мой голос, если он есть: true — «живая», false — «сдана». Нет поля — не голосовал. */
  my_vote?: boolean;
  /** Моя квартира на паузе: её возвращает «Проверил — она живая» (§8). */
  restorable?: boolean;
  pool_count: number;
  responses: number;
  in_pool: boolean;

  author?: PointAuthorDTO;
}

/** Тело формы «Зову соседа к себе» (§2.2). Заезд обязателен у `moving_in`. */
export interface CreateHomePoint {
  type: 'intent_home';
  stage: PointStage;
  zone_id: string;
  price_share: number;
  rooms?: number | null;
  isolated?: boolean | null;
  restrictions?: Restriction[];
  /** До 10; первый ключ — обложка. */
  photo_keys: string[];
  move_in?: string | null;
  deposit?: number | null;
}

/** Тип один: рыночные квартиры выключены, и сервер их тело отбивает (§2.2). */
export type CreatePointBody = CreateHomePoint;

export interface MyPointsResponse {
  /** Мои точки во всех статусах, кроме закрытых: они и считаются в лимит. */
  created: PointDTO[];
  /** Чужие квартиры, в чьи пулы я вошёл. Лимита у них нет. */
  entered: PointDTO[];
}

/**
 * Где пересеклись: зона пула или точка. Взаимоисключимы — лайк несёт один
 * контекст (§2.3), и оба разом сервер отбивает 400 `invalid_field`.
 */
export interface MeetingContext {
  zoneId?: string;
  pointId?: string;
}

/* ── Профиль, дорогой лайк, мэтчи (§2.3, §6) ────────────────────────────────
   Те же правила, что и выше: snake_case как на проводе, русских строк нет. */

/** Один ответ анкеты, который двое дали одинаково. `field` — ключ анкеты. */
export interface SharedAnswer {
  field: string;
  value: string;
}

/**
 * Остаток дорогих лайков: 5 слотов на скользящем окне в 24 часа (§2.3).
 *
 * `next_free_at` нет, пока ничего не потрачено: возвращаться нечему. Время —
 * ISO с зоной, показывать его надо по Москве.
 */
export interface LikeBudget {
  remaining: number;
  next_free_at?: string;
}

/**
 * Экран профиля человека целиком.
 *
 * Контактов здесь нет и не будет: @username и tg_id живут только в мэтче (§3).
 * `about` отсутствует, пока «о себе» не одобрено модератором (§5) — блок тогда
 * не рисуется вовсе, а не пустым.
 */
export interface PersonProfileDTO {
  user_id: string;
  name: string;
  age: number;
  /** Пустая строка = фото не одобрено: вместо него заглушка с инициалами (§5). */
  photo_url: string;

  occupation: NonNullable<Questionnaire['occupation']>;
  schedule: NonNullable<Questionnaire['schedule']>;
  smoking: NonNullable<Questionnaire['smoking']>;
  pets_own: NonNullable<Questionnaire['pets_own']>;
  pets_ok: NonNullable<Questionnaire['pets_ok']>;
  noise: NonNullable<Questionnaire['noise']>;
  guests: NonNullable<Questionnaire['guests']>;
  cleaning: NonNullable<Questionnaire['cleaning']>;
  alcohol: NonNullable<Questionnaire['alcohol']>;
  expenses: NonNullable<Questionnaire['expenses']>;
  about?: string;

  shared: SharedAnswer[];
  /** Пул, из которого открыт профиль: «пересеклись: {зона}». Может не быть. */
  context_zone?: ZoneRef;
  /** То же для точки: пул квартиры или карточка «к себе». Взаимоисключим с зоной. */
  context_point?: PointRef;

  liked_me: boolean;
  my_like: boolean;
  /** Вы уже в мэтче: кнопка ведёт в него, лайкать нечего. */
  match_id?: string;
  budget: LikeBudget;
}

/**
 * Ответ на дорогой лайк.
 *
 * `already` — повторный тап: лимит не тронут, контекст остался от первого.
 * Взаимный лайк отвечает `matched` вместе с `match_id`, и повторный тап по уже
 * сматченному человеку отвечает так же — по нему и ветвиться.
 */
export interface LikeResponse {
  matched: boolean;
  match_id?: string;
  already: boolean;
  budget: LikeBudget;
}

/** Карточка входящего лайка (§6 «Тебя лайкнули»). Контактов тут тоже нет. */
export interface IncomingLikeDTO {
  user_id: string;
  name: string;
  age: number;
  photo_url: string;
  schedule: NonNullable<Questionnaire['schedule']>;
  smoking: NonNullable<Questionnaire['smoking']>;
  pets_own: NonNullable<Questionnaire['pets_own']>;

  zone?: ZoneRef;
  /**
   * Контекст-точка лайка. У «к себе» сервер уже написал метку от лица
   * адресата — «на твою комнату · м. Сокол»: автор точки и есть тот, кто
   * читает этот список (§2.2).
   */
  point?: PointRef;
  /** «сейчас не в поиске» — серая пометка, а не причина спрятать лайк (§2.3). */
  not_searching: boolean;
  at: string;
}

export interface MatchPeer {
  user_id: string;
  name: string;
  age: number;
  photo_url: string;
}

/**
 * Единственное место продукта, где есть способ связаться (§3).
 *
 * У забаненного или заблокированного пира оба поля пустые — сервер их снимает,
 * и звонить экрану некуда.
 */
export interface MatchPeerContact extends MatchPeer {
  username?: string;
  tg_id: number;
}

export interface MatchDTO {
  match_id: string;
  peer: MatchPeer;
  zone?: ZoneRef;
  at: string;
}

/**
 * Пересечение одного параметра поисков.
 *
 * У цены это вилка (`min`/`max`), у наборов — общие значения (`values`):
 * комнаты числами-строками, месяцы «2026-09», сроки — коды Term.
 */
export interface ParamOverlap {
  param: 'price' | 'rooms' | 'move_in' | 'term';
  min?: number;
  max?: number;
  values?: string[];
}

/** «Это мэтч» тремя блоками (§6): где, по параметрам, по анкете. */
export interface MatchDetail {
  match_id: string;
  peer: MatchPeerContact;
  at: string;
  where?: ZoneRef;
  /** Тот же блок ГДЕ, когда пересеклись на точке. С `where` взаимоисключим. */
  where_point?: PointRef;
  params: ParamOverlap[];
  profile: SharedAnswer[];
}

/** Конверт, которым отвечает любая ошибка API. */
export interface ApiErrorBody {
  error: { code: string; message: string; fields?: string[] };
}

/**
 * Неудачный вызов API. `code` — стабильный идентификатор, по которому ветвимся;
 * `status` — HTTP-статус, он отличает «не вошёл» от «сервер лежит»;
 * `fields` — какие именно поля не понравились или каких не хватает.
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly fields: string[];

  constructor(status: number, code: string, message: string, fields: string[] = []) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

/**
 * Базовый адрес API.
 *
 * Пустой по умолчанию, поэтому все вызовы same-origin: в проде Caddy отдаёт
 * фронт и проксирует /api, в разработке то же делает Vite (vite.config.ts).
 * VITE_API_BASE_URL задавать только если API реально живёт в другом месте.
 */
const BASE_URL: string = import.meta.env.VITE_API_BASE_URL ?? '';

/**
 * Вызывает API от имени текущего пользователя Telegram.
 *
 * Аутентификация — сырая строка initData, которую выдал Telegram, заголовком
 * `Authorization: tma <initData>`. Хранить нечего и обновлять нечего:
 * credential перечитывается на каждом вызове.
 */
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await call(path, init);
  return (await response.json()) as T;
}

/** Вызов, который отвечает 204: тела нет, и разбирать его нечем. */
async function requestNoContent(path: string, init?: RequestInit): Promise<void> {
  await call(path, init);
}

async function call(path: string, init?: RequestInit): Promise<Response> {
  const initData = retrieveRawInitData();
  if (!initData) {
    throw new ApiError(0, 'no_init_data', 'Приложение открыто не из Telegram');
  }

  const response = await fetch(`${BASE_URL}${path}`, {
    ...init,
    headers: {
      ...init?.headers,
      Authorization: `tma ${initData}`,
    },
  });

  if (!response.ok) {
    throw await readError(response);
  }
  return response;
}

/**
 * Достаёт код и сообщение из неудачного ответа, переживая тело, которое вообще
 * не наш конверт — например, HTML от прокси.
 */
async function readError(response: Response): Promise<ApiError> {
  try {
    const body = (await response.json()) as Partial<ApiErrorBody>;
    if (body.error?.code) {
      const message = body.error.message ?? response.statusText;
      return new ApiError(response.status, body.error.code, message, body.error.fields ?? []);
    }
  } catch {
    // Тело нечитаемое — падаем в общее сообщение ниже.
  }
  return new ApiError(response.status, 'unknown', `HTTP ${response.status} ${response.statusText}`);
}

/** Профиль вызывающего. Заодно — единственное место, где узнаётся имя бота. */
export async function fetchMe(): Promise<Profile> {
  const me = await request<Profile>('/api/v1/me');
  if (me.bot_username) botUsername = me.bot_username;
  return me;
}

/**
 * Сохраняет ответы, которые несёт запрос, и не трогает остальные.
 *
 * Тот же вызов обслуживает и двенадцать экранов онбординга, и правки из
 * профиля: различаются они набором полей, а не эндпоинтом.
 */
export function patchProfile(patch: Questionnaire & { onboarding_step?: number }): Promise<Profile> {
  return request<Profile>('/api/v1/me/profile', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  });
}

/**
 * Загружает фото анкеты.
 *
 * Content-Type не выставляем намеренно: его должен проставить браузер, вписав
 * boundary multipart-формы. Заданный руками заголовок сломает разбор на сервере.
 */
export function uploadPhoto(file: File): Promise<Profile> {
  const form = new FormData();
  form.append('photo', file);
  return request<Profile>('/api/v1/me/photo', { method: 'POST', body: form });
}

/**
 * Отправляет заполненную анкету модераторам: draft → pending.
 *
 * На неполной анкете отвечает 400 с кодом `profile_incomplete`, и в
 * `fields` — чего не хватает.
 */
export function submitProfile(): Promise<Profile> {
  return request<Profile>('/api/v1/me/submit', { method: 'POST' });
}

/**
 * Откуда человек пришёл: код из `startapp=src_<код>` (§13, посев).
 *
 * Сервер запишет его, только если источника ещё нет: побеждает ПЕРВОЕ касание,
 * поэтому звать можно на каждом запуске — повтор с другим кодом он молча
 * игнорирует. Отказ ничего не ломает: трекинг посева не гейт на входе.
 */
export function postSource(source: string): Promise<void> {
  return requestNoContent('/api/v1/me/source', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ source }),
  });
}

/**
 * Кэш справочника зон.
 *
 * Хранится именно промис, а не разобранный ответ: два параллельных вызова
 * (пикер открылся раньше, чем догрузился фон) иначе ушли бы двумя запросами
 * по 210 КБ. При отказе кэш сбрасывается — запомненная навсегда ошибка
 * оставила бы пикер пустым до перезапуска приложения.
 */
let zonesCache: Promise<ZoneDirectory> | null = null;

/**
 * Топ живых зон для пустой ленты: «Где уже ищут» (§6).
 *
 * Не кэшируется в отличие от справочника: это счётчики людей, а не словарь, и
 * показываются они ровно там, где своя зона оказалась пустой.
 */
export function fetchTopZones(limit = 5): Promise<TopZoneDTO[]> {
  return request<{ zones: TopZoneDTO[] }>(`/api/v1/zones/top?limit=${limit}`).then((r) => r.zones);
}

/** Справочник зон целиком. Второй вызов запроса не делает. */
export function fetchZones(): Promise<ZoneDirectory> {
  if (!zonesCache) {
    zonesCache = request<ZoneDirectory>('/api/v1/zones').catch((e: unknown) => {
      zonesCache = null;
      throw e;
    });
  }
  return zonesCache;
}

/**
 * Колода подбора по всем активным поискам смотрящего (§2.1).
 *
 * Наборы едут одним ключом через запятую (`pets=none,ok_mine`), пустые —
 * не едут вовсе: отсутствующий параметр сервер считает «неважно», а
 * `pets=` разобрал бы как неизвестное значение.
 */
export function fetchDeck(f: DeckFilters): Promise<DeckResponse> {
  const q = new URLSearchParams();
  const list = (name: string, values: string[]) => {
    if (values.length > 0) q.set(name, values.join(','));
  };
  list('who', f.who);
  if (f.age_min != null) q.set('age_min', String(f.age_min));
  if (f.age_max != null) q.set('age_max', String(f.age_max));
  list('pets', f.pets);
  list('smoking', f.smoking);
  list('schedule', f.schedule);
  list('occupation', f.occupation);
  const query = q.toString();
  return request<DeckResponse>(`/api/v1/deck${query ? `?${query}` : ''}`);
}

/** Пропуск человека — свайп влево: уходит из подбора на 7 дней (§2.1). */
export function passPerson(id: string): Promise<void> {
  return requestNoContent(`/api/v1/people/${encodeURIComponent(id)}/pass`, { method: 'POST' });
}

/** Пропуск комнаты. Тот же жест и тот же срок, что у человека. */
export function passPoint(id: string): Promise<void> {
  return requestNoContent(`/api/v1/points/${encodeURIComponent(id)}/pass`, { method: 'POST' });
}

/** «Показать пропущенных»: стирает все пропуски разом, отвечает их числом. */
export function clearPasses(): Promise<number> {
  return request<{ restored: number }>('/api/v1/me/passes', { method: 'DELETE' }).then(
    (r) => r.restored,
  );
}

/**
 * Все поиски человека: активные первыми, внутри — новые первыми.
 *
 * Единственный источник счётчиков и номеров слотов. Локально список не
 * «подправляем»: после любой мутации перечитываем его целиком.
 */
export function fetchSearches(): Promise<SearchResponse[]> {
  return request<SearchResponse[]>('/api/v1/searches');
}

/** Подписка на зону. При занятых слотах — 400 `search_limit`. */
export function createSearch(p: SearchParams): Promise<SearchResponse> {
  return request<SearchResponse>('/api/v1/searches', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: paramsBody(p),
  });
}

/**
 * Правка параметров. Заменяет набор ЦЕЛИКОМ: очищенное поле становится NULL.
 *
 * На закрытом поиске отвечает 400 `search_closed` — сначала реактивация.
 */
export function updateSearch(id: string, p: SearchParams): Promise<SearchResponse> {
  return request<SearchResponse>(`/api/v1/searches/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: paramsBody(p),
  });
}

/** Закрытие с причиной. «нашёл здесь» — ГЛАВНАЯ МЕТРИКА (§3). */
export function closeSearch(id: string, reason: CloseReason): Promise<SearchResponse> {
  return request<SearchResponse>(`/api/v1/searches/${encodeURIComponent(id)}/close`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason }),
  });
}

/** Возврат из архива. Перепроверяет слоты — может ответить 400 `search_limit`. */
export function reactivateSearch(id: string): Promise<SearchResponse> {
  return request<SearchResponse>(`/api/v1/searches/${encodeURIComponent(id)}/reactivate`, {
    method: 'POST',
  });
}

/**
 * Создаёт комнату «к себе» — единственную точку, которую заводят руками (§2.2).
 *
 * Отказы приходят кодами: 400 `invalid_field` — в `fields` ровно те поля,
 * которые не понравились; 400 `point_limit` — слоты заняты (сообщение сервера
 * уже названо по типу формы).
 */
export function postPoint(body: CreatePointBody): Promise<PointDTO> {
  return request<PointDTO>('/api/v1/points', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

/**
 * Загружает фото точки — скрин объявления или фото комнаты.
 *
 * НЕ `/me/photo`: тот аплоад заменяет лицо в анкете и сбрасывает его на
 * перепроверку (§5), то есть скрин Авито стёр бы человеку фото профиля.
 * Content-Type не выставляем — boundary проставит браузер.
 */
export function uploadPointPhoto(file: File): Promise<{ photo_key: string }> {
  const form = new FormData();
  form.append('photo', file);
  return request<{ photo_key: string }>('/api/v1/points/photo', { method: 'POST', body: form });
}

/** Карточка точки. Точка на проверке видна только создателю, остальным 404 (§5). */
export function fetchPoint(id: string): Promise<PointDTO> {
  return request<PointDTO>(`/api/v1/points/${encodeURIComponent(id)}`);
}

/** Блоки 2–3 «Моих поисков»: свои точки и чужие пулы, в которые вошёл. */
export function fetchPointsMine(): Promise<MyPointsResponse> {
  return request<MyPointsResponse>('/api/v1/points/mine');
}

/**
 * Закрывает свою точку. Причин у точки нет — в отличие от поиска, где
 * «нашёл здесь» и есть метрика продукта (§3).
 */
export function closePoint(id: string): Promise<void> {
  return requestNoContent(`/api/v1/points/${encodeURIComponent(id)}/close`, { method: 'POST' });
}

/**
 * Бот, в котором живёт Mini App.
 *
 * Имя приезжает с сервера (`bot_username` в /me — оно же getMe у бота): ссылка
 * «Позвать в пул» обязана вести в тот же бот, из которого приложение открыли,
 * а бот на стенде и на бою разный. Константа — запас на случай, когда сервер
 * имени не знает: при выключенном поллинге спрашивать его не у кого.
 */
const BOT_USERNAME_FALLBACK = 'flatmatch_finder_bot';

let botUsername = '';

/* Словарь startapp: его пишут кнопки пушей и ссылка шеринга, читает приложение.
   Значения общие с ботом (§2 дизайна пушей) — менять их можно только вместе. */
const POINT_START_PARAM = 'point_';
const POOL_START_PARAM = 'pool_';
const MATCH_START_PARAM = 'match_';
const SOURCE_START_PARAM = 'src_';

/**
 * Код источника посева: `?startapp=src_hsetndr`.
 *
 * Тот же набор, что проверяет сервер, — иначе кривая ссылка из чужого поста
 * стоила бы приложению 400 на входе. Не подошёл — цели просто нет, вход
 * обычный: трекинг не повод не пустить человека внутрь.
 */
const SOURCE_CODE = /^[a-z0-9_]{1,32}$/;

/** Куда открыть приложение по диплинку. */
export type StartTarget =
  | { kind: 'point'; id: string }
  | { kind: 'pool'; zoneId: string }
  | { kind: 'match'; id: string }
  | { kind: 'incoming' }
  | { kind: 'feed' }
  | { kind: 'photo' }
  | { kind: 'about' }
  | { kind: 'mine' }
  /** Ссылка посева: никуда не ведёт, только помечает первое касание. */
  | { kind: 'source'; code: string };

/**
 * Ссылка «Позвать в пул — поделиться в чат»: открывает у гостя карточку точки.
 *
 * Форма `t.me/{бот}?startapp=` — главный Mini App бота (Configure Mini App в
 * BotFather): ему, в отличие от `t.me/{бот}/{short_name}`, не нужно имя
 * приложения, которого у нас нигде нет.
 *
 * Два уровня кодирования не случайны: внутренний адрес целиком становится
 * значением параметра `url` шеринга, и без encodeURIComponent его `?startapp=`
 * оторвался бы от ссылки.
 */
export function pointShareUrl(pointId: string): string {
  const bot = botUsername || BOT_USERNAME_FALLBACK;
  const start = `${POINT_START_PARAM}${pointId}`;
  const app = `https://t.me/${bot}?startapp=${start}`;
  return `https://t.me/share/url?url=${encodeURIComponent(app)}`;
}

/** Обратная сторона диплинка: куда вести запуск, иначе null. */
export function startParam(): StartTarget | null {
  return parseStartParam(rawStartParam());
}

/**
 * Сырой startapp запуска.
 *
 * Источника два, и порядок между ними не случаен: web_app-кнопка пуша открывает
 * приложение адресом `APP_URL?startapp=…`, и параметр приезжает в query самой
 * страницы; ссылка шеринга `t.me/{бот}?startapp=` — наоборот, только параметром
 * запуска. Query первым: у кнопки пуша параметр точнее, а параметры запуска
 * держатся у клиента и после перехода.
 *
 * retrieveLaunchParams умеет бросить — в браузере без мока их просто нет, и
 * приложение обязано открыться обычным входом, а не упасть.
 */
function rawStartParam(): string | undefined {
  const fromQuery = new URLSearchParams(window.location.search).get('startapp');
  if (fromQuery) return fromQuery;
  try {
    return retrieveLaunchParams().tgWebAppStartParam;
  } catch {
    return undefined;
  }
}

function parseStartParam(raw?: string): StartTarget | null {
  if (!raw) return null;
  if (raw.startsWith(POINT_START_PARAM)) {
    const id = raw.slice(POINT_START_PARAM.length);
    return id ? { kind: 'point', id } : null;
  }
  if (raw.startsWith(POOL_START_PARAM)) {
    const zoneId = raw.slice(POOL_START_PARAM.length);
    return zoneId ? { kind: 'pool', zoneId } : null;
  }
  if (raw.startsWith(MATCH_START_PARAM)) {
    const id = raw.slice(MATCH_START_PARAM.length);
    return id ? { kind: 'match', id } : null;
  }
  if (raw.startsWith(SOURCE_START_PARAM)) {
    const code = raw.slice(SOURCE_START_PARAM.length);
    return SOURCE_CODE.test(code) ? { kind: 'source', code } : null;
  }
  switch (raw) {
    case 'incoming':
      return { kind: 'incoming' };
    case 'feed':
      return { kind: 'feed' };
    case 'photo':
      return { kind: 'photo' };
    case 'about':
      return { kind: 'about' };
    case 'mine':
      return { kind: 'mine' };
    default:
      return null;
  }
}

/**
 * Профиль человека и состояние лайка между вами.
 *
 * `zone` — пул, из которого профиль открыт: он же станет контекстом лайка.
 * В отличие от пула, этот GET ничего не мутирует — звать можно свободно.
 * Невидимый по правилу пола или заблокировавший отвечает 404 `not_found`:
 * невидимый и несуществующий — один ответ (§4).
 */
export function fetchPerson(id: string, ctx?: MeetingContext): Promise<PersonProfileDTO> {
  const q = new URLSearchParams();
  if (ctx?.zoneId) q.set('zone', ctx.zoneId);
  if (ctx?.pointId) q.set('point', ctx.pointId);
  const query = q.toString();
  return request<PersonProfileDTO>(
    `/api/v1/people/${encodeURIComponent(id)}${query ? `?${query}` : ''}`,
  );
}

/** Причины жалобы — коды провода; русские чипы собирает api/format.ts. */
export type ReportReason = 'spam' | 'scam' | 'abuse' | 'other';

/**
 * Откуда пожаловались: пул, точка или мэтч — модератору это половина разбора.
 *
 * Все три взаимоисключимы и все три необязательны: жалоба из места, которого
 * экран не знает, уходит без контекста, а не с выдуманным.
 */
export interface ReportContext {
  zoneId?: string;
  pointId?: string;
  matchId?: string;
}

/**
 * Блокировка (§5): невидимость обоюдная и немедленная.
 *
 * Разблокировки нет ни в API, ни в UI. После вызова профиль цели отвечает тем
 * же 404, что и невидимый по полу, — экрану остаётся уйти назад, а не
 * показывать отказ.
 */
export function blockPerson(id: string): Promise<void> {
  return requestNoContent(`/api/v1/people/${encodeURIComponent(id)}/block`, { method: 'POST' });
}

/**
 * Жалоба модератору (§5). Повтор той же пары — тот же 204 без новой карточки.
 *
 * Блокировки за собой НЕ несёт: заблокировать предлагается отдельной кнопкой
 * после отправки — иногда хватает разбора, а иногда нужно и то и другое.
 */
export function reportPerson(id: string, reason: ReportReason, ctx?: ReportContext): Promise<void> {
  const context = ctx?.zoneId
    ? { zone_id: ctx.zoneId }
    : ctx?.pointId
      ? { point_id: ctx.pointId }
      : ctx?.matchId
        ? { match_id: ctx.matchId }
        : undefined;
  return requestNoContent(`/api/v1/people/${encodeURIComponent(id)}/report`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(context ? { reason, context } : { reason }),
  });
}

/**
 * Дорогой лайк «Хочу жить вместе» (§2.3).
 *
 * Отказы приходят кодами: 400 `need_search` — нет активного поиска (мини-форма),
 * 400 `like_limit` — израсходованы 5 в сутки (сообщение сервера уже называет
 * час возврата), 403 `not_approved` — анкета не одобрена. Ровно три ключа в
 * теле: декодер сервера с DisallowUnknownFields откажет на любом лишнем.
 * Пустой контекст легален — у ответного лайка пула нет; точечному лайку
 * активный поиск не нужен вовсе, отношение несёт сам пул или карточка (§2.2).
 */
export function postLike(toId: string, ctx?: MeetingContext): Promise<LikeResponse> {
  return request<LikeResponse>('/api/v1/likes', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      to_id: toId,
      zone_id: ctx?.zoneId ?? '',
      point_id: ctx?.pointId ?? '',
    }),
  });
}

/** «Тебя лайкнули»: входящие без ответа, новые первыми. */
export function fetchIncoming(): Promise<IncomingLikeDTO[]> {
  return request<IncomingLikeDTO[]>('/api/v1/likes/incoming');
}

/** Мои мэтчи списком. Контакты — только в карточке одного (§3). */
export function fetchMatches(): Promise<MatchDTO[]> {
  return request<MatchDTO[]>('/api/v1/matches');
}

/** Один мэтч с саммари «почему совпали» и контактом. */
export function fetchMatch(id: string): Promise<MatchDetail> {
  return request<MatchDetail>(`/api/v1/matches/${encodeURIComponent(id)}`);
}

/**
 * Удаление мэтча любой стороной (§2.3).
 *
 * Стирает мэтч и ОБА лайка пары: люди снова видят друг друга в пулах, а
 * потраченные слоты не возвращаются. Второй стороне не уходит ничего — ни
 * пуша, ни следа. Повторный вызов отвечает 404: удалять уже нечего.
 */
export function deleteMatch(id: string): Promise<void> {
  return requestNoContent(`/api/v1/matches/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

/**
 * Тело POST/PATCH поиска.
 *
 * Ровно семь ключей и ни одного лишнего: декодер сервера с
 * DisallowUnknownFields ответит 400 на любой `id`, `status` или `zone`,
 * случайно доехавший из ответа обратно в запрос. Пустая цена едет как null,
 * пустой набор — как `[]`: обе формы означают «неважно».
 */
function paramsBody(p: SearchParams): string {
  return JSON.stringify({
    zone_id: p.zone_id,
    price_min: p.price_min ?? null,
    price_max: p.price_max,
    rooms: p.rooms,
    terms: p.terms,
    move_in: p.move_in,
    has_stuff: p.has_stuff ?? [],
  });
}
