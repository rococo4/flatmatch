/* Оболочка приложения: тема, табы §6 и переходы между экранами.

   Живыми данными питаются анкета (§12 шаг 2), зоны и поиски (§12 шаг 3),
   профиль человека, лайки и мэтчи (§12 шаг 4), комнаты «к себе» (§12 шаг 5),
   подбор «Соседи» (§12 шаг 11). DEMO-экранов не осталось.

   Здесь же живёт колода подбора — намеренно выше экранов: переключение табов
   не должно ронять её и терять место, на котором человек остановился. И здесь
   единственный владелец списка поисков: после любой мутации он перечитывается
   с сервера целиком, локальных «оптимистичных» правок нет — счётчики и номера
   слотов считает только сервер. Входящие лайки и мэтчи — по тому же правилу:
   бейдж на табе считается по одному списку, который перечитывается после
   каждого лайка, откуда бы его ни поставили. */
import { useCallback, useEffect, useRef, useState } from 'react';
import { isMiniAppDark, openTelegramLink, useSignal } from '@telegram-apps/sdk-react';

import {
  ApiError,
  clearPasses,
  closePoint,
  closeSearch,
  createSearch,
  fetchDeck,
  fetchIncoming,
  fetchMatches,
  fetchMe,
  fetchPoint,
  fetchPointsMine,
  fetchSearches,
  fetchTopZones,
  fetchZones,
  hasDeckFilters,
  passPerson,
  passPoint,
  pointShareUrl,
  postLike,
  postSource,
  reactivateSearch,
  startParam,
  updateSearch,
  type CloseReason,
  type DeckCard,
  type DeckFilters,
  type IncomingLikeDTO,
  type MatchDTO,
  type MyPointsResponse,
  type PointDTO,
  type Profile,
  type SearchResponse,
  type StartTarget,
  type TopZoneDTO,
  type Zone,
} from './api/client';
import {
  closeReasonLabel,
  dayMonth,
  habitsLine,
  occupationLabel,
  ownPhotoSrc,
  paramChips,
  relativeDay,
} from './api/format';
import { launchedByTelegram } from './telegram/mockEnv';
import { TabBar, type Tab } from './ui/TabBar';
import { OnboardingFlow } from './screens/OnboardingFlow';
import { DeckScreen } from './screens/DeckScreen';
import { FiltersSheet, loadDeckFilters, saveDeckFilters } from './screens/FiltersSheet';
import {
  draftToParams,
  EMPTY_DRAFT,
  isDraftReady,
  paramsToDraft,
  SearchParamsScreen,
  type SearchDraft,
} from './screens/SearchParamsScreen';
import { PersonProfileView } from './screens/PersonProfileView';
import { MatchView } from './screens/MatchView';
import { HomePointView } from './screens/HomePointView';
import { IncomingLikesScreen, type IncomingLike } from './screens/IncomingLikesScreen';
import { MySearchesScreen, type ArchivedSearch, type SavedSearch } from './screens/MySearchesScreen';
import { MyProfileScreen, type FixTarget, type MyMatch } from './screens/MyProfileScreen';
import { ProfileEditScreen } from './screens/ProfileEditScreen';
import { HomePointFormScreen } from './screens/PointFormScreens';
import { CloseSearchSheet } from './screens/EdgeStates';

type View = 'tab' | 'searchForm' | 'person' | 'match' | 'homePoint' | 'homeForm' | 'editProfile';

type Loading =
  | { kind: 'loading' }
  | { kind: 'ready'; profile: Profile }
  | { kind: 'failed'; error: ApiError };

/**
 * Открытый профиль человека.
 *
 * `zoneId` и `pointId` — откуда пришли: контекст и строки «пересеклись», и
 * лайка, и самого доступа к профилю (по точке сервер проверяет отношение
 * только по параметру запроса, §2.3). Взаимоисключимы. `back` — куда
 * возвращает стрелка: из пула уходят обратно в пул, а не «в никуда».
 */
type PersonTarget = { userId: string; zoneId?: string; pointId?: string; back: View };

/**
 * Колода подбора.
 *
 * `index` — курсор, а не вырезание из массива: карточки не переставляются, и
 * возврат на вкладку не должен начинать колоду заново. `loaded` отличает
 * «ещё не грузили» от «пусто»: в первом случае экран говорит «Секунду…», во
 * втором — «Пока все».
 */
type DeckState = {
  cards: DeckCard[];
  index: number;
  total: number;
  passed: number;
  noSearch: boolean;
  loaded: boolean;
  /** Отказ последнего действия (лимит лайков, анкета на проверке) — словами сервера. */
  error: string | null;
};

/**
 * Открытая форма поиска.
 *
 * У правки свой черновик и запертая зона (§3), у создания — свой: две формы
 * не должны переписывать друг другу параметры.
 */
type FormMode =
  | {
      kind: 'create';
      draft: SearchDraft;
      busy: boolean;
      error: string | null;
      invalidFields: string[];
    }
  | {
      kind: 'edit';
      searchId: string;
      /** Имя зоны из поиска — пока справочник грузится, строка зоны не пустует. */
      zoneName: string;
      draft: SearchDraft;
      busy: boolean;
      error: string | null;
      invalidFields: string[];
    };

const EMPTY_DECK: DeckState = {
  cards: [],
  index: 0,
  total: 0,
  passed: 0,
  noSearch: false,
  loaded: false,
  error: null,
};

/** Слотов поисков на человека (§2.1). Потолок знает спека, сервер его же и держит. */
const SEARCH_SLOTS = 3;

/** Слотов дорогих лайков на окне 24 ч (§2.3): столько точек под колодой. */
const LIKE_SLOTS = 5;

