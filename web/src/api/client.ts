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
