import {
  init as initSDK,
  setDebug,
  mountMiniAppSync,
  miniAppReady,
  bindMiniAppCssVars,
  mountThemeParamsSync,
  bindThemeParamsCssVars,
  mountViewport,
  bindViewportCssVars,
  expandViewport,
  mountBackButton,
} from '@telegram-apps/sdk-react';

/**
 * Wires the app to the Telegram client.
 *
 * Order matters: the SDK has to be initialised before any component is
 * mounted, and `miniAppReady()` must come last — it is what tells Telegram to
 * hide the loading placeholder, and calling it before the UI can paint shows
 * the user a blank screen.
 *
 * The CSS variable bindings are why the app follows the client's light and
 * dark theme without any theming code of our own (§10: colours come from
 * Telegram's theme variables).
 */
export function initTelegram(debug: boolean): void {
  setDebug(debug);
  initSDK();

  mountMiniAppSync();
  bindMiniAppCssVars();

  mountThemeParamsSync();
  bindThemeParamsCssVars();

  // Without mounting, every showBackButton() is a no-op and screens that ask
  // for the client's back arrow silently get none — which is how the
  // questionnaire ended up with no way back.
  if (mountBackButton.isAvailable()) {
    mountBackButton();
  }

  // The viewport mounts asynchronously — the client has to answer with its
  // size. Failing to get it is not fatal: it costs the safe-area variables,
  // not the app.
  mountViewport()
    .then(() => {
      bindViewportCssVars();
      // Open at full height instead of the half-screen sheet Telegram starts
      // with, so a list is usable without the user dragging it up first.
      if (expandViewport.isAvailable()) {
        expandViewport();
      }
    })
    .catch((error: unknown) => {
      console.warn('[flatmatch] viewport unavailable, continuing without it', error);
    });

  miniAppReady();
}
