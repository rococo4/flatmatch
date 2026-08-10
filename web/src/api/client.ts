import { retrieveRawInitData } from '@telegram-apps/sdk-react';

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
  /** Всегда 0 до §12 шага 5: точек ещё нет. */
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
  /** Всегда [] до §12 шага 5. Пустой список — пустое состояние, не DEMO-квартира. */
  points: unknown[];
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
  return (await response.json()) as T;
}

/**
 * Достаёт код и сообщение из неудачного ответа, переживая тело, которое вообще
 * не наш конверт — например, HTML от прокси.
 */
async function readError(response: Response): Promise<ApiError> {
  try {
    const body = (await response.json()) as Partial<ApiErrorBody>;
    if (body.error?.code) {
      return new ApiError(
        response.status,
        body.error.code,
        body.error.message ?? response.statusText,
        body.error.fields ?? [],
      );
    }
  } catch {
    // Тело нечитаемое — падаем в общее сообщение ниже.
  }
  return new ApiError(response.status, 'unknown', `HTTP ${response.status} ${response.statusText}`);
}

/** Профиль вызывающего. */
export function fetchMe(): Promise<Profile> {
  return request<Profile>('/api/v1/me');
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
