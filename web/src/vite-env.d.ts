/// <reference types="vite/client" />

interface ImportMetaEnv {
  /**
   * Signed initData for browser development, produced by
   * `cd api && go run ./cmd/devinitdata`. Only read in dev builds.
   */
  readonly VITE_MOCK_INIT_DATA?: string;
  /**
   * Absolute API origin. Leave unset — the dev server proxies /api and Caddy
   * does the same in production, which keeps every call same-origin.
   */
  readonly VITE_API_BASE_URL?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
