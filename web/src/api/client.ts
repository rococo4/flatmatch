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
 * Обязательны только зона (граница пула) и «до» по цене; всё остальное
 * отсутствующее или null означает «неважно» и совпадает со всем.
 */
export interface SearchParams {
  zone_id: string;
  price_min?: number | null;
  price_max: number;
  rooms_min?: number | null;
  rooms_max?: number | null;
  /** Месяц заезда, «2026-09». Дня в модели нет — обещать его нельзя. */
  move_in?: string | null;
  term?: Term | null;
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

/**
 * Розовый промо-блок ленты (§6) — единственное место, где сохраняется поиск.
 *
 * `avatars` — URL фотографий (до 4, вида `/media/<key>`), а НЕ имена: в стек
 * попадают только люди с одобренным фото, поэтому их бывает меньше четырёх
 * даже при большом `people`.
 * `my_search_id` есть ⇔ у смотрящего активный поиск ровно этой зоны.
 */
export interface FeedPromo {
  zone: ZoneRef;
  people: number;
  full_match: number;
  avatars: string[];
  my_search_id?: string;
  slots_used: number;
  slots_total: number;
}

export interface FeedResponse {
  promo: FeedPromo;
  /** Точки зоны выдачи, оба типа вперемешку: ранжирование считает сервер (§8). */
  points: PointDTO[];
}

/**
 * Вердикт параметра против поиска смотрящего.
 *
 * На проводе четыре значения, в UI три: `grey` и `none` рисуются одним серым
 * цветом, но разным текстом — `grey` показывает значение параметра, `none`
 * говорит «не указана» (§6). Схлопывание живёт в api/format.ts.
 */
export interface PoolChip {
  param: 'price' | 'rooms' | 'move_in' | 'term';
  verdict: 'green' | 'edge' | 'grey' | 'none';
}

/** Человек в зонном пуле. Метки чипов собирает клиент из `params`. */
export interface PoolPersonDTO {
  user_id: string;
  name: string;
  age: number;
  /** Пустая строка = фото не одобрено: вместо него заглушка с инициалами (§5). */
  photo_url: string;
  /** Три поля анкеты для строки привычек — те же значения, что в Questionnaire. */
  schedule: NonNullable<Questionnaire['schedule']>;
  smoking: NonNullable<Questionnaire['smoking']>;
  pets_own: NonNullable<Questionnaire['pets_own']>;
  shared_answers: number;
  /** Нет вовсе в unranked-пуле: сравнивать не с чем. */
  chips?: PoolChip[];
  params: SearchParamsNoZone;
  /** Зона его поиска — «где именно он ищет». */
  zone: ZoneRef;
  /** Полная территориальная совместимость с поиском смотрящего. */
  zone_match: boolean;
}

export interface PoolSectionDTO {
  id: 'all' | 'partial' | 'none';
  people: PoolPersonDTO[];
}

/**
 * Зонный пул. Пул виртуален: жёсткая граница — только зона (§2.1).
 *
 * `sections` и `people` всегда массивы; какой из них несёт пул — говорит
 * `ranked`, второй пуст. `my_search_id` — отдельный флаг: ранжирование
 * довольствуется поиском соседней пересекающейся зоны, поэтому `ranked`
 * бывает true без подписки именно сюда, и плашка «Ты в пуле» тогда соврала бы.
 */
export interface PoolResponse {
  zone: ZoneRef;
  my_search_id?: string;
  ranked: boolean;
  sections: PoolSectionDTO[];
  people: PoolPersonDTO[];
}

/* ── Точки: квартиры с рынка и «сосед к себе» (§2.2, §6) ────────────────────
   Словарные значения едут кодами — русские метки разные у двух типов карточек
   («без животных» на квартире, «нельзя с животными» у «к себе»), и собирает их
   api/format.ts. */

/** Тип точки решает всё: у `flat` пул и дешёвый вход, у `intent_home` — отклики. */
export type PointType = 'flat' | 'intent_home';

/** Стадия «к себе»: уже живёт или заезжает. У квартиры с рынка стадии нет. */
export type PointStage = 'live' | 'moving_in';

/** unknown — значение уровня кода, как у Profile.status. */
export type PointStatus = 'unknown' | 'pending' | 'active' | 'paused' | 'closed' | 'rejected';

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
  pool_count: number;
  responses: number;
  in_pool: boolean;

