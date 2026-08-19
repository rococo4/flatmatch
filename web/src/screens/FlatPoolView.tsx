/* Контейнер квартиры с рынка: карточка, её пул и членство в нём (§2.2, §6).

   Карточку отдаёт GET /points/{id}, список — GET /points/{id}/pool, и второй
   отвечает 403 need_enter тому, кто не входил. Это НЕ ошибка экрана: не
   вошедший видит ту же карточку с «в пуле N — увидишь после входа», поэтому
   need_enter превращается в отсутствие списка, а не в сообщение об отказе.

   Вход и выход мутируют пул, поэтому после каждого — перечитывание карточки:
   счётчик пула считает только сервер. */
import { useCallback, useEffect, useState } from 'react';
import type { CSSProperties } from 'react';
import { openLink } from '@telegram-apps/sdk-react';

import {
  ApiError,
  enterPointPool,
  fetchPoint,
  fetchPointPool,
  leavePointPool,
  restorePoint,
  votePoint,
  type PointDTO,
  type PointPoolMemberDTO,
} from '../api/client';
import {
  chipLabel,
  flatFacts,
  freshnessLabel,
  goneBanner,
  habitsLine,
  money,
  openOriginalLabel,
  pointStatusLabel,
  pointTitle,
  pointTitleGen,
  pricePerPerson,
  restrictionLabels,
  VERDICT_UI,
} from '../api/format';
import { launchedByTelegram } from '../telegram/mockEnv';
import { LeavePoolSheet } from './EdgeStates';
import { FlatPoolScreen, type FlatPoolCard, type FlatPoolPerson } from './FlatPoolScreen';

/** Карточка и список: список null — смотрящий не в пуле (403 need_enter). */
type Loaded = { point: PointDTO; members: PointPoolMemberDTO[] | null };

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
};

export function FlatPoolView({
  pointId,
  mine,
  onBack,
  onOpenPerson,
  onChanged,
  onLeft,
  onShare,
}: {
  pointId: string;
  /** Своя точка: «Выйти из пула» на ней не рисуется — вернуться было бы некуда. */
  mine?: boolean;
  onBack?: () => void;
  onOpenPerson?: (userId: string) => void;
  onShare?: (pointId: string) => void;
  /** Членство изменилось: хозяин списков перечитывает «Мои поиски» и ленту. */
  onChanged?: () => void;
  /** Вышли из пула: смотреть на экране больше нечего. */
  onLeft?: () => void;
}) {
  const [data, setData] = useState<Loaded | null>(null);
  const [error, setError] = useState<string | null>(null);
  /** Отказ действия — в отличие от `error` не заменяет собой экран. */
  const [refusal, setRefusal] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [leaving, setLeaving] = useState(false);
  /** По ссылке сходили — спрашиваем про свежесть. Возврат из браузера не ловим (§8). */
  const [asked, setAsked] = useState(false);
  const [voteBusy, setVoteBusy] = useState(false);
  /** Ответили только что: «Спасибо, отметили» вместо «Ты отметил: …». */
  const [justVoted, setJustVoted] = useState(false);
  /** Тапнули «изменить»: показываем кнопки, будто голоса ещё нет. */
  const [changing, setChanging] = useState(false);

  const load = useCallback(async (): Promise<Loaded> => {
    const point = await fetchPoint(pointId);
    const members = await fetchPointPool(pointId)
      .then((pool) => pool.members)
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.code === 'need_enter') return null;
        throw e;
      });
    return { point, members };
  }, [pointId]);

  useEffect(() => {
    let cancelled = false;
    setData(null);
    setError(null);
    setRefusal(null);
    setAsked(false);
    setJustVoted(false);
    setChanging(false);
    load()
      .then((loaded) => {
        if (!cancelled) setData(loaded);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(message(e));
      });
    return () => {
      cancelled = true;
    };
  }, [load]);

  const enter = async () => {
    if (busy) return;
    setBusy(true);
    setRefusal(null);
    try {
      await enterPointPool(pointId);
      onChanged?.();
      setData(await load());
    } catch (e: unknown) {
      setRefusal(message(e));
    } finally {
      setBusy(false);
    }
  };

  const leave = async () => {
    setLeaving(false);
    if (busy) return;
    setBusy(true);
    setRefusal(null);
    try {
      await leavePointPool(pointId);
      onChanged?.();
      onLeft?.();
    } catch (e: unknown) {
      setRefusal(message(e));
    } finally {
      setBusy(false);
    }
  };

  /**
   * Голос о свежести. Карточка берётся из ответа, а не перечитывается: голос
   * мог прямо сейчас скрыть точку (порог набран), и ответ уже несёт paused.
   */
  const vote = async (alive: boolean) => {
    if (voteBusy) return;
    setVoteBusy(true);
    setRefusal(null);
    try {
      const point = await votePoint(pointId, alive);
      setData((d) => (d ? { ...d, point } : d));
      setChanging(false);
      setJustVoted(true);
      onChanged?.();
    } catch (e: unknown) {
      setRefusal(message(e));
    } finally {
      setVoteBusy(false);
    }
  };

  /** «Проверил — она живая»: пауза снята, голоса стёрты — карточка из ответа. */
  const restore = async () => {
    if (busy) return;
    setBusy(true);
    setRefusal(null);
    try {
      const point = await restorePoint(pointId);
      setData((d) => (d ? { ...d, point } : d));
      setJustVoted(false);
      setChanging(false);
      onChanged?.();
    } catch (e: unknown) {
      setRefusal(message(e));
    } finally {
      setBusy(false);
    }
  };

  if (error || !data) {
    return (
      <div style={V.screen}>
        <div style={V.header}>
          <span style={V.back} onClick={onBack}>
            ‹
          </span>
          <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Квартира</span>
        </div>
        <div style={V.centered}>{error ?? 'Секунду…'}</div>
      </div>
    );
  }

  const { point, members } = data;
  // Спрашивать про свежесть есть смысл только у живой квартиры с рынка со
  // ссылкой: у скрытой голоса уже сыграли, у «к себе» их не бывает вовсе (§8).
  const votable = point.type === 'flat' && !!point.url && point.status === 'active';
  const myVote = point.my_vote ?? null;
  return (
    <>
      <FlatPoolScreen
        flat={toCard(point)}
        members={members?.map(toPerson) ?? null}
        // Вошедшему счётчик считать нечего: в списке ровно те, кого он видит,
        // а сам он в пуле не числится (visible_people не возвращает смотрящего).
        poolCount={members ? members.length : point.pool_count}
        // Точку на проверке видит только тот, кто её подал (§5), — она своя,
        // даже если список «Моих поисков» ещё не доехал. Своя пауза узнаётся по
        // restorable: «Выйти из пула» на собственной скрытой точке — не про неё.
        // Чужая пауза — другое дело: вошедшему выйти из такого пула можно.
        mine={mine || !!point.restorable || (point.status !== 'active' && point.status !== 'paused')}
        busy={busy}
        error={refusal}
        vote={
          votable
            ? {
                asked: asked || changing,
                // «изменить» возвращает кнопки: экрану это тот же «голоса нет».
                myVote: changing ? null : myVote,
                busy: voteBusy,
                justVoted,
              }
            : undefined
        }
        onBack={onBack}
        onOpenSource={() => {
          if (point.url) openExternal(point.url);
          if (votable) setAsked(true);
        }}
        onOpenPerson={onOpenPerson}
        onShare={onShare && point.status === 'active' ? () => onShare(pointId) : undefined}
        onEnter={() => void enter()}
        onLeave={() => setLeaving(true)}
        onVote={(alive) => void vote(alive)}
        onRestore={() => void restore()}
        onChangeVote={() => setChanging(true)}
      />
      {leaving && (
        <LeavePoolSheet
          pointTitle={pointTitleGen(point)}
          onStay={() => setLeaving(false)}
          onLeave={() => void leave()}
        />
      )}
    </>
  );
}

