import { mockTelegramEnv } from '@telegram-apps/sdk-react';

/**
 * Pretends to be a Telegram client so the app can be developed in an ordinary
 * browser tab, with devtools and hot reload, instead of only inside Telegram.
 *
 * The API still checks the HMAC on every request, so the mock has to carry
 * genuinely signed initData to get past `GET /api/v1/me`. Produce some with
 * the bot token and paste it into `web/.env.local`:
 *
 *   cd api && go run ./cmd/devinitdata
 *   # web/.env.local
 *   VITE_MOCK_INIT_DATA=user=%7B%22id%22%3A777000001...&hash=...
 *
 * Without it the screen still renders — it shows the 401 and says what to do,
 * which is itself the error path worth being able to see.
 */
export function mockTelegramEnvIfNeeded(): void {
  // Never mock inside a real client: the launch data is already there, and
  // wrapping postMessage would only get in the way.
  if (launchedByTelegram()) {
    return;
  }

  const initData: string = import.meta.env.VITE_MOCK_INIT_DATA ?? UNSIGNED_INIT_DATA;

  mockTelegramEnv({
    launchParams: {
      tgWebAppData: initData,
      tgWebAppVersion: '8.0',
      tgWebAppPlatform: 'tdesktop',
      tgWebAppThemeParams: LIGHT_THEME,
    },
  });

  console.info(
    '[flatmatch] Telegram environment mocked for browser development.',
    import.meta.env.VITE_MOCK_INIT_DATA
      ? ''
      : 'VITE_MOCK_INIT_DATA is not set, so the API will answer 401 — see src/telegram/mockEnv.ts.',
  );
}

/**
 * Whether a real Telegram client started this page.
 *
 * The SDK's own `isTMA()` cannot answer this: it looks at the launch params it
 * can retrieve, and `mockTelegramEnv` writes those to sessionStorage. So the
 * first mocked load makes every later reload in the same tab claim to be
 * running inside Telegram, the mock is skipped, and mounting fails with
 * UnknownEnvError against a client that is not there.
 *
 * A real client always hands the parameters over in the URL, so that is what
 * gets asked. Storage left over from an earlier load cannot fake it.
 */
function launchedByTelegram(): boolean {
  return /tgWebApp/.test(window.location.hash) || /tgWebApp/.test(window.location.search);
}

/**
 * Structurally valid launch data with a hash that will never verify. It exists
 * so the SDK has something to parse; the API is expected to reject it.
 */
const UNSIGNED_INIT_DATA =
  'user=%7B%22id%22%3A777000001%2C%22first_name%22%3A%22%D0%94%D0%B5%D0%B2%22%2C%22username%22%3A%22dev_user%22%7D' +
  '&chat_type=private&auth_date=1754000000&hash=0000000000000000000000000000000000000000000000000000000000000000';

/**
 * Telegram's default light palette, so the mocked app is not unstyled.
 * `as const` keeps the literals matching the SDK's `#${string}` colour type.
 */
const LIGHT_THEME = {
  accent_text_color: '#6ab2f2',
  bg_color: '#ffffff',
  button_color: '#5288c1',
  button_text_color: '#ffffff',
  destructive_text_color: '#ec3942',
  header_bg_color: '#ffffff',
  hint_color: '#999999',
  link_color: '#6ab3f3',
  secondary_bg_color: '#f1f1f1',
  section_bg_color: '#ffffff',
  section_header_text_color: '#6ab3f3',
  subtitle_text_color: '#999999',
  text_color: '#000000',
} as const;