  author?: PointAuthorDTO;
}

/** Тело формы «Найди, с кем снять эту квартиру» (§6). */
export interface CreateFlatPoint {
  type: 'flat';
  url: string;
  zone_id: string;
  price_total: number;
  rooms?: number | null;
  isolated?: boolean | null;
  bathrooms?: Bathrooms | null;
  utilities?: Utilities | null;
  restrictions?: Restriction[];
  /** До 10; первый ключ — обложка. */
  photo_keys: string[];
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

export type CreatePointBody = CreateFlatPoint | CreateHomePoint;

export interface MyPointsResponse {
  /** Мои точки во всех статусах, кроме закрытых: они и считаются в лимит. */
  created: PointDTO[];
  /** Чужие квартиры, в чьи пулы я вошёл. Лимита у них нет. */
  entered: PointDTO[];
}

/**
 * Участник пула квартиры (§6): совпадения анкеты и вердикт-чипы.
 *
 * Поиска у него может не быть вовсе — вход в пул его не требует (§2.2), — и
 * тогда не приезжают ни `params`, ни `chips`, ни `zone`. Чипов нет и когда
 * поиска нет у СМОТРЯЩЕГО: сравнивать не с чем.
 */
export interface PointPoolMemberDTO {
  user_id: string;
  name: string;
  age: number;
  /** Пустая строка = фото не одобрено: вместо него заглушка с инициалами (§5). */
  photo_url: string;
  schedule: NonNullable<Questionnaire['schedule']>;
  smoking: NonNullable<Questionnaire['smoking']>;
  pets_own: NonNullable<Questionnaire['pets_own']>;
  shared_answers: number;
  chips?: PoolChip[];
  params?: SearchParamsNoZone;
  /** Зона его поиска — «ищет на красной ветке». */
  zone?: ZoneRef;
  entered_at: string;
  /** Вы уже в мэтче: карточка остаётся (пошли смотреть вместе), лайкать нечего. */
  matched?: boolean;
}

/**
 * Пул квартиры — только список вошедших.
 *
 * Шапка-карточка сюда не едет: её отдаёт `GET /points/{id}`, и дублировать
 * двадцать полей во втором ответе незачем.
 */
export interface PointPoolResponse {
  members: PointPoolMemberDTO[];
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

/** Пересечение одного параметра поисков: вилка, месяц или срок — что уж есть. */
export interface ParamOverlap {
  param: 'price' | 'rooms' | 'move_in' | 'term';
  min?: number;
  max?: number;
  month?: string;
  term?: Term;
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

/** 409 на ссылку, которая в продукте уже есть: всё для кнопки «Войти» — в теле. */
interface DuplicateBody extends ApiErrorBody {
  point_id: string;
  pool_count: number;
  status: PointStatus;
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
 * Ссылку уже приносили: 409 `duplicate_url`.
 *
 * Отдельный класс, потому что форме мало текста — ей нужна сама точка: у
 * активной есть пул, в который зовут войти, а точка на проверке ещё ничья, и
 * входить в неё некуда (§5). Ветвиться по `pointStatus`, не по сообщению.
 */
export class DuplicatePointError extends ApiError {
  readonly pointId: string;
  readonly poolCount: number;
  readonly pointStatus: PointStatus;

