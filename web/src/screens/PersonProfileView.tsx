/* Контейнер профиля человека: загрузка, дорогой лайк и его отказы (§2.3, §6).

   ЕДИНСТВЕННОЕ место, откуда ставится дорогой лайк: продукт разрешает его
   только с полного профиля. Отсюда же берётся контекст лайка — зона пула, из
   которого профиль открыт.

   В отличие от зонного пула, этот GET ничего не мутирует, поэтому ref-guard
   ему не нужен: второй прогон эффекта в StrictMode стоит лишнего запроса, а
   не съеденного бейджа. А вот POST лайка защищён флагом busy — второй тап
   отправил бы второй запрос за тот же лайк. */
import { useEffect, useRef, useState } from 'react';
import type { CSSProperties } from 'react';
import {
  mainButton,
  mountMainButton,
  onMainButtonClick,
  setMainButtonParams,
} from '@telegram-apps/sdk-react';

import { ApiError, fetchPerson, postLike, type PersonProfileDTO } from '../api/client';
import { fieldLabel, occupationLabel, untilTime } from '../api/format';
import { launchedByTelegram } from '../telegram/mockEnv';
import { PersonProfileScreen, type LikeAction, type MatchRow } from './PersonProfileScreen';

/** Слотов дорогих лайков на окне 24ч (§2.3). Сервер шлёт остаток, потолок знает спека. */
const LIKE_SLOTS = 5;

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
};

export function PersonProfileView({
  userId,
  zoneId,
  pointId,
  onBack,
  onLiked,
  onMatched,
  onNeedSearch,
}: {
  userId: string;
  /** Пул, из которого открыт профиль: и строка «пересеклись», и контекст лайка. */
  zoneId?: string;
  /**
   * Точка, из которой открыт профиль: пул квартиры или карточка «к себе».
   *
   * Не только контекст лайка, но и пропуск к самому профилю: отношение по
   * точке сервер проверяет ТОЛЬКО по этому параметру, без него профиль
   * со-участника пула отвечает 404 (§2.3 «мы увиделись»).
   */
  pointId?: string;
  onBack?: () => void;
  /**
   * Лайк ушёл. Списки входящих и мэтчей держит оболочка: ответ на входящий
   * убирает его из «Тебя лайкнули», а взаимный — добавляет мэтч. Остаток
   * бюджета едет вместе с ним: на ленте своего запроса за ним нет.
   */
  onLiked?: (remaining: number) => void;
  /** Взаимный лайк: экран мэтча забирает пара, лайк уже необратим (§2.3). */
  onMatched?: (matchId: string) => void;
  /** Лайк без активного поиска: продукт просит мини-форму поиска, не отказ. */
  onNeedSearch?: (message: string) => void;
}) {
  const [person, setPerson] = useState<PersonProfileDTO | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  /** Отказ сервера под кнопкой: лимит, непроверенная анкета, сеть. */
  const [refusal, setRefusal] = useState<string | null>(null);
  const [native, setNative] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setPerson(null);
    setError(null);
    setRefusal(null);
    fetchPerson(userId, { zoneId, pointId })
      .then((data) => {
        if (!cancelled) setPerson(data);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(message(e));
      });
    return () => {
      cancelled = true;
    };
  }, [userId, zoneId, pointId]);

  const like = async () => {
    // Кнопка гаснет не мгновенно (MainButton отвечает на setParams), а второй
    // тап ушёл бы вторым запросом за тот же лайк.
    if (busy || !person) return;
    // У мэтча кнопка не лайкает — ведёт в мэтч, где контакты.
    if (person.match_id) {
      onMatched?.(person.match_id);
      return;
    }
    setBusy(true);
    setRefusal(null);
    try {
      const res = await postLike(person.user_id, { zoneId, pointId });
      onLiked?.(res.budget.remaining);
      // matched приходит и на повторный тап по уже сматченному человеку —
      // ветвимся по нему, а не по already.
      if (res.matched && res.match_id) {
        onMatched?.(res.match_id);
        return;
      }
      // Лайк ушёл: кнопка становится «Лайк отправлен», остаток пересчитал сервер.
      setPerson({ ...person, my_like: true, budget: res.budget });
    } catch (e: unknown) {
      const err = asApiError(e);
      if (err.code === 'need_search') {
        onNeedSearch?.(err.message);
        return;
      }
      // like_limit несёт в сообщении час возврата, not_approved — причину:
      // оба показываем как есть, своих слов у клиента тут нет.
      setRefusal(err.message);
    } finally {
      setBusy(false);
    }
  };

  // Свежий обработчик в ref: подписка на MainButton ставится один раз, а like
  // пересобирается на каждый рендер (в нём и busy, и загруженный человек).
  const likeRef = useRef(like);
  useEffect(() => {
    likeRef.current = like;
  });

  // Родная кнопка Telegram. Спрашиваем не только SDK: под моком разработки
  // (mockEnv) все isAvailable() отвечают «да», вызовы уходят в перехваченный
  // postMessage, а кнопки в браузерной вкладке нет — экран остался бы вовсе
  // без действия. Настоящий клиент узнаётся по параметрам запуска в адресе.
  useEffect(() => {
    if (!launchedByTelegram() || !mountMainButton.isAvailable()) return undefined;
    if (!mainButton.isMounted()) mountMainButton();
    if (!setMainButtonParams.isAvailable() || !onMainButtonClick.isAvailable()) return undefined;

    setNative(true);
    const off = onMainButtonClick(() => {
      void likeRef.current();
    });
    return () => {
      off();
      setNative(false);
      // Прячем, а не размонтируем: размонтированную кнопку следующий экран
      // поднимал бы заново, а состояние Telegram всё равно восстанавливает.
      if (setMainButtonParams.isAvailable()) setMainButtonParams({ isVisible: false });
    };
  }, []);

  const action = person
    ? likeAction(person, busy, refusal, native)
    : { label: '', disabled: true, native };

  // Параметры кнопки — отдельным эффектом: текст и запрет меняются на каждый
  // ответ сервера, а подписка на клик от этого пересобираться не должна.
  useEffect(() => {
    if (!native || !setMainButtonParams.isAvailable()) return;
    setMainButtonParams({
      text: action.label,
      isEnabled: !action.disabled,
      isVisible: Boolean(action.label),
      isLoaderVisible: busy,
    });
  }, [native, action.label, action.disabled, busy]);

  if (error || !person) {
    return (
      <div style={V.screen}>
        <div style={V.header}>
          <span style={V.back} onClick={onBack}>
            ‹
          </span>
          <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Профиль</span>
        </div>
        <div style={V.centered}>{error ?? 'Секунду…'}</div>
      </div>
    );
  }

  return (
    <PersonProfileScreen
      name={person.name}
      age={person.age}
      occupation={occupationLabel(person.occupation)}
      photoUrl={person.photo_url || undefined}
      zone={person.context_zone?.name}
      point={person.context_point?.label}
      matches={toRows(person)}
      matchesTotal={HABIT_FIELDS}
      about={person.about}
      like={action}
      onBack={onBack}
      onLike={() => void like()}
    />
  );
}

