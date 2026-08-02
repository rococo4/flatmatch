import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@telegram-apps/telegram-ui/dist/styles.css';

import './index.css';
import { App } from './App';
import { initTelegram } from './telegram/init';
import { mockTelegramEnvIfNeeded } from './telegram/mockEnv';

// Mocking must happen before the SDK reads launch parameters, and both must
// happen before React renders anything that talks to Telegram.
if (import.meta.env.DEV) {
  mockTelegramEnvIfNeeded();
}

try {
  initTelegram(import.meta.env.DEV);
} catch (error) {
  // Opened outside Telegram in a production build: there is no client to talk
  // to, and nothing below will work. Say so in plain words instead of leaving
  // a blank page.
  document.body.innerHTML =
    '<p style="font-family: system-ui; padding: 24px; text-align: center">' +
    'Это Telegram Mini App — открой его через бота FlatMatch.</p>';
  console.error('[flatmatch] not running inside Telegram', error);
  throw error;
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