export function App() {
  const isDark = useSignal(isMiniAppDark);
  const [state, setState] = useState<Loading>({ kind: 'loading' });
  const [tab, setTab] = useState<Tab>('deck');
  const [view, setView] = useState<View>('tab');
  // Держит финальную развилку §4 на экране после отправки: статус к этому
  // моменту уже не draft, и без флага оболочка увела бы в подбор сразу.
  const [finishing, setFinishing] = useState(false);
  /**
   * Что открыть на экране анкеты сразу. Пусто — просто список.
   *
   * Правку зовут точечно: вердикт модератора отклоняет фото или «о себе», и
   * человека надо привести ровно туда, а не прогонять анкету заново (§5).
   */
  const [editTarget, setEditTarget] = useState<'photo' | 'about' | null>(null);
  /**
   * Метка перезалитого своего фото (`ownPhotoSrc`).
   *
   * Живёт здесь, а не в экране анкеты: адрес фото после замены прежний, и
   * кэш браузера отдал бы старый кадр и на профиле, куда человек уходит
   * сразу после «Заменить фото».
   */
  const [photoStamp, setPhotoStamp] = useState<number | null>(null);
  /**
   * Диплинк запуска: кнопка пуша или ссылка шеринга (§5 дизайна пушей).
   *
   * Разбирается один раз при старте и гасится сразу после перехода: второй раз
   * он увёл бы человека с экрана, куда тот уже ушёл сам.
   */
  const [startTarget, setStartTarget] = useState<StartTarget | null>(() => startParam());

  const [zones, setZones] = useState<Zone[] | null>(null);
  const [searches, setSearches] = useState<SearchResponse[] | null>(null);
  /**
   * Мои точки: блоки «Поисков».
   *
   * Как и поиски, живут выше экранов и перечитываются целиком после каждой
   * мутации — счётчики откликов и статусы модерации считает только сервер.
   */
  const [myPoints, setMyPoints] = useState<MyPointsResponse>({ created: [], entered: [] });
  const [deck, setDeck] = useState<DeckState>(EMPTY_DECK);
  /** Фильтры подбора: сервер их не помнит, а человек между заходами — да (§6). */
  const [filters, setFilters] = useState<DeckFilters>(() => loadDeckFilters());
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [formMode, setFormMode] = useState<FormMode | null>(null);
  /** Открытая комната «к себе»: пула у неё нет, только витрина и отклик (§2.2). */
  const [homePoint, setHomePoint] = useState<string | null>(null);
  /**
   * Отказ отклика на экране комнаты — своя строка, не общая с подбором.
   *
   * Иначе «лимит исчерпан» из колоды встречал бы человека на карточке
   * комнаты, куда он зашёл совсем за другим, и наоборот.
   */
  const [roomError, setRoomError] = useState<string | null>(null);
  /**
   * «Где уже ищут» для подбора без поиска (§6).
   *
   * Грузится один раз и только когда поиска нет: топ зон одинаков для всех, а
   * тратить запрос на каждый заход в непустую колоду незачем.
   */
  const [topZones, setTopZones] = useState<TopZoneDTO[] | null>(null);
  /**
   * Остаток дорогих лайков: точки под колодой и подпись на карточке комнаты.
   *
   * Своей ручки у бюджета нет — он приезжает ответом на лайк и с профилем
   * человека, поэтому до первого лайка в сессии точки показывают полный
   * запас: окно скользящее (§2.3), и чаще всего он и правда полон.
   */
  const [likesRemaining, setLikesRemaining] = useState<number | null>(null);
  /**
   * Точки, на которые отклик уже улетел.
   *
   * Локально, а не с сервера: карточка точки не несёт «я откликался», а
   * повторный тап сервер бы простил (already), но человеку кнопка должна
   * отвечать сразу.
   */
  const [respondedPoints, setRespondedPoints] = useState<string[]>([]);
  const [person, setPerson] = useState<PersonTarget | null>(null);
  /** Открытый мэтч: саммари по нему грузит MatchView. */
  const [matchId, setMatchId] = useState<string | null>(null);
  /**
   * Входящие лайки и мэтчи — здесь, а не в экранах: по длине входящих считается
   * бейдж таба «Профиль», и он обязан быть тем же числом, что в разделе.
   */
  const [incoming, setIncoming] = useState<IncomingLikeDTO[]>([]);
  const [matches, setMatches] = useState<MatchDTO[]>([]);
  /** Кому сейчас отвечаем: второй тап не должен уйти вторым запросом. */
  const [replying, setReplying] = useState<string | null>(null);
  const [incomingError, setIncomingError] = useState<string | null>(null);
  /**
   * Профиль, который ждёт сохранённого поиска.
   *
   * Лайк без активного поиска сервер отклоняет (§2.3), и продукт вместо отказа
   * показывает форму поиска. Сохранился поиск — возвращаем человека туда,
   * откуда он ушёл: искать тот же профиль заново он не должен.
   */
  const [likeReturn, setLikeReturn] = useState<PersonTarget | null>(null);
  const [closing, setClosing] = useState<string | null>(null);
  const [searchesError, setSearchesError] = useState<string | null>(null);

  // Свежие колода и фильтры в ref: их читают стабильные колбэки (первый
  // запрос, обновление вкладки), а пересобирать те на каждую пролистанную
  // карточку — значит пересобирать и подписки, которые на них висят.
  const deckRef = useRef(deck);
  deckRef.current = deck;
  const filtersRef = useRef(filters);
  filtersRef.current = filters;

  // Токены дизайна переключают тему по data-theme на <html>, а не по классу на
  // корне приложения: под ними живут и шторки, которые рисуются выше по дереву.
  useEffect(() => {
    document.documentElement.dataset.theme = isDark ? 'dark' : 'light';
  }, [isDark]);

  /**
   * Перечитывает входящие лайки и мэтчи.
   *
   * Оба списка меняет один и тот же жест: ответ на входящий убирает его из
   * «Тебя лайкнули» и добавляет мэтч. Отказ переживаем молча — на экране
   * останутся прежние цифры, а не пустота вместо них.
   */
  const reloadLikes = useCallback(() => {
    void fetchIncoming()
      .then(setIncoming)
      .catch(() => undefined);
    void fetchMatches()
      .then(setMatches)
      .catch(() => undefined);
  }, []);

  /** Перечитывает блок «Зову к себе». Отказ переживаем молча. */
  const reloadPoints = useCallback(() => {
    void fetchPointsMine()
      .then(setMyPoints)
      .catch(() => undefined);
  }, []);

  /**
   * Загружает колоду заново — с начала.
   *
   * Курсор всегда обнуляется: колода приезжает целиком и в своём порядке, а
   * «продолжить с шестой карточки» после смены фильтров значило бы пропустить
   * пять новых.
   */
  const loadDeck = useCallback(async (f: DeckFilters) => {
    try {
      const data = await fetchDeck(f);
      setDeck({
        cards: data.cards,
        index: 0,
        total: data.total,
        passed: data.passed,
        noSearch: data.no_search,
        loaded: true,
        error: null,
      });
    } catch (e: unknown) {
      // `loaded` не трогаем: неудачная первая загрузка — это не пустая колода,
      // и «Пока все» на ней было бы неправдой. Экран покажет отказ строкой.
      setDeck((d) => ({ ...d, error: asApiError(e).message }));
    }
  }, []);

  useEffect(() => {
    let cancelled = false;
    fetchMe()
      .then((profile) => {
        if (cancelled) return;
        setState({ kind: 'ready', profile });

        // Справочник — фоном и без ожидания: он нужен только пикеру зон, а
        // промис уже кэшируется в client.ts. Не успеет — пикер честно скажет
        // «Загружаю справочник…».
        void fetchZones()
          .then((dir) => {
            if (!cancelled) setZones(dir.zones);
          })
          .catch(() => {
            // Молча: без справочника работает всё, кроме выбора зоны.
          });

        // Поиски нужны не подбору (его колоду сервер собирает сам), а счётчику
        // слотов и вкладке «Поиски».
        void fetchSearches()
          .then((list) => {
            if (!cancelled) setSearches(list);
          })
          .catch(() => {
            // Молча: колода приедет и без списка, слоты покажет следующий заход.
          });

        // Колода — первое, что человек видит: «no_search» в её ответе и есть
        // развилка «с поиском / без поиска» (§6).
        void loadDeck(filtersRef.current);

        // Бейдж входящих на табе «Лайки» нужен с первого экрана — иначе о
        // лайке узнают только те, кто сам зашёл в раздел.
        reloadLikes();
        reloadPoints();
      })
      .catch((error: unknown) => {
        if (cancelled) return;
        setState({
          kind: 'failed',
          error: error instanceof ApiError ? error : new ApiError(0, 'network', String(error)),
        });
      });
    return () => {
      cancelled = true;
    };
  }, [reloadLikes, reloadPoints, loadDeck]);

  const setProfile = useCallback((profile: Profile) => setState({ kind: 'ready', profile }), []);

  /**
   * Перечитывает поиски целиком: счётчики и номера слотов считает только сервер.
   *
   * Живёт выше экранов — и выше ранних возвратов оболочки: её зовут и
   * переключение табов, и разбор диплинка, то есть замыкания, созданные в
   * рендере, который до тела экранов не дошёл.
   */
  const reloadSearches = useCallback(async (): Promise<SearchResponse[]> => {
    const list = await fetchSearches();
    setSearches(list);
    return list;
  }, []);

  /** Форма поиска: «Найти», «ГДЕ УЖЕ ИЩУТ», «Создать ещё», диплинк без поиска. */
  const openSearchForm = useCallback((draft: SearchDraft) => {
    setFormMode({ kind: 'create', draft, busy: false, error: null, invalidFields: [] });
    setTab('deck');
    setView('searchForm');
  }, []);

  /**
   * Перечитывает вкладку. Реалтайма нет (§7): вердикты модератора, входящие
   * лайки и мэтчи приходят вне приложения, поэтому вкладка обновляет своё при
   * каждом входе и при возврате приложения из фона — иначе счётчики стоят до
   * рестарта. Полной перезагрузки страницы TMA не делают: она сбрасывает
   * состояние и заново инициализирует SDK.
   */
  const refreshTab = useCallback(
    (which: Tab) => {
      if (which === 'searches') {
        reloadPoints();
        void reloadSearches().catch(() => undefined);
      }
      if (which === 'likes' || which === 'profile') {
        reloadLikes();
      }
      if (which === 'profile') {
        void fetchMe()
          .then(setProfile)
          .catch(() => undefined);
      }
      // Колоду перечитываем, только когда терять нечего: недосмотренная
      // колода — это место, на котором человек остановился, и обновление
      // вернуло бы его к первой карточке.
      if (which === 'deck') {
        const d = deckRef.current;
        if (!d.loaded || d.index >= d.cards.length) void loadDeck(filtersRef.current);
      }
    },
    [reloadPoints, reloadSearches, reloadLikes, setProfile, loadDeck],
  );

  const toTab = useCallback(
    (next: Tab) => {
      setTab(next);
      setView('tab');
      refreshTab(next);
    },
    [refreshTab],
  );

  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState === 'visible' && view === 'tab') refreshTab(tab);
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => document.removeEventListener('visibilitychange', onVisible);
  }, [refreshTab, tab, view]);

  // Стабильная ссылка: форма подписывает её на родную кнопку «назад».
  const closeForm = useCallback(() => {
    setFormMode(null);
    setView('tab');
    // Форму бросили — ждать возврата к профилю больше нечему: иначе поиск,
    // сохранённый через неделю совсем по другому поводу, унёс бы туда.
    setLikeReturn(null);
  }, []);

  /**
   * Диплинк запуска: куда открыть приложение (§5 дизайна пушей).
   *
   * Сработавшая цель гасится: второй раз она увела бы человека с экрана, куда
   * он ушёл сам.
   *
   * Эффект живёт выше ранних возвратов оболочки и потому ходит только через
   * setState и стабильные колбэки — тела экранов ниже в этом рендере может и
   * не быть (draft, редактор анкеты).
   */
  useEffect(() => {
    if (!startTarget || state.kind !== 'ready') return;

    // Правка анкеты открывается при любом статусе: реджект возвращает
    // непроверенного в draft, а у одобренного статус не меняется (§5) —
    // экран правки один и тот же.
    if (startTarget.kind === 'photo' || startTarget.kind === 'about') {
      setEditTarget(startTarget.kind);
      setView('editProfile');
      setStartTarget(null);
      return;
    }
    // Ссылка посева никуда не ведёт — она только помечает первое касание, и
    // помечать его надо в том числе у пришедшего в draft: он и есть тот, кого
    // привёл посев. Отказ переживаем молча: трекинг не гейт на входе.
    if (startTarget.kind === 'source') {
      void postSource(startTarget.code).catch(() => undefined);
      setStartTarget(null);
      return;
    }
    // Остальные цели ведут в продукт, а незаполненная анкета его не показывает.
    if (state.profile.status === 'draft') {
      setStartTarget(null);
      return;
    }

    switch (startTarget.kind) {
      case 'point': {
        // Живая точка теперь одна — комната «к себе» (§2.2): её экран и
        // открываем. Отказ переживаем молча: точку могли закрыть или она ещё
        // на проверке, и приложение всё равно открывается, просто с подбора.
        void fetchPoint(startTarget.id)
          .then((point) => {
            if (point.type !== 'intent_home') return;
            setRoomError(null);
            setHomePoint(point.id);
            setView('homePoint');
          })
          .catch(() => undefined);
        break;
      }
      // Пуш «в твоей зоне ищут ещё» ведёт в подбор: отдельного экрана зоны
      // больше нет, а колода и так собрана по всем активным поискам (§2.1).
      case 'pool':
      case 'feed':
        toTab('deck');
        break;
      case 'match':
        setMatchId(startTarget.id);
        setView('match');
        break;
      case 'incoming':
        setIncomingError(null);
        toTab('likes');
        break;
      case 'mine':
        toTab('searches');
        break;
    }
    setStartTarget(null);
  }, [startTarget, state, toTab]);

  /**
   * Топ живых зон нужен ровно одному состоянию — подбору без поиска (§6).
   * Считается здесь, а не в экране: от него зависит запрос, а экран о
   * запросах не знает.
   */
  useEffect(() => {
    if (!deck.noSearch || topZones) return;
    void fetchTopZones(5)
      .then(setTopZones)
      .catch(() => {
        // Без топа состояние остаётся с вопросом и кнопкой «Найти» — хватает.
      });
  }, [deck.noSearch, topZones]);

  /**
   * Колода приезжает пачкой до 50 карточек, а `total` считает всех подошедших.
   *
   * Досмотрел пачку, а подошедших больше — дозапрашиваем следующую: иначе на
   * сотне совпавших человек упирался бы в «Пока все» после пятидесятого
   * свайпа, а шапка над этим писала бы «137 в твоих зонах». Загрузка обычная,
   * с начала: пропущенных и лайкнутых сервер уже исключил, и повторов не
   * будет. Циклиться не на чем — `total` считается той же выборкой, и вместе
   * с карточками он доходит до нуля.
   */
  const needsMore =
    deck.loaded &&
    !deck.noSearch &&
    !deck.error &&
    deck.index >= deck.cards.length &&
    deck.cards.length < deck.total;

  useEffect(() => {
    if (!needsMore) return;
    void loadDeck(filtersRef.current);
  }, [needsMore, loadDeck]);

  if (state.kind === 'loading') return <Centered>Секунду…</Centered>;
  if (state.kind === 'failed') return <Failure error={state.error} />;

  const { profile } = state;

  // Анкета не дозаполнена — весь остальной продукт подождёт. В draft можно
  // листать ленту, но входом по умолчанию должен быть онбординг (§5).
  // Исключение — вернувшийся в draft реджектом: анкета у него заполнена, ему
  // нужна одна правка, и держать его в онбординге значило бы прятать строку
  // «что не так» вместе со всем продуктом (§5, две ветки реджекта).
  // Открытый экран анкеты главнее: у чинящего фото реджект снимается самой
  // загрузкой нового, и без этой оговорки человека уносило бы в двенадцать
  // шагов ровно в тот момент, когда осталось нажать «Отправить на проверку».
  // Что именно отклонил модератор: плашка подбора и экран анкеты ведут в один
  // и тот же кусок правки, а фото главнее — без него человека не видно (§4).
  const fixTarget: 'photo' | 'about' | null =
    profile.photo_status === 'rejected'
      ? 'photo'
      : profile.about_status === 'rejected'
        ? 'about'
        : null;
  if (view !== 'editProfile' && ((profile.status === 'draft' && !fixTarget) || finishing)) {
    return (
      <OnboardingFlow
        profile={profile}
        onProfile={setProfile}
        onSubmitted={() => setFinishing(true)}
        onFinished={() => {
          setFinishing(false);
          setView('tab');
          setTab('deck');
        }}
      />
    );
  }

  const unreadLikes = incoming.length;
  const back = () => setView('tab');

  const active = (searches ?? []).filter((s) => s.status === 'active');
  const archived = (searches ?? []).filter((s) => s.status !== 'active');
  const hasNewInSearches = active.some((s) => s.new_count > 0);

  /**
   * «Сохранить и смотреть» (§3): просмотра без подписки не существует, поэтому
   * форма всегда сохраняет — и уводит в подбор.
   *
   * На зону с уже существующим активным поиском это правка, а не второй
   * поиск: дубль зоны съел бы слот и раздвоил счётчики (§2.1).
   */
  const saveSearch = async (form: Extract<FormMode, { kind: 'create' }>) => {
    if (form.busy) return;
    setFormMode({ ...form, busy: true, error: null, invalidFields: [] });
    const params = draftToParams(form.draft);
    try {
      const owned = (searches ?? []).find(
        (s) => s.status === 'active' && s.zone.id === params.zone_id,
      );
      if (owned) await updateSearch(owned.id, params);
      else await createSearch(params);
      await reloadSearches().catch(() => undefined);
      setFormMode(null);
      setTab('deck');
      setView('tab');
      await loadDeck(filters);
      // Поиск сохранён — «и лайк улетит» из подписи формы обязано быть
      // правдой: выстреливаем лайк сами и ведём человека к результату.
      if (likeReturn) {
        const target = likeReturn;
        setLikeReturn(null);
        try {
          const res = await postLike(target.userId, {
            zoneId: target.zoneId,
            pointId: target.pointId,
          });
          setLikesRemaining(res.budget.remaining);
          reloadLikes();
          if (res.matched && res.match_id) {
            openMatch(res.match_id);
            return;
          }
        } catch {
          // Отказ (лимит, анкета на проверке) расскажет профиль состоянием
          // кнопки — туда и ведём.
        }
        setPerson(target);
        setView('person');
      }
    } catch (e: unknown) {
      const err = asApiError(e);
      setFormMode({
        ...form,
        busy: false,
        error: err.message,
        invalidFields: err.code === 'invalid_field' ? err.fields : [],
      });
    }
  };

  /** Свайп влево: цель уходит из подбора на 7 дней (§2.1). Курсор — сразу. */
  const passCard = async (card: DeckCard) => {
    setDeck((d) => ({ ...d, index: d.index + 1, error: null }));
    try {
      if (card.kind === 'person') await passPerson(card.person.user_id);
      else await passPoint(card.room.id);
      // Счётчик пропущенных считает сервер: конец колоды обязан звать
      // «Показать пропущенных · N» с настоящим числом.
      setDeck((d) => ({ ...d, passed: d.passed + 1 }));
    } catch (e: unknown) {
      // Карточка уже улетела — возвращать её обратно значит спорить с жестом;
      // говорим словами сервера, а сама цель вернётся следующей колодой.
      setDeck((d) => ({ ...d, error: asApiError(e).message }));
    }
  };

  /**
   * Свайп вправо: дорогой лайк с контекстом карточки (§2.3).
   *
   * Курсор двигается только после ответа: отказ по лимиту или непроверенной
   * анкете не должен съедать карточку — человек вернётся к ней, когда слот
   * освободится. Отсюда и `true`/`false` в ответе: по нему экран решает,
   * улетела карточка совсем или возвращается на место.
   */
  const likeCard = async (card: DeckCard): Promise<boolean> => {
    const target =
      card.kind === 'person'
        ? { userId: card.person.user_id, zoneId: card.pair.zone.id }
        : { userId: card.room.author.user_id, pointId: card.room.id };
    setDeck((d) => ({ ...d, error: null }));
    try {
      const res = await postLike(target.userId, {
        zoneId: target.zoneId,
        pointId: target.pointId,
      });
      setLikesRemaining(res.budget.remaining);
      if (target.pointId) setRespondedPoints((list) => [...list, target.pointId]);
      reloadLikes();
      setDeck((d) => ({ ...d, index: d.index + 1 }));
      if (res.matched && res.match_id) openMatch(res.match_id);
      return true;
    } catch (e: unknown) {
      const err = asApiError(e);
      // Гонка «поиск умер, пока смотрел карточку» (§2.3): вместо отказа —
      // мини-форма с предзаполненной зоной, лайк уйдёт сам после сохранения.
      if (err.code === 'need_search') {
        askSearchForLike({ ...target, back: 'tab' }, err.message);
        return true;
      }
      // Слотов не осталось — точки под колодой обязаны это показать: своей
      // ручки у бюджета нет, а отказ по лимиту и есть его настоящий ноль.
      if (err.code === 'like_limit') setLikesRemaining(0);
      setDeck((d) => ({ ...d, error: err.message }));
      return false;
    }
  };

  /** «Показать пропущенных · N»: стирает все пропуски и собирает колоду заново. */
  const restorePassed = async () => {
    try {
      await clearPasses();
    } catch (e: unknown) {
      setDeck((d) => ({ ...d, error: asApiError(e).message }));
      return;
    }
    await loadDeck(filters);
  };

  const applyFilters = (next: DeckFilters) => {
    setFilters(next);
    saveDeckFilters(next);
    setFiltersOpen(false);
    void loadDeck(next);
  };

  /** «Позвать в чат»: диплинк на комнату уходит в чат (§6). */
  const sharePoint = (pointId: string) => openShare(pointShareUrl(pointId));

  /**
   * «Хочу эту комнату» с экрана комнаты (§2.2): дорогой лайк автору.
   *
   * Поиска этот лайк не требует — отношение несёт сама карточка-витрина, —
   * поэтому ветки need_search тут нет: отказать могут только лимит и
   * неодобренная анкета, и обоим отвечаем словами сервера.
   */
  const wantRoom = async (point: PointDTO) => {
    const author = point.author;
    if (!author || respondedPoints.includes(point.id)) return;
    // Кнопка отвечает до сервера, и она же держит второй тап: повторный лайк
    // сервер бы простил, но человеку нужен ответ сразу.
    setRespondedPoints((list) => [...list, point.id]);
    setRoomError(null);
    try {
      const res = await postLike(author.user_id, { pointId: point.id });
      setLikesRemaining(res.budget.remaining);
      reloadLikes();
      if (res.matched && res.match_id) openMatch(res.match_id);
    } catch (e: unknown) {
      const err = asApiError(e);
      setRespondedPoints((list) => list.filter((id) => id !== point.id));
      if (err.code === 'like_limit') setLikesRemaining(0);
      setRoomError(err.message);
    }
  };

  /** Закрытие своей точки. Причин у неё нет — в отличие от поиска (§3). */
  const closeMyPoint = async (pointId: string) => {
    setSearchesError(null);
    try {
      await closePoint(pointId);
      reloadPoints();
    } catch (e: unknown) {
      setSearchesError(asApiError(e).message);
    }
  };

  const openMatch = (id: string) => {
    setMatchId(id);
    setView('match');
  };

  /**
   * Экран анкеты. Цель задаётся, когда ведут в конкретную правку: реджект
   * зовёт чинить фото или «о себе», а не проходить анкету заново (§5).
   */
  const openEditor = (target?: 'photo' | 'about') => {
    setEditTarget(target ?? null);
    setView('editProfile');
  };

  /** Куда ведёт тап в строку статуса профиля. «Заполни анкету» — в общий список. */
  const fixProfile = (target: FixTarget) =>
    openEditor(target === 'draft' ? undefined : target);

  /**
   * «Ответить» из «Лайков»: тот же дорогой лайк, но бесплатный (§2.3).
   *
   * Контекст берётся из самого входящего — пул, в котором пересеклись, а не
   * тот, где мы сейчас стоим. Взаимность гарантирована (в списке ровно те, кто
   * лайкнул), но ветвимся всё равно по ответу сервера: мэтч рождается там.
   */
  const replyLike = async (userId: string) => {
    if (replying) return;
    setReplying(userId);
    setIncomingError(null);
    try {
      const like = incoming.find((l) => l.user_id === userId);
      const res = await postLike(userId, { zoneId: like?.zone?.id, pointId: like?.point?.id });
      setLikesRemaining(res.budget.remaining);
      reloadLikes();
      if (res.matched && res.match_id) openMatch(res.match_id);
    } catch (e: unknown) {
      // 403 «анкета на проверке» — единственный ожидаемый отказ ответа:
      // лимита он не тратит и поиска не требует. Показываем слова сервера.
      setIncomingError(asApiError(e).message);
    } finally {
      setReplying(null);
    }
  };

  const openPerson = (target: PersonTarget) => {
    setPerson(target);
    setView('person');
  };

  /** Лайк без поиска: §2.3 просит мини-форму «укажи, где ищешь», а не отказ. */
  const askSearchForLike = (target: PersonTarget, serverMessage: string) => {
    setLikeReturn(target);
    // Зона контекста предзаполнена: там они и пересеклись, и поиск скорее
    // всего нужен именно на неё.
    const draft = target.zoneId ? { ...EMPTY_DRAFT, zoneId: target.zoneId } : EMPTY_DRAFT;
    setFormMode({ kind: 'create', draft, busy: false, error: serverMessage, invalidFields: [] });
    setTab('deck');
    setView('searchForm');
  };

  const openEditForm = (id: string) => {
    const s = active.find((x) => x.id === id) ?? (searches ?? []).find((x) => x.id === id);
    if (!s) return;
    setSearchesError(null);
    setFormMode({
      kind: 'edit',
      searchId: s.id,
      zoneName: s.zone.name,
      draft: paramsToDraft(s),
      busy: false,
      error: null,
      invalidFields: [],
    });
    setView('searchForm');
  };

  const saveEdit = async (form: Extract<FormMode, { kind: 'edit' }>) => {
    setFormMode({ ...form, busy: true, error: null, invalidFields: [] });
    try {
      await updateSearch(form.searchId, draftToParams(form.draft));
      await reloadSearches();
      setFormMode(null);
      setTab('searches');
      setView('tab');
    } catch (e: unknown) {
      const err = asApiError(e);
      setFormMode({
        ...form,
        busy: false,
        error:
          err.code === 'search_closed'
            ? 'Поиск закрыт — сначала верни его из архива'
            : err.message,
        invalidFields: err.code === 'invalid_field' ? err.fields : [],
      });
    }
  };

  const confirmClose = async (reason: CloseReason) => {
    const id = closing;
    setClosing(null);
    if (!id) return;
    setSearchesError(null);
    try {
      await closeSearch(id, reason);
      await reloadSearches();
    } catch (e: unknown) {
      setSearchesError(asApiError(e).message);
    }
  };

  const reactivate = async (id: string) => {
    setSearchesError(null);
    try {
      await reactivateSearch(id);
      await reloadSearches();
    } catch (e: unknown) {
      // «поиски заняты 3 из 3» — единственное русское сообщение сервера,
      // показываем как есть.
      setSearchesError(asApiError(e).message);
    }
  };

  const savedSearches: SavedSearch[] = active.map((s) => ({
    id: s.id,
    zone: s.zone.name,
    since: `ищу с ${dayMonth(s.created_at)}`,
    // Зона уже в заголовке карточки — в чипах она была бы повтором.
    params: paramChips(s),
    peopleCount: s.people,
    fullMatchCount: s.full_match,
    newCount: s.new_count || undefined,
  }));

  const archivedSearches: ArchivedSearch[] = archived.map((s) => ({
    id: s.id,
    zone: s.zone.name,
    params: paramChips(s),
    reason: closeReasonLabel(s.close_reason),
  }));

  const tabScreen = () => {
    switch (tab) {
      case 'deck': {
        const card = deck.cards[deck.index];
        return (
          <DeckScreen
            card={card}
            total={deck.total}
            passed={deck.passed}
            // Пока едет следующая пачка — «Секунду…», а не «Пока все»: конец
            // подбора наступает только когда кончились и карточки, и счётчик.
            loading={(!deck.loaded && !deck.error) || needsMore}
            noSearch={deck.noSearch}
            ended={deck.loaded && !deck.noSearch && !card && !needsMore}
            error={deck.error}
            // Бюджет своей ручки не имеет: до первого лайка в сессии
            // показываем полный запас (§2.3, окно скользящее).
            likesRemaining={likesRemaining ?? LIKE_SLOTS}
            pending={profile.status === 'pending'}
            rejected={fixTarget}
            likeLocked={profile.status !== 'active'}
            filtersActive={hasDeckFilters(filters)}
            topZones={topZones ?? undefined}
            slotsUsed={active.length}
            slotsTotal={SEARCH_SLOTS}
            onOpenFilters={() => setFiltersOpen(true)}
            onPass={(c) => void passCard(c)}
            onLike={likeCard}
            onOpenCard={(c) => {
              if (c.kind === 'room') {
                // Прошлый отказ отклика уже рассказан — встречать им на входе
                // в другую комнату нечего.
                setRoomError(null);
                setHomePoint(c.room.id);
                setView('homePoint');
                return;
              }
              openPerson({ userId: c.person.user_id, zoneId: c.pair.zone.id, back: 'tab' });
            }}
            // Анкета автора комнаты открывается только с точкой: отношение по
            // ней сервер проверяет по этому параметру, без него — 404 (§2.3).
            onOpenAuthor={(room) =>
              openPerson({ userId: room.author.user_id, pointId: room.id, back: 'tab' })
            }
            onFixProfile={() => openEditor(fixTarget ?? undefined)}
            onFind={() => openSearchForm(EMPTY_DRAFT)}
            onPickZone={(zoneId) => openSearchForm({ ...EMPTY_DRAFT, zoneId })}
            onRestorePassed={() => void restorePassed()}
            onAddSearch={() => openSearchForm(EMPTY_DRAFT)}
          />
        );
      }
      // Плитки «Лайков» — задача шага 11.5; пока вкладку держит прежний
      // список входящих, чтобы петля лайк → мэтч не ломалась на пересадке.
      case 'likes':
        return (
          <IncomingLikesScreen
            likes={incoming.map(toIncomingRow)}
            replyingId={replying}
            error={incomingError}
            onBack={() => toTab('deck')}
            onOpenProfile={(id) => {
              const like = incoming.find((l) => l.user_id === id);
              openPerson({
                userId: id,
                zoneId: like?.point ? undefined : like?.zone?.id,
                pointId: like?.point?.id,
                back: 'tab',
              });
            }}
            onReply={(id) => void replyLike(id)}
            onToFeed={() => toTab('deck')}
          />
        );
      case 'searches':
        return (
          <MySearchesScreen
            searches={savedSearches}
            searchSlotsTotal={SEARCH_SLOTS}
            archived={archivedSearches}
            error={searchesError}
            homes={myPoints.created.filter((p) => p.type === 'intent_home')}
            homeSlotsTotal={2}
            // Отдельного экрана зоны больше нет: люди живут в подборе (§6).
            onOpenPool={() => toTab('deck')}
            onEditSearch={openEditForm}
            onCloseSearch={setClosing}
            onAddSearch={() => openSearchForm(EMPTY_DRAFT)}
            onReactivate={(id) => void reactivate(id)}
            onSharePoint={sharePoint}
            onClosePoint={(id) => void closeMyPoint(id)}
            onCreateHomePoint={() => setView('homeForm')}
          />
        );
      case 'profile':
        return (
          <MyProfileScreen
            name={profile.name}
            age={profile.age ?? 0}
            occupation={occupationLabel(profile.occupation)}
            photoUrl={ownPhotoSrc(profile.photo_url, photoStamp)}
            // Статус и вердикты по фото и «о себе» едут врозь: сворачивать их
            // в один — значит потерять петлю реджекта (§5).
            status={profile.status}
            photoStatus={profile.photo_status}
            aboutStatus={profile.about_status}
            emailKind={profile.occupation === 'study' ? 'Учебная почта' : 'Рабочая почта'}
            incomingLikes={unreadLikes}
            matches={matches.map(toMyMatch)}
            onEdit={() => openEditor()}
            onFix={fixProfile}
            onOpenIncomingLikes={() => toTab('likes')}
            onOpenMatch={openMatch}
          />
        );
    }
  };

  const screen = () => {
    switch (view) {
      case 'searchForm': {
        if (!formMode) return tabScreen();
        if (formMode.kind === 'edit') {
          const form = formMode;
          return (
            <SearchParamsScreen
              mode="edit"
              zones={zones}
              zoneNameFallback={form.zoneName}
              value={form.draft}
              onChange={(draft) =>
                setFormMode((f) => (f?.kind === 'edit' ? { ...f, draft } : f))
              }
              onSubmit={() => void saveEdit(form)}
              onBack={closeForm}
              busy={form.busy}
              error={form.error}
              invalidFields={form.invalidFields}
            />
          );
        }
        const form = formMode;
        return (
          <SearchParamsScreen
            mode="create"
            zones={zones}
            updatesSearch={(searches ?? []).some(
              (s) => s.status === 'active' && s.zone.id === form.draft.zoneId,
            )}
            value={form.draft}
            onChange={(draft) =>
              setFormMode((f) => (f?.kind === 'create' ? { ...f, draft } : f))
            }
            onSubmit={() => {
              if (isDraftReady(form.draft)) void saveSearch(form);
            }}
            onBack={closeForm}
            busy={form.busy}
            error={form.error}
            invalidFields={form.invalidFields}
          />
        );
      }
      case 'person': {
        if (!person) return tabScreen();
        const target = person;
        return (
          <PersonProfileView
            userId={target.userId}
            zoneId={target.zoneId}
            pointId={target.pointId}
            onBack={() => setView(target.back)}
            onLiked={(remaining) => {
              setLikesRemaining(remaining);
              // Лайк с точки-контекста — тот же отклик: экран комнаты обязан
              // про него знать.
              const responded = target.pointId;
              if (responded) setRespondedPoints((list) => [...list, responded]);
              reloadLikes();
            }}
            onMatched={openMatch}
            onNeedSearch={(serverMessage) => askSearchForLike(target, serverMessage)}
            // Заблокированный стал невидимым: профиль по нему ответил бы 404,
            // а списки, где он стоял, обязаны перечитаться (§5). Комната
            // уходит вместе с автором, поэтому назад в неё не возвращаем:
            // экран показал бы отказ сервера вместо результата блокировки.
            onBlocked={() => {
              if (target.back === 'homePoint') toTab('deck');
              else setView(target.back);
              reloadLikes();
              reloadPoints();
              void loadDeck(filters);
            }}
          />
        );
      }
      case 'match':
        if (!matchId) return tabScreen();
        return (
          // Ключ — id мэтча: следующий должен собрать экран заново, а не
          // донести на нём саммари предыдущего.
          <MatchView
            key={matchId}
            matchId={matchId}
            onBack={back}
            // Мэтча больше нет (удалили или заблокировали пира): экран по нему
            // ответил бы 404 — уводим туда, откуда пришли, обычно в «Мои
            // мэтчи». Перечитывается всё, что мэтч прятал: сам список,
            // входящие (лайк второго перестал быть входящим вместе с ним) и
            // колода — пара снова видит друг друга (§2.3), а после блока,
            // наоборот, пропадает отовсюду (§5).
            onRemoved={() => {
              back();
              reloadLikes();
              reloadPoints();
              void reloadSearches().catch(() => undefined);
              void loadDeck(filters);
            }}
            onToFeed={() => toTab('deck')}
          />
        );
      case 'homePoint': {
        if (!homePoint) return tabScreen();
        const pointId = homePoint;
        return (
          <HomePointView
            key={pointId}
            pointId={pointId}
            likesRemaining={likesRemaining ?? undefined}
            myUserId={profile.id}
            respondedPoints={respondedPoints}
            matchedAuthors={matches.map((m) => m.peer.user_id)}
            error={roomError}
            onBack={back}
            onWantRoom={(point) => void wantRoom(point)}
            onOpenProfile={(point) =>
              point.author &&
              openPerson({ userId: point.author.user_id, pointId: point.id, back: 'homePoint' })
            }
          />
        );
      }
      case 'homeForm':
        return (
          <HomePointFormScreen
            zones={zones}
            hasLivePoint={myPoints.created.some((p) => p.stage === 'live')}
            onBack={back}
            // У «к себе» пула нет (§2.2), и «ты первый в пуле» ему сказать
            // нечего: точка сразу видна в своём блоке со статусом проверки.
            onCreated={() => {
              reloadPoints();
              toTab('searches');
            }}
          />
        );
      case 'editProfile':
        return (
          <ProfileEditScreen
            profile={profile}
            autoOpen={editTarget ?? undefined}
            photoStamp={photoStamp}
            onProfile={setProfile}
            onPhotoReplaced={() => setPhotoStamp(Date.now())}
            // Назад — на таб «Профиль», а не «в никуда»: сюда приходят и по
            // диплинку пуша, с любого таба. Перечитываем себя: строка статуса
            // на профиле обязана знать про только что отправленное фото.
            onBack={() => {
              setEditTarget(null);
              toTab('profile');
            }}
          />
        );
      default:
        return tabScreen();
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', minHeight: '100vh' }}>
      {/* Колонка, а не просто блок: экранам с прижатым низом (кнопки подбора)
          нужна определённая высота, а не «сколько получилось по содержимому». */}
      <div style={{ flex: '1 1 auto', minHeight: 0, display: 'flex', flexDirection: 'column' }}>
        {screen()}
      </div>
      {/* Таб-бар рисует только оболочка: у экранов своего нет, иначе на табах
          их два и внутренний — мёртвый, без onChange. */}
      {view === 'tab' && (
        <TabBar
          active={tab}
          unreadLikes={unreadLikes}
          hasNewInSearches={hasNewInSearches}
          onChange={toTab}
        />
      )}

      {filtersOpen && (
        <FiltersSheet
          value={filters}
          total={deck.total}
          onApply={applyFilters}
          onClose={() => setFiltersOpen(false)}
        />
      )}

      {closing && (
        <CloseSearchSheet onPick={(r) => void confirmClose(r)} onCancel={() => setClosing(null)} />
      )}
    </div>
  );
}