/**
 * Текст и запрет кнопки — одни на MainButton и на кнопку вёрстки.
 *
 * Порядок веток и есть правило §2.3: свой лайк уже ушёл — делать нечего;
 * входящий лайк отвечается всегда, даже с исчерпанным лимитом (ответ его не
 * тратит); и только потом лимит запирает кнопку.
 */
function likeAction(
  p: PersonProfileDTO,
  busy: boolean,
  refusal: string | null,
  native: boolean,
): LikeAction {
  const base = { native, caption: refusal ?? undefined };
  if (p.match_id) return { ...base, label: '✓ У вас мэтч — открыть', match: true };
  if (p.my_like) return { ...base, label: 'Лайк отправлен', disabled: true };
  if (p.liked_me) {
    return {
      ...base,
      label: 'Ответить — мэтч сразу',
      caption: refusal ?? 'Ответный лайк не тратит лимит',
      disabled: busy,
    };
  }
  if (p.budget.remaining <= 0) {
    return {
      ...base,
      label: p.budget.next_free_at
        ? `Лимит исчерпан · следующий через ${untilTime(p.budget.next_free_at)}`
        : 'Лимит исчерпан',
      disabled: true,
    };
  }
  return {
    ...base,
    label: `Хочу жить вместе · осталось ${p.budget.remaining} из ${LIKE_SLOTS}`,
    disabled: busy,
  };
}

/**
 * Совпадения анкеты: сервер шлёт только их, различий в ответе нет — красного
 * в сравнении людей не существует (§6).
 *
 * Подпись строки — «вы оба» и ничего больше: метки ответов не склоняются
 * («не курю», «творческий беспорядок»), и фраза, собранная из них, читалась
 * бы поломанной.
 */
function toRows(p: PersonProfileDTO): MatchRow[] {
  return p.shared.map((s) => ({
    tone: 'both',
    title: capitalize(fieldLabel(s.field, s.value)),
    context: 'вы оба',
  }));
}

function capitalize(s: string): string {
  return s ? s[0].toUpperCase() + s.slice(1) : s;
}

function message(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}

function asApiError(e: unknown): ApiError {
  return e instanceof ApiError ? e : new ApiError(0, 'network', String(e));
}
