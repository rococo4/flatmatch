import { retrieveRawInitData } from '@telegram-apps/sdk-react';

/** A FlatMatch account, as `GET /api/v1/me` returns it. */
export interface Profile {
  id: string;
  tg_id: number;
  name: string;
  tg_username?: string;
  photo_url?: string;
  /**
   * draft — questionnaire not finished, which a rejected photo also returns
   * you to, since the photo is mandatory (§4); pending — submitted, waiting
   * for a moderator; active — approved and visible; banned (§5).
   *
   * unknown means the API read a value it does not recognise. It should never
   * appear, and is listed so that switching on status stays exhaustive.
   */
  status: 'unknown' | 'draft' | 'pending' | 'active' | 'banned';
  photo_status: 'unknown' | 'pending' | 'ok' | 'rejected';
  created_at: string;
}

/** The error envelope every API failure uses. */
export interface ApiErrorBody {
  error: { code: string; message: string };
}

/**
 * An API call that failed. `code` is the stable identifier to branch on;
 * `status` is the HTTP status, useful for telling "not signed in" apart from
 * "server is down".
 */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

/**
 * Base URL of the API.
 *
 * Empty by default, which makes every call same-origin: in production Caddy
 * serves the front-end and proxies /api to the backend, and in development
 * Vite proxies it (see vite.config.ts). Set VITE_API_BASE_URL only when the
 * API genuinely lives somewhere else.
 */
const BASE_URL: string = import.meta.env.VITE_API_BASE_URL ?? '';

/**
 * Calls the API on behalf of the current Telegram user.
 *
 * Authentication is the raw initData string Telegram handed the app, sent as
 * `Authorization: tma <initData>`. There is no token to store and no session
 * to refresh — the credential is re-read on every call, so it stays as fresh
 * as the client makes it.
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
    throw new ApiError(response.status, ...(await readError(response)));
  }
  return (await response.json()) as T;
}

/**
 * Pulls the code and message out of a failed response, tolerating a body that
 * is not our envelope at all — a proxy returning HTML, for instance.
 */
async function readError(response: Response): Promise<[string, string]> {
  try {
    const body = (await response.json()) as Partial<ApiErrorBody>;
    if (body.error?.code) {
      return [body.error.code, body.error.message ?? response.statusText];
    }
  } catch {
    // fall through to the generic message
  }
  return ['unknown', `HTTP ${response.status} ${response.statusText}`];
}

/** Fetches the caller's own profile. */
export function fetchMe(): Promise<Profile> {
  return request<Profile>('/api/v1/me');
}