/**
 * Открывает шеринг в чат.
 *
 * Родным вызовом — но только в настоящем клиенте: под моком разработки
 * isAvailable() отвечает «да», вызов уходит в перехваченный postMessage, и по
 * кнопке не происходило бы ничего (то же правило, что у контакта мэтча).
 */
function openShare(url: string): void {
  if (launchedByTelegram() && openTelegramLink.isAvailable()) {
    openTelegramLink(url);
    return;
  }
  window.open(url, '_blank', 'noopener');
}

/**
 * Карточка входящего лайка (§6).
 *
 * Контекста может не быть: зона выпала из справочника или лайк пришёл ответом,
 * у которого зоны нет. Тогда мини-карточки просто нет — выдумывать ей место
 * пересечения нельзя.
 *
 * Метка точки приезжает готовой и уже от лица адресата: отклик на комнату
 * сервер подписывает «на твою комнату» (§2.2), и переписывать её здесь нечем.
 * Тапабельной она больше не бывает: отдельных экранов зоны и квартиры нет.
 */
function toIncomingRow(l: IncomingLikeDTO): IncomingLike {
  const context = l.point
    ? { label: l.point.label }
    : l.zone
      ? { label: `ищет ${l.zone.name_in}` }
      : undefined;
  return {
    id: l.user_id,
    name: l.name,
    age: l.age,
    photoUrl: l.photo_url || undefined,
    habits: habitsLine(l),
    when: relativeDay(l.at),
    notSearching: l.not_searching,
    context,
  };
}