/**
 * Шапка-карточка.
 *
 * Цена доли — только когда есть на что делить: комнатность объявление называет
 * не всегда, и придумывать «на двоих» за него нельзя (то же правило в ленте).
 */
function toCard(p: PointDTO): FlatPoolCard {
  const share = pricePerPerson(p.price_total, p.rooms);
  const total = `${money(p.price_total ?? 0)} / мес за квартиру`;
  return {
    title: pointTitle(p),
    photoUrls: p.photo_urls.filter(Boolean),
    price: share !== null ? `${money(share)} с человека` : total,
    priceTotal: share !== null ? total : undefined,
    facts: flatFacts(p),
    restrictions: restrictionLabels(p.restrictions, 'flat'),
    freshness: freshnessLabel(p),
    // У точки на паузе строки статуса нет: то же самое, только громче,
    // говорит жёлтая плашка — две «похоже, сдана» подряд были бы шумом.
    status: p.status === 'paused' ? undefined : pointStatusLabel(p.status) || undefined,
    gone:
      p.status === 'paused'
        ? { banner: goneBanner(p.gone_votes), restorable: !!p.restorable }
        : undefined,
    sourceLabel: p.url ? openOriginalLabel(p.url) : undefined,
  };
}

/**
 * Участник пула.
 *
 * Вердикт-чипы собираются как в зонном пуле: цвет из VERDICT_UI, текст — из
 * параметров самого человека. Поиска у него может не быть вовсе (вход в пул
 * его не требует), и тогда чипов нет — сравнивать нечего.
 */
function toPerson(m: PointPoolMemberDTO): FlatPoolPerson {
  const params = m.params;
  return {
    id: m.user_id,
    name: m.name,
    age: m.age,
    photoUrl: m.photo_url || undefined,
    habits: habitsLine(m),
    matches: m.shared_answers,
    matched: m.matched,
    verdicts: params
      ? (m.chips ?? []).map((c) => ({ verdict: VERDICT_UI[c.verdict], label: chipLabel(c, params) }))
      : [],
  };
}

/**
 * Открывает оригинал объявления снаружи приложения.
 *
 * Родным вызовом — но только в настоящем клиенте: под моком разработки
 * isAvailable() отвечает «да», а вызов уходит в перехваченный postMessage, и
 * по ссылке не происходило бы ничего (то же правило, что у контакта мэтча).
 */
function openExternal(url: string): void {
  if (launchedByTelegram() && openLink.isAvailable()) {
    openLink(url);
    return;
  }
  window.open(url, '_blank', 'noopener');
}

function message(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}