  constructor(status: number, message: string, body: DuplicateBody) {
    super(status, body.error.code, message);
    this.name = 'DuplicatePointError';
    this.pointId = body.point_id;
    this.poolCount = body.pool_count;
    this.pointStatus = body.status;
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
    const body = (await response.json()) as Partial<DuplicateBody>;
    if (body.error?.code) {
      const message = body.error.message ?? response.statusText;
      if (body.error.code === 'duplicate_url' && body.point_id) {
        return new DuplicatePointError(response.status, message, body as DuplicateBody);
      }
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
 * Кэш справочника зон.
 *
 * Хранится именно промис, а не разобранный ответ: два параллельных вызова
 * (пикер открылся раньше, чем догрузился фон) иначе ушли бы двумя запросами
 * по 210 КБ. При отказе кэш сбрасывается — запомненная навсегда ошибка
 * оставила бы пикер пустым до перезапуска приложения.
 */
let zonesCache: Promise<ZoneDirectory> | null = null;

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
 * Выдача по параметрам просмотра (§6 шаг 3).
 *
 * Смотреть — не подписываться: этот вызов ничего не сохраняет, поиск создаёт
 * только кнопка внутри промо-блока. Пустые ключи опускаются — сервер считает
 * отсутствующий параметр «неважно», а `rooms_min=` разобрал бы как ошибку.
 * `has_stuff` не отправляется вовсе: сервер их из query не читает.
 */
export function fetchFeed(p: SearchParams): Promise<FeedResponse> {
  const q = new URLSearchParams({ zone_id: p.zone_id, price_max: String(p.price_max) });
  if (p.price_min != null) q.set('price_min', String(p.price_min));
  if (p.rooms_min != null) q.set('rooms_min', String(p.rooms_min));
  if (p.rooms_max != null) q.set('rooms_max', String(p.rooms_max));
  if (p.move_in) q.set('move_in', p.move_in);
  if (p.term) q.set('term', p.term);
  return request<FeedResponse>(`/api/v1/feed?${q.toString()}`);
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
 * Зонный пул.
 *
 * ПОБОЧНЫЙ ЭФФЕКТ: сдвигает `pool_seen_at` активного поиска смотрящего в этой
 * зоне, то есть гасит бейдж «M новых». Звать только при явном переходе на
 * экран пула — никаких префетчей и refetch-on-focus.
 */
export function fetchZonePool(zoneId: string): Promise<PoolResponse> {
  return request<PoolResponse>(`/api/v1/pools/zone/${encodeURIComponent(zoneId)}`);
}

/**
 * Создаёт точку: обе формы §6 ходят одним вызовом, тип решает тело.
 *
 * Отказы приходят кодами: 400 `invalid_field` — в `fields` ровно те поля,
 * которые не понравились; 400 `point_limit` — слоты заняты (сообщение сервера
 * уже названо по типу формы); 409 `duplicate_url` — ссылку уже приносили,
 * и тогда это DuplicatePointError с самой точкой.
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
 * Список вошедших в пул квартиры.
 *
 * Не вошедшему отвечает 403 `need_enter` — это не поломка, а состояние экрана:
 * карточка видна всем, список — только тем, кто в пуле (§6).
 */
export function fetchPointPool(id: string): Promise<PointPoolResponse> {
  return request<PointPoolResponse>(`/api/v1/points/${encodeURIComponent(id)}/pool`);
}

/**
 * Закрывает свою точку. Причин у точки нет — в отличие от поиска, где
 * «нашёл здесь» и есть метрика продукта (§3).
 */
export function closePoint(id: string): Promise<void> {
  return requestNoContent(`/api/v1/points/${encodeURIComponent(id)}/close`, { method: 'POST' });
}

/** «Интересно — войти в пул»: бесплатно, безлимитно, повторный тап не ошибка. */
export function enterPointPool(id: string): Promise<void> {
  return requestNoContent(`/api/v1/points/${encodeURIComponent(id)}/enter`, { method: 'POST' });
}

/** «Выйти из пула». Дорогие лайки при этом не сгорают (§2.3) — говорить о них нечего. */
export function leavePointPool(id: string): Promise<void> {
  return requestNoContent(`/api/v1/points/${encodeURIComponent(id)}/enter`, { method: 'DELETE' });
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

/** Куда открыть приложение по диплинку. */
export type StartTarget =
  | { kind: 'point'; id: string }
  | { kind: 'pool'; zoneId: string }
  | { kind: 'match'; id: string }
  | { kind: 'incoming' }
  | { kind: 'feed' }
  | { kind: 'photo' }
  | { kind: 'about' }
  | { kind: 'mine' };

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
 * Тело POST/PATCH поиска.
 *
 * Ровно восемь ключей и ни одного лишнего: декодер сервера с
 * DisallowUnknownFields ответит 400 на любой `id`, `status` или `zone`,
 * случайно доехавший из ответа обратно в запрос. Отсутствующее едет как null —
 * документированная форма «неважно».
 */
function paramsBody(p: SearchParams): string {
  return JSON.stringify({
    zone_id: p.zone_id,
    price_min: p.price_min ?? null,
    price_max: p.price_max,
    rooms_min: p.rooms_min ?? null,
    rooms_max: p.rooms_max ?? null,
    move_in: p.move_in ?? null,
    term: p.term ?? null,
    has_stuff: p.has_stuff ?? [],
  });
}
