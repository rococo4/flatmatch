/* Экран комнаты «к себе» — §6, вход по диплинку `point_<id>` и тапом по
   карточке подбора.

   Пул у «к себе» отсутствует (§2.2), поэтому смотреть здесь больше не на
   что — только витрина автора и «Хочу эту комнату».

   Лайк ставит оболочка: остаток бюджета, список уже отправленных откликов и
   мэтчи живут выше экранов, и второй их владелец разошёлся бы с первым. */
import { useEffect, useRef, useState } from 'react';
import type { CSSProperties } from 'react';

import { ApiError, fetchPoint, type PointDTO } from '../api/client';
import {
  dayMonth,
  habitChips,
  lastSeenLabel,
  money,
  occupationLabel,
  plural,
  pointTitle,
  restrictionLabels,
  roomCardTitle,
  shareValue,
} from '../api/format';
import { PhotoPager } from '../ui/PhotoPager';
import {
  FactsBlock,
  GreyChips,
  HabitsBlock,
  LimitChip,
  PersonRow,
  S,
  type Fact,
} from '../ui/primitives';

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

/**
 * Карточка комнаты целиком: витрина автора, факты, отклик.
 *
 * Живёт здесь, а не в подборе: в колоде карточка своя — без блока привычек и
 * без кнопки, её место занимает сердце под колодой (§6).
 */
function HomePointCard({
  point,
  likesRemaining,
  mine,
  responded,
  matched,
  onWantRoom,
  onOpenProfile,
}: {
  point: PointDTO;
  likesRemaining?: number;
  /** Своя точка: откликаться самому себе не на что, кнопки нет. */
  mine?: boolean;
  responded?: boolean;
  matched?: boolean;
  onWantRoom?: (point: PointDTO) => void;
  onOpenProfile?: (point: PointDTO) => void;
}) {
  const author = point.author;
  // Витрина «к себе» — это автор: без него карточке нечего показывать (§2.2).
  if (!author) return null;

  return (
    <article style={S.card}>
      <div style={{ ...S.cardHeadBase, background: 'var(--type-home-bg)', color: 'var(--type-home-fg)' }}>
        <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>
          {roomCardTitle(point.stage ?? 'live')}
        </span>
        <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82 }}>
          {lastSeenLabel(author.last_seen)}
        </span>
      </div>
      <div style={S.cardBody}>
        <div
          onClick={onOpenProfile ? () => onOpenProfile(point) : undefined}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            padding: 8,
            margin: -8,
            borderRadius: 11,
            cursor: onOpenProfile ? 'pointer' : 'default',
          }}
        >
          <PersonRow
            name={author.name}
            age={author.age}
            occupation={occupationLabel(author.occupation)}
            photoUrl={author.photo_urls[0]}
          />
          {onOpenProfile && (
            <span
              style={{
                marginLeft: 'auto',
                flex: '0 0 auto',
                display: 'flex',
                alignItems: 'center',
                gap: 4,
                fontSize: 13,
                fontWeight: 510,
                color: 'var(--tg-theme-accent-text-color)',
              }}
            >
              Анкета<span style={{ fontSize: 17, lineHeight: 1 }}>›</span>
            </span>
          )}
        </div>
        <FactsBlock title={`Комната у ${author.name}`} facts={homeFacts(point)}>
          {/* Фото листаются прямо здесь: их до десяти, и одна обложка от
              комнаты рассказывает меньше, чем один кадр квартиры. */}
          <div style={{ borderRadius: 8, overflow: 'hidden' }}>
            <PhotoPager
              urls={point.photo_urls}
              emptyLabel="фото комнаты"
              ratio="16/9"
              maxHeight="none"
            />
          </div>
          <GreyChips items={restrictionLabels(point.restrictions, 'intent_home')} />
        </FactsBlock>
        {/* Рассказ автора — под фактами: цифры отвечают «сколько», описание —
            «как там жить» (§2.2). Пусто — блока просто нет. */}
        {point.description && (
          <span style={{ fontSize: 14, lineHeight: 1.45, textWrap: 'pretty' }}>
            {point.description}
          </span>
        )}
        <HabitsBlock items={habitChips(author)} />
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            fontSize: 13,
            color: 'var(--tg-theme-subtitle-text-color)',
          }}
        >
          <span
            style={{
              width: 6,
              height: 6,
              borderRadius: '50%',
              background: 'var(--type-home-fg)',
              flex: '0 0 auto',
            }}
          />
          <span style={{ color: 'var(--tg-theme-text-color)', fontWeight: 510 }}>
            {point.responses} {plural(point.responses, 'отклик', 'отклика', 'откликов')}
          </span>
        </div>
        {/* Своя точка кнопки не несёт: отклик самому себе бессмыслен, и сервер
            его всё равно отобьёт. Остаток показываем, только когда он известен:
            своего запроса за бюджетом у экрана нет. */}
        {onWantRoom && !mine && matched && (
          <div
            style={{
              ...S.secondaryButton,
              background: 'var(--verdict-ok-bg)',
              color: 'var(--verdict-ok-fg)',
              textAlign: 'center',
            }}
          >
            ✓ У вас мэтч — контакты в «Моих мэтчах»
          </div>
        )}
        {onWantRoom && !mine && !matched && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 7 }}>
            <button
              type="button"
              style={responded ? S.secondaryButton : S.filledButton}
              disabled={responded}
              onClick={() => onWantRoom(point)}
            >
              {responded ? 'Отклик отправлен' : 'Хочу эту комнату'}
            </button>
            {!responded && likesRemaining != null && (
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <LimitChip small>{likesRemaining} из 5</LimitChip>
              </div>
            )}
          </div>
        )}
      </div>
    </article>
  );
}

function homeFacts(p: PointDTO): Fact[] {
  const facts: Fact[] = [];
  if (p.price_share) {
    facts.push({ label: 'Твоя доля', value: shareValue(p.price_share, p.utilities) });
  }
  if (p.zone.name) facts.push({ label: 'Метро', value: p.zone.name });
  if (p.isolated != null) {
    facts.push({ label: 'Комната', value: p.isolated ? 'изолированная' : 'проходная' });
  }
  if (p.deposit) facts.push({ label: 'Депозит', value: money(p.deposit) });
  if (p.move_in) facts.push({ label: 'Заезд', value: `с ${dayMonth(p.move_in)}` });
  return facts;
}

function message(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}
