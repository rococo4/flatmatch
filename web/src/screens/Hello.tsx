import { useEffect, useState } from 'react';
import { Avatar, Caption, Placeholder, Spinner } from '@telegram-apps/telegram-ui';

import { ApiError, fetchMe, type Profile } from '../api/client';

type State =
  | { kind: 'loading' }
  | { kind: 'ready'; profile: Profile }
  | { kind: 'failed'; error: ApiError };

/**
 * The greeting screen — the whole of §13's front-end requirement.
 *
 * It exists to prove the chain end to end: the SDK hands over initData, the API
 * verifies the HMAC, finds or creates the account, and answers with a profile.
 * The feed from §6 replaces it in §12 step 3.
 */
export function Hello() {
  const [state, setState] = useState<State>({ kind: 'loading' });

  useEffect(() => {
    let cancelled = false;

    fetchMe()
      .then((profile) => {
        if (!cancelled) setState({ kind: 'ready', profile });
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setState({
          kind: 'failed',
          error: error instanceof ApiError ? error : new ApiError(0, 'network', String(error)),
        });
      });

    // The screen can unmount before the request settles — during hot reload,
    // or in StrictMode's double-invoked effect.
    return () => {
      cancelled = true;
    };
  }, []);

  if (state.kind === 'loading') {
    return (
      <Placeholder description="Секунду…">
        <Spinner size="l" />
      </Placeholder>
    );
  }

  if (state.kind === 'failed') {
    return <Failure error={state.error} />;
  }

  const { profile } = state;

  return (
    <Placeholder
      header={`Привет, ${profile.name}`}
      description="Квартиру найдёшь где угодно — мы найдём того, с кем её снимать."
    >
      {profile.photo_url ? (
        <Avatar size={96} src={profile.photo_url} alt={profile.name} />
      ) : (
        <Avatar size={96} acronym={profile.name.slice(0, 1)} />
      )}
      <Caption level="1" weight="3">
        {statusLine(profile)}
      </Caption>
    </Placeholder>
  );
}

/**
 * Placeholder wording for the skeleton screen.
 *
 * `draft` and `pending` must not read the same: telling somebody their
 * questionnaire is «на проверке» when they never filled one in is simply
 * false. Both lines disappear in §12 step 2, when a draft leads into
 * onboarding instead of a caption.
 */
function statusLine(profile: Profile): string {
  switch (profile.status) {
    case 'draft':
      return 'Анкета не заполнена';
    case 'pending':
      return 'Анкета на проверке';
    default:
      return `Аккаунт создан ${formatDate(profile.created_at)}`;
  }
}

function Failure({ error }: { error: ApiError }) {
  // 401 during development almost always means the mocked initData is not
  // signed with this bot's token, so say that rather than just "unauthorized".
  const hint =
    error.status === 401
      ? 'Проверь BOT_TOKEN у api и VITE_MOCK_INIT_DATA в web/.env.local — см. README.'
      : error.message;

  return (
    <Placeholder header="Не получилось войти" description={hint}>
      <Caption level="1" weight="3">
        {error.code}
        {error.status ? ` · HTTP ${error.status}` : ''}
      </Caption>
    </Placeholder>
  );
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('ru-RU', {
    day: 'numeric',
    month: 'long',
    year: 'numeric',
  });
}