/** Строка «Моих мэтчей»: контакты остаются на экране самого мэтча (§3). */
function toMyMatch(m: MatchDTO): MyMatch {
  return {
    id: m.match_id,
    name: m.peer.name,
    age: m.peer.age,
    photoUrl: m.peer.photo_url || undefined,
    context: m.zone?.name,
    when: `мэтч ${relativeDay(m.at)}`,
  };
}

/** Любая неудача вызова — в ApiError: экраны ветвятся по code, а не по тексту. */
function asApiError(e: unknown): ApiError {
  return e instanceof ApiError ? e : new ApiError(0, 'network', String(e));
}

function Centered({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'grid',
        placeItems: 'center',
        padding: 24,
        background: 'var(--app-bg)',
        color: 'var(--tg-theme-hint-color)',
        fontSize: 15,
      }}
    >
      {children}
    </div>
  );
}

function Failure({ error }: { error: ApiError }) {
  // 401 в разработке почти всегда значит, что мок initData подписан не тем
  // токеном, — так и скажем, вместо голого «unauthorized».
  const banned = error.status === 403;
  const hint = banned
    ? 'Аккаунт заблокирован'
    : error.status === 401
      ? 'Проверь BOT_TOKEN у api и VITE_MOCK_INIT_DATA в web/.env.local — см. README.'
      : error.message;

  return (
    <Centered>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8, textAlign: 'center' }}>
        <span style={{ fontSize: 18, fontWeight: 640, color: 'var(--tg-theme-text-color)' }}>
          {banned ? 'Доступ закрыт' : 'Не получилось войти'}
        </span>
        <span>{hint}</span>
        <span style={{ fontSize: 12 }}>
          {error.code}
          {error.status ? ` · HTTP ${error.status}` : ''}
        </span>
      </div>
    </Centered>
  );
}
