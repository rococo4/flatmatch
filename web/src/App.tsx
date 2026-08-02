import { AppRoot } from '@telegram-apps/telegram-ui';
import { useSignal, isMiniAppDark, useLaunchParams } from '@telegram-apps/sdk-react';

import { Hello } from './screens/Hello';

/**
 * Application root.
 *
 * AppRoot is told the appearance and platform explicitly rather than letting it
 * guess: the appearance comes from a signal, so switching the Telegram client
 * between light and dark re-renders the app immediately instead of only on the
 * next launch.
 */
export function App() {
  const isDark = useSignal(isMiniAppDark);
  const { tgWebAppPlatform } = useLaunchParams(true);

  // telegram-ui only distinguishes Apple platforms from the rest: iOS gets
  // Apple's control shapes, everything else gets Telegram's own.
  const platform = tgWebAppPlatform === 'ios' || tgWebAppPlatform === 'macos' ? 'ios' : 'base';

  return (
    <AppRoot appearance={isDark ? 'dark' : 'light'} platform={platform}>
      <Hello />
    </AppRoot>
  );
}
