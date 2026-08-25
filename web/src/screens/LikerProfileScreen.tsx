/* Анкета лайкнувшего — вход из плиток «Лайков» (§6).

   Та же анкета, что в подборе (PersonProfileScreen), но внизу два жеста, а не
   один: стрелка отклоняет (тот же пропуск на 7 дней, §2.1 дизайна — сам лайк
   при этом цел, сгорает он только с мэтчем и блоком, §2.3), сердце отвечает
   взаимностью. Ответный лайк бесплатен и рождает мэтч сразу, поэтому остатка
   слотов под кнопками нет — вместо него подпись про то, что лимит цел. */
import { useEffect, useState } from 'react';
import type { CSSProperties } from 'react';

import { ApiError, fetchPerson, type PersonProfileDTO } from '../api/client';
import { likedYouLabel, occupationLabel, seenLabel } from '../api/format';
import { ActionPills } from '../ui/ActionPills';
import { S } from '../ui/primitives';
import { PersonProfileScreen, sharedRows } from './PersonProfileScreen';

/** Знаменатель «6 ИЗ 9»: девять привычек анкеты, по которым считаются совпадения (§4). */
const HABIT_FIELDS = 9;

const V = {
  screen: {
    display: 'flex',
    flexDirection: 'column',
    minHeight: '100%',
    background: 'var(--app-bg)',
    color: 'var(--tg-theme-text-color)',
  } as CSSProperties,
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    padding: '0 14px',
    height: 52,
    flex: '0 0 52px',
    background: 'var(--tg-theme-header-bg-color)',
    borderBottom: '1px solid var(--sep)',
  } as CSSProperties,
  back: {
    fontSize: 22,
    lineHeight: 1,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  centered: {
    flex: '1 1 auto',
    display: 'grid',
    placeItems: 'center',
    padding: 24,
    fontSize: 14,
    textAlign: 'center',
    color: 'var(--tg-theme-hint-color)',
  } as CSSProperties,
  bottom: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 8,
  } as CSSProperties,
};

export function LikerProfileScreen({
  userId,
  zoneId,
  pointId,
  busy,
  error,
  onBack,
  onDecline,
  onReply,
}: {
  userId: string;
  /** Контекст лайка: и строка «пересеклись», и пропуск к самой анкете (§2.3). */
  zoneId?: string;
  pointId?: string;
  /** Ответ или отклонение в полёте: кнопки гаснут, второй тап не уходит вторым запросом. */
  busy?: boolean;
  /** Отказ сервера — словами сервера: «анкета на проверке» и подобное. */
  error?: string | null;
  onBack?: () => void;
  onDecline?: (userId: string) => void;
  onReply?: (userId: string) => void;
}) {
  const [person, setPerson] = useState<PersonProfileDTO | null>(null);
  const [failure, setFailure] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setPerson(null);
    setFailure(null);
    fetchPerson(userId, { zoneId, pointId })
      .then((data) => {
        if (!cancelled) setPerson(data);
      })
      .catch((e: unknown) => {
        if (!cancelled) setFailure(message(e));
      });
    return () => {
      cancelled = true;
    };
  }, [userId, zoneId, pointId]);

  if (failure || !person) {
    return (
      <div style={V.screen}>
        <div style={V.header}>
          <span style={V.back} onClick={onBack}>
            ‹
          </span>
          <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Профиль</span>
        </div>
        <div style={V.centered}>{failure ?? 'Секунду…'}</div>
      </div>
    );
  }

  return (
    <PersonProfileScreen
      name={person.name}
      age={person.age}
      occupation={occupationLabel(person.occupation)}
      photoUrls={person.photo_urls}
      lastSeen={seenLabel(person.last_seen, person.gender)}
      note={likedYouLabel(person.gender)}
      zone={person.context_zone?.name}
      point={person.context_point?.label}
      matches={sharedRows(person.shared)}
      matchesTotal={HABIT_FIELDS}
      about={person.about}
      bottom={
        <div style={V.bottom}>
          <ActionPills
            likeLabel="Взаимно"
            likeLocked={busy}
            onPass={() => onDecline?.(person.user_id)}
            onLike={() => onReply?.(person.user_id)}
          />
          {error ? (
            <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>
              {error}
            </span>
          ) : (
            <span style={S.caption}>
              Сердце — взаимный лайк: не тратит лимит и открывает контакты
            </span>
          )}
        </div>
      }
      onBack={onBack}
    />
  );
}

function message(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}
