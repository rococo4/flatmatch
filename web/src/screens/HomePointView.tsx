/* Экран карточки «к себе» — §6, вход по диплинку `point_<id>`.

   Своего вида у экрана нет намеренно: это ровно та же карточка, что в ленте
   (HomePointCard), с тем же откликом. Пул у «к себе» отсутствует (§2.2),
   поэтому смотреть здесь больше не на что — только витрина автора и «Хочу эту
   комнату».

   Лайк ставит оболочка: остаток бюджета, список уже отправленных откликов и
   мэтчи живут выше экранов, и второй их владелец разошёлся бы с первым. */
import { useEffect, useRef, useState } from 'react';
import type { CSSProperties } from 'react';

import { ApiError, fetchPoint, type PointDTO } from '../api/client';
import { pointTitle } from '../api/format';
import { HomePointCard } from './FeedScreen';

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
  body: {
    flex: '1 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
    padding: '12px 12px 20px',
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
};

export function HomePointView({
  pointId,
  likesRemaining,
  myUserId,
  respondedPoints,
  matchedAuthors,
  error,
  onBack,
  onWantRoom,
  onOpenProfile,
}: {
  pointId: string;
  /** Остаток дорогих лайков для подписи «N из 5» — тот же, что в ленте. */
  likesRemaining?: number;
  /** Своя карточка: откликаться самому себе не на что, кнопки нет. */
  myUserId?: string;
  respondedPoints?: string[];
  matchedAuthors?: string[];
  /** Отказ отклика (лимит, анкета на проверке) — словами сервера. */
  error?: string | null;
  onBack?: () => void;
  onWantRoom?: (point: PointDTO) => void;
  onOpenProfile?: (point: PointDTO) => void;
}) {
  const [point, setPoint] = useState<PointDTO | null>(null);
  const [failure, setFailure] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setPoint(null);
    setFailure(null);
    fetchPoint(pointId)
      .then((data) => {
        if (!cancelled) setPoint(data);
      })
      .catch((e: unknown) => {
        if (!cancelled) setFailure(message(e));
      });
    return () => {
      cancelled = true;
    };
  }, [pointId]);

  const responded = respondedPoints?.includes(pointId) ?? false;
  // Счётчик откликов считает сервер, поэтому после своего отклика карточка
  // перечитывается целиком. Ref-сторож нужен, чтобы не сходить за ней второй
  // раз при входе на экран с уже отправленным откликом.
  const wasResponded = useRef(responded);
  useEffect(() => {
    if (responded === wasResponded.current) return;
    wasResponded.current = responded;
    if (!responded) return;
    void fetchPoint(pointId)
      .then(setPoint)
      .catch(() => undefined);
  }, [responded, pointId]);

  return (
    <div style={V.screen}>
      <div style={V.header}>
        <span style={V.back} onClick={onBack}>
          ‹
        </span>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>
          {point ? pointTitle(point) : 'Комната'}
        </span>
      </div>

      {failure || !point ? (
        <div style={V.centered}>{failure ?? 'Секунду…'}</div>
      ) : (
        <div style={V.body}>
          {error && (
            <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>
              {error}
            </span>
          )}
          <HomePointCard
            point={point}
            likesRemaining={likesRemaining}
            mine={point.author?.user_id != null && point.author.user_id === myUserId}
            responded={responded}
            matched={point.author != null && matchedAuthors?.includes(point.author.user_id)}
            onWantRoom={onWantRoom}
            onOpenProfile={onOpenProfile}
          />
        </div>
      )}
    </div>
  );
}

function message(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}
