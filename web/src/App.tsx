/* Оболочка приложения: тема, табы §6 и переходы между экранами.

   Живыми данными питаются анкета (§12 шаг 2), зоны, поиски, лента и зонный
   пул (§12 шаг 3), профиль человека, лайки и мэтчи (§12 шаг 4), точки, их
   формы, квартирные пулы и отклики (§12 шаг 5). DEMO-экранов не осталось.

   Здесь же живёт состояние ленты — намеренно выше экранов: переключение табов
   не должно ронять выдачу и перезапрашивать её. И здесь единственный владелец
   списка поисков: после любой мутации он перечитывается с сервера целиком,
   локальных «оптимистичных» правок нет — счётчики и номера слотов считает
   только сервер. Входящие лайки и мэтчи — по тому же правилу: бейдж на табе
   считается по одному списку, который перечитывается после каждого лайка,
   откуда бы его ни поставили. */
import { useCallback, useEffect, useRef, useState } from 'react';
import { isMiniAppDark, openTelegramLink, useSignal } from '@telegram-apps/sdk-react';

import {
  ApiError,
  closePoint,
  closeSearch,
  createSearch,
  enterPointPool,
  leavePointPool,
  fetchFeed,
  fetchIncoming,
  fetchMatches,
  fetchMe,
  fetchPoint,
  fetchPointsMine,
  fetchSearches,
  fetchTopZones,
  fetchZones,
  pointShareUrl,
  postLike,
  postSource,
  reactivateSearch,
  restorePoint,
  startParam,
  updateSearch,
  type CloseReason,
  type FeedPromo,
  type FeedResponse,
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
  money,
  occupationLabel,
  paramChips,
  plural,
  pointTitle,
  pricePerPerson,
  relativeDay,
} from './api/format';
import { launchedByTelegram } from './telegram/mockEnv';
import { TabBar, type Tab } from './ui/TabBar';
import type { PromoState } from './ui/PoolPromoBlock';
import { OnboardingFlow } from './screens/OnboardingFlow';
import { FeedScreen } from './screens/FeedScreen';
import { FeedEntryScreen } from './screens/FeedEntryScreen';
import {
  draftToParams,
  EMPTY_DRAFT,
  isDraftReady,
  paramsToDraft,
  SearchParamsScreen,
  type SearchDraft,
} from './screens/SearchParamsScreen';
import { ZonePoolView } from './screens/ZonePoolView';
import { PersonProfileView } from './screens/PersonProfileView';
import { MatchView } from './screens/MatchView';
import { FlatPoolView } from './screens/FlatPoolView';
import { HomePointView } from './screens/HomePointView';
import { IncomingLikesScreen, type IncomingLike } from './screens/IncomingLikesScreen';
import { MySearchesScreen, type ArchivedSearch, type SavedSearch } from './screens/MySearchesScreen';
import { MyProfileScreen, type FixTarget, type MyMatch } from './screens/MyProfileScreen';
import { ProfileEditScreen } from './screens/ProfileEditScreen';
import { FlatLinkFormScreen, HomePointFormScreen } from './screens/PointFormScreens';
import { PointCreatedScreen } from './screens/PointCreatedScreen';
import { CloseSearchSheet } from './screens/EdgeStates';

type View =
  | 'tab'
  | 'searchForm'
  | 'zonePool'
  | 'person'
  | 'match'
  | 'flatPool'
  | 'homePoint'
  | 'incoming'
  | 'flatForm'
  | 'homeForm'
  | 'pointCreated'
  | 'editProfile';

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
 * Состояние ленты.
 *
 * `mode` — что показывает таб «Лента»: экран входа, форму параметров или
 * выдачу. `data` переживает уход на другие табы, поэтому возврат в ленту
 * ничего не перезапрашивает.
 */
type FeedState = {
  mode: 'entry' | 'form' | 'results';
  draft: SearchDraft;
  /**
   * Черновик показанной выдачи, снятый на время открытой формы просмотра.
   *
   * Форма правит `draft` на каждую нажатую цифру, а брошенная форма не должна
   * оставлять после себя чужие параметры: чипы над выдачей и кнопка «Войти в
   * пул» обязаны описывать именно то, что на экране. У правки поиска своя
   * копия черновика, а у просмотра — этот снимок.
   */
  shownDraft: SearchDraft | null;
  data: FeedResponse | null;
  busy: boolean;
  error: string | null;
  invalidFields: string[];
  /**
   * Сервер отказал по слотам. Отдельный флаг, а не вывод из `data`: отказ
   * приходит на POST, а цифры в ленте — из последнего GET, и до его обновления
   * блок иначе продолжал бы предлагать кнопку, которая уже не сработает.
   */
  slotsFull: boolean;
};

/**
 * Открытая форма параметров.
 *
 * У правки свой черновик: она не должна переписывать фильтры просмотра —
 * иначе после «Сохранить» чипы над выдачей рассказывали бы про чужой поиск.
 */
type FormMode =
  | { kind: 'look' }
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

const EMPTY_FEED: FeedState = {
  mode: 'entry',
  draft: EMPTY_DRAFT,
  shownDraft: null,
  data: null,
  busy: false,
  error: null,
  invalidFields: [],
  slotsFull: false,
};

export function App() {
  const isDark = useSignal(isMiniAppDark);
  const [state, setState] = useState<Loading>({ kind: 'loading' });
  const [tab, setTab] = useState<Tab>('feed');
  const [view, setView] = useState<View>('tab');
  // Держит финальную развилку §4 на экране после отправки: статус к этому
  // моменту уже не draft, и без флага оболочка увела бы в ленту сразу.
  const [finishing, setFinishing] = useState(false);
  /**
   * Что открыть на экране анкеты сразу. Пусто — просто список.
   *
   * Правку зовут точечно: вердикт модератора отклоняет фото или «о себе», и
   * человека надо привести ровно туда, а не прогонять анкету заново (§5).
   */
  const [editTarget, setEditTarget] = useState<'photo' | 'about' | null>(null);
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
   * Мои точки и чужие пулы, в которые я вошёл: блоки 2–3 «Моих поисков».
   *
   * Как и поиски, живут выше экранов и перечитываются целиком после каждой
   * мутации — счётчики пулов и статусы модерации считает только сервер.
   */
  const [myPoints, setMyPoints] = useState<MyPointsResponse>({ created: [], entered: [] });
  /** Только что созданная точка — её показывает экран успеха. */
  const [createdPoint, setCreatedPoint] = useState<PointDTO | null>(null);
  const [feed, setFeed] = useState<FeedState>(EMPTY_FEED);
  const [formMode, setFormMode] = useState<FormMode | null>(null);
  const [poolZone, setPoolZone] = useState<string | null>(null);
  /** Открытая квартира: её карточка и пул грузит FlatPoolView. */
  const [flatPoint, setFlatPoint] = useState<string | null>(null);
  /** Открытая карточка «к себе»: пула у неё нет, только витрина и отклик (§2.2). */
  const [homePoint, setHomePoint] = useState<string | null>(null);
  /**
   * «Где уже ищут» для пустой ленты (§6).
   *
   * Грузится один раз и только когда пусто: топ зон одинаков для всей выдачи,
   * а тратить запрос на каждую непустую ленту незачем.
   */
  const [topZones, setTopZones] = useState<TopZoneDTO[] | null>(null);
  /**
   * Остаток дорогих лайков для подписи на карточке «к себе».
   *
   * Своей ручки у бюджета нет: он приезжает ответом на лайк и с профилем
   * человека, поэтому до первого лайка в сессии на ленте остаток не пишем —
   * выдуманная цифра хуже её отсутствия.
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

  /** Перечитывает блоки 2–3 «Моих поисков». Отказ переживаем молча. */
  const reloadPoints = useCallback(() => {
    void fetchPointsMine()
      .then(setMyPoints)
      .catch(() => undefined);
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

        // Список поисков решает, с чего начинается лента: с выдачи активного
        // поиска или с экрана входа (§6 шаг 1).
        void fetchSearches()
          .then((list) => {
            if (cancelled) return;
            setSearches(list);
            // Активные идут первыми и внутри — новые первыми, так что первый
            // активный и есть самый свежий.
            const active = list.find((s) => s.status === 'active');
            if (!active) return;
            const draft = paramsToDraft(active);
            setFeed((f) => ({ ...f, draft }));
            // На 'results' переключаемся вместе с данными, а не заранее: иначе
            // на входе мигает экран «С кем и где будешь жить?».
            void fetchFeed(draftToParams(draft))
              .then((data) => {
                if (!cancelled) setFeed((f) => ({ ...f, mode: 'results', data }));
              })
              .catch(() => {
                // Выдача не приехала — остаёмся на экране входа.
              });
          })
          .catch(() => {
            // Лента остаётся на экране входа — оттуда всё доступно.
          });

        // Бейдж входящих на табе «Профиль» нужен с первого экрана — иначе о
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
  }, [reloadLikes, reloadPoints]);

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
    // Слот освободился (закрыли поиск) — снимаем отказ, иначе промо-блок
    // продолжал бы врать «поиски заняты» до следующего запроса ленты.
    setFeed((f) => {
      const total = f.data?.promo.slots_total ?? 3;
      const used = list.filter((s) => s.status === 'active').length;
      return f.slotsFull && used < total ? { ...f, slotsFull: false } : f;
    });
    return list;
  }, []);

  /** Форма параметров просмотра: «Найти» с входа, «Изменить» с выдачи, диплинк пула. */
  const openLookForm = useCallback((draft: SearchDraft) => {
    setFeed((f) => ({
      ...f,
      mode: 'form',
      draft,
      // Снимок на случай, если форму бросят: «+ поиск» и «Найти» приходят с
      // пустым черновиком, а выдача на экране осталась прежней.
      shownDraft: f.draft,
      error: null,
      invalidFields: [],
    }));
    setFormMode({ kind: 'look' });
    setTab('feed');
    setView('searchForm');
  }, []);

  const feedRef = useRef(feed);
  feedRef.current = feed;

  /** Перечитывает выдачу по текущим параметрам просмотра; входа и формы не трогает. */
  const refreshFeed = useCallback(async () => {
    const f = feedRef.current;
    if (!f.data) return;
    const data = await fetchFeed(draftToParams(f.draft)).catch(() => null);
    if (data) setFeed((s) => ({ ...s, data }));
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
      if (which === 'profile') {
        reloadLikes();
        void fetchMe()
          .then(setProfile)
          .catch(() => undefined);
      }
      if (which === 'feed') void refreshFeed();
    },
    [reloadPoints, reloadSearches, reloadLikes, setProfile, refreshFeed],
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
    setFeed((f) => ({
      ...f,
      // Форму бросили — возвращаем черновик показанной выдачи. Иначе чипы
      // рассказывали бы про ненабранные параметры, а «Войти в пул» молча
      // сохранил бы поиск, которого человек не видел.
      draft: f.shownDraft ?? f.draft,
      shownDraft: null,
      mode: f.data ? 'results' : 'entry',
      busy: false,
      error: null,
      invalidFields: [],
    }));
  }, []);

  /**
   * Диплинк запуска: куда открыть приложение (§5 дизайна пушей).
   *
   * Переход откладывается до данных, от которых он зависит: пул зоны ждёт
   * списка поисков — без него не решить, свой ли это пул. Сработавшая цель
   * гасится, второй раз она увела бы человека с экрана, куда он ушёл сам.
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
        // Тип решает, куда вести: у квартиры экран с пулом, у «к себе» — та же
        // карточка, что в ленте, без пула (§2.2). Отказ переживаем молча —
        // точку могли закрыть или она ещё на проверке, и приложение всё равно
        // открывается, просто с обычного входа.
        const pointId = startTarget.id;
        void fetchPoint(pointId)
          .then((point) => {
            if (point.type === 'flat') {
              setFlatPoint(point.id);
              setView('flatPool');
              return;
            }
            setFeed((f) => ({ ...f, error: null }));
            setHomePoint(point.id);
            setView('homePoint');
          })
          .catch(() => undefined);
        break;
      }
      case 'pool': {
        // Пул отдаётся только со своим активным поиском этой зоны (§2.3):
        // ждём список, иначе решение было бы гаданием.
        if (!searches) return;
        const zoneId = startTarget.zoneId;
        if (searches.some((s) => s.status === 'active' && s.zone.id === zoneId)) {
          setPoolZone(zoneId);
          setView('zonePool');
          break;
        }
        // Поиска на эту зону больше нет (закрыл, пока пуш шёл) — ведём в ленту
        // с предзаполненной зоной: отказом на кнопку пуша отвечать нечем.
        openLookForm({ ...EMPTY_DRAFT, zoneId });
        break;
      }
      case 'match':
        setMatchId(startTarget.id);
        setView('match');
        break;
      case 'incoming':
        setIncomingError(null);
        setView('incoming');
        break;
      case 'mine':
        toTab('searches');
        break;
      case 'feed':
        toTab('feed');
        break;
    }
    setStartTarget(null);
  }, [startTarget, state, searches, openLookForm, toTab]);

  /**
   * Пустая зона (§6): ни людей в промо, ни точек — тогда и только тогда ленте
   * нужен топ живых зон. Считается прямо здесь, а не в экране: от него зависит
   * запрос, а экран о запросах не знает.
   */
  const emptyZone =
    feed.mode === 'results' &&
    feed.data !== null &&
    feed.data.points.length === 0 &&
    feed.data.promo.people === 0;

  useEffect(() => {
    if (!emptyZone || topZones) return;
    void fetchTopZones(5)
      .then(setTopZones)
      .catch(() => {
        // Без топа пустая лента остаётся с промо-блоком — этого хватает.
      });
  }, [emptyZone, topZones]);

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
  const rejected = profile.photo_status === 'rejected' || profile.about_status === 'rejected';
  if (view !== 'editProfile' && ((profile.status === 'draft' && !rejected) || finishing)) {
    return (
      <OnboardingFlow
        profile={profile}
        onProfile={setProfile}
        onSubmitted={() => setFinishing(true)}
        onFinished={() => {
          setFinishing(false);
          setView('tab');
          setTab('feed');
        }}
      />
    );
  }

  const unreadLikes = incoming.length;
  const back = () => setView('tab');

  const active = (searches ?? []).filter((s) => s.status === 'active');
  const archived = (searches ?? []).filter((s) => s.status !== 'active');
  const hasNewInSearches = active.some((s) => s.new_count > 0);

  /** Перечитывает выдачу текущих параметров: счётчики пулов считает сервер. */
  /**
   * «Интересно — войти в пул» (§2.2): вход бесплатный и в один тап, поэтому
   * карточка меняется сразу — держать её мёртвой на время запроса не за что.
   * Отказ снимает отметку и говорит словами сервера.
   */
  const enterFlatPool = async (id: string) => {
    setFeed((f) => ({ ...f, data: withPoolMark(f.data, id, true), error: null }));
    try {
      await enterPointPool(id);
      reloadPoints();
    } catch (e: unknown) {
      setFeed((f) => ({
        ...f,
        data: withPoolMark(f.data, id, false),
        error: asApiError(e).message,
      }));
    }
  };

  /** «Показать» — просмотр выдачи. Поиск этим НЕ создаётся (§3). */
  const showFeed = async (draft: SearchDraft) => {
    setFeed((f) => ({ ...f, busy: true, error: null, invalidFields: [] }));
    try {
      const params = draftToParams(draft);
      // На зону с активным поиском форма — редактор подписки, не просмотр:
      // иначе счётчик считался бы по форме, пул — по подписке, и они врали
      // бы друг другу (§6, решение 2026-08-11).
      const owned = (searches ?? []).find((s) => s.status === 'active' && s.zone.id === params.zone_id);
      if (owned) {
        await updateSearch(owned.id, params);
        void reloadSearches().catch(() => undefined);
      }
      const data = await fetchFeed(params);
      setFeed((f) => ({
        ...f,
        mode: 'results',
        data,
        // Показали — этот черновик и есть теперь черновик выдачи, откатывать
        // больше не к чему.
        shownDraft: null,
        busy: false,
        error: null,
        invalidFields: [],
        slotsFull: false,
      }));
      setFormMode(null);
      setTab('feed');
      setView('tab');
    } catch (e: unknown) {
      const err = asApiError(e);
      setFeed((f) => ({
        ...f,
        busy: false,
        error: err.message,
        invalidFields: err.code === 'invalid_field' ? err.fields : [],
      }));
    }
  };

  /** «Войти в пул {зоны} — сохранить поиск»: единственное место подписки (§6). */
  const joinPool = async () => {
    // Кнопка внутри блока не гаснет на время запроса, а второй тап создал бы
    // второй поиск и съел бы ещё один слот.
    if (feed.busy) return;
    const params = draftToParams(feed.draft);
    setFeed((f) => ({ ...f, busy: true, error: null }));
    try {
      await createSearch(params);
      // Счётчики и номера слотов — серверные, поэтому список перечитываем
      // целиком, а ленту следом: в ней появится my_search_id, и блок позеленеет.
      await reloadSearches();
      const data = await fetchFeed(params);
      setFeed((f) => ({ ...f, data, busy: false, error: null, slotsFull: false }));
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
      if (err.code === 'search_limit') {
        await reloadSearches().catch(() => undefined);
        const data = await fetchFeed(params).catch(() => null);
        setFeed((f) => ({ ...f, data: data ?? f.data, busy: false, error: null, slotsFull: true }));
        return;
      }
      if (err.code === 'invalid_field') {
        setFeed((f) => ({
          ...f,
          mode: 'form',
          // Форма открылась не сама — снимок нужен ей так же: правки в ней
          // могут кончиться кнопкой «назад».
          shownDraft: f.shownDraft ?? f.draft,
          busy: false,
          error: err.message,
          invalidFields: err.fields,
        }));
        setFormMode({ kind: 'look' });
        setView('searchForm');
        return;
      }
      setFeed((f) => ({ ...f, busy: false, error: err.message }));
    }
  };

  const openZonePool = (zoneId: string) => {
    setPoolZone(zoneId);
    setView('zonePool');
  };

  const openFlatPool = (pointId: string) => {
    setFlatPoint(pointId);
    setView('flatPool');
  };

  /** «Позвать в пул — поделиться в чат»: диплинк на точку уходит в чат (§6). */
  const sharePoint = (pointId: string) => openShare(pointShareUrl(pointId));

  /**
   * «Хочу эту комнату» (§2.2): дорогой лайк автору прямо с карточки.
   *
   * Поиска этот лайк не требует — отношение несёт сама карточка-витрина, —
   * поэтому ветки need_search тут нет: отказать могут только лимит и
   * неодобренная анкета, и обоим отвечаем словами сервера.
   */
  const wantRoom = async (point: PointDTO) => {
    const author = point.author;
    if (!author || respondedPoints.includes(point.id)) return;
    // Кнопка отвечает до сервера, и она же держит второй тап: повторный лайк
    // сервер бы простил, но человеку в ленте нужен ответ сразу.
    setRespondedPoints((list) => [...list, point.id]);
    setFeed((f) => ({ ...f, error: null }));
    try {
      const res = await postLike(author.user_id, { pointId: point.id });
      setLikesRemaining(res.budget.remaining);
      reloadLikes();
      if (res.matched && res.match_id) {
        openMatch(res.match_id);
        return;
      }
      // Счётчик откликов считает сервер — перечитываем выдачу целиком.
      void refreshFeed();
    } catch (e: unknown) {
      setRespondedPoints((list) => list.filter((id) => id !== point.id));
      setFeed((f) => ({ ...f, error: asApiError(e).message }));
    }
  };

  /** Выход из чужого пула прямо из «Присмотренных» — лайки не трогает (§2.3). */
  const leaveFromList = async (pointId: string) => {
    setSearchesError(null);
    try {
      await leavePointPool(pointId);
      reloadPoints();
      void refreshFeed();
    } catch (e: unknown) {
      setSearchesError(asApiError(e).message);
    }
  };

  /** Закрытие своей точки. Причин у неё нет — в отличие от поиска (§3). */
  const closeMyPoint = async (pointId: string) => {
    setSearchesError(null);
    try {
      await closePoint(pointId);
      reloadPoints();
      void refreshFeed();
    } catch (e: unknown) {
      setSearchesError(asApiError(e).message);
    }
  };

  /** «Проверил — она живая»: своя скрытая голосами квартира возвращается в ленту (§8). */
  const restoreMyPoint = async (pointId: string) => {
    setSearchesError(null);
    try {
      await restorePoint(pointId);
      reloadPoints();
      void refreshFeed();
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

  const openIncoming = () => {
    // Отказ прошлого ответа не должен встречать на входе: он уже рассказан.
    setIncomingError(null);
    setView('incoming');
  };

  /**
   * «Ответить» из «Тебя лайкнули»: тот же дорогой лайк, но бесплатный (§2.3).
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
    // Зона пула предзаполнена: там они и пересеклись, и поиск скорее всего
    // нужен именно на неё.
    openLookForm(target.zoneId ? { ...EMPTY_DRAFT, zoneId: target.zoneId } : EMPTY_DRAFT);
    setFeed((f) => ({ ...f, error: serverMessage }));
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

  /** Выдача точек под параметры сохранённого поиска (строка «M квартир…»). */
  const openSearchFeed = (id: string) => {
    const s = active.find((x) => x.id === id);
    if (!s) return;
    const draft = paramsToDraft(s);
    setFeed((f) => ({ ...f, draft }));
    setTab('feed');
    setView('tab');
    void showFeed(draft);
  };

  const savedSearches: SavedSearch[] = active.map((s) => ({
    id: s.id,
    zone: s.zone.name,
    since: `в пуле с ${dayMonth(s.created_at)}`,
    // Зона уже в заголовке карточки — в чипах она была бы повтором.
    params: paramChips(s),
    peopleCount: s.people,
    fullMatchCount: s.full_match,
    newCount: s.new_count || undefined,
    pointsLine: s.points_count
      ? `${s.points_count} ${plural(s.points_count, 'квартира', 'квартиры', 'квартир')} под эти параметры`
      : '',
  }));

  const archivedSearches: ArchivedSearch[] = archived.map((s) => ({
    id: s.id,
    zone: s.zone.name,
    params: paramChips(s),
    reason: closeReasonLabel(s.close_reason),
  }));

  const tabScreen = () => {
    switch (tab) {
      case 'feed': {
        if (feed.mode !== 'results' || !feed.data) {
          return <FeedEntryScreen onSearch={() => openLookForm(EMPTY_DRAFT)} />;
        }
        const promo = feed.data.promo;
        return (
          <FeedScreen
            zone={promo.zone.name}
            params={paramChips(draftToParams(feed.draft), { zoneName: promo.zone.name })}
            promo={buildPromo(promo, active, feed.slotsFull)}
            points={feed.data.points}
            error={feed.error}
            likesRemaining={likesRemaining ?? undefined}
            myUserId={profile.id}
            respondedPoints={respondedPoints}
            topZones={topZones ?? undefined}
            onEditParams={() => openLookForm(feed.draft)}
            onJoinPool={() => void joinPool()}
            onOpenPool={() => openZonePool(promo.zone.id)}
            onFreeSlot={() => toTab('searches')}
            onEnterFlatPool={(id) => void enterFlatPool(id)}
            onOpenFlat={(point) => openFlatPool(point.id)}
            matchedAuthors={matches.map((m) => m.peer.user_id)}
            onWantRoom={(point) => void wantRoom(point)}
            // Профиль автора «к себе» открывается только с точкой: отношение
            // по ней сервер проверяет по этому параметру, без него — 404.
            onOpenProfile={(point) =>
              point.author &&
              openPerson({ userId: point.author.user_id, pointId: point.id, back: 'tab' })
            }
            // «Где уже ищут» из пустой ленты: форма просмотра с этой зоной —
            // подписки тап не создаёт, смотреть можно без неё (§3).
            onPickZone={(zoneId) => openLookForm({ ...EMPTY_DRAFT, zoneId })}
          />
        );
      }
      case 'searches':
        return (
          <MySearchesScreen
            searches={savedSearches}
            searchSlotsTotal={3}
            archived={archivedSearches}
            error={searchesError}
            flats={myPoints.created.filter((p) => p.type === 'flat')}
            enteredFlats={myPoints.entered}
            flatSlotsTotal={5}
            homes={myPoints.created.filter((p) => p.type === 'intent_home')}
            homeSlotsTotal={2}
            onOpenPool={(id) => {
              const s = active.find((x) => x.id === id);
              if (s) openZonePool(s.zone.id);
            }}
            onOpenSearch={openSearchFeed}
            onEditSearch={openEditForm}
            onCloseSearch={setClosing}
            onAddSearch={() => openLookForm(EMPTY_DRAFT)}
            onReactivate={(id) => void reactivate(id)}
            onOpenFlatPool={openFlatPool}
            onSharePoint={sharePoint}
            onClosePoint={(id) => void closeMyPoint(id)}
            onRestorePoint={(id) => void restoreMyPoint(id)}
            onLeavePool={(id) => void leaveFromList(id)}
            onCreateFlatPoint={() => setView('flatForm')}
            onCreateHomePoint={() => setView('homeForm')}
          />
        );
      case 'profile':
        return (
          <MyProfileScreen
            name={profile.name}
            age={profile.age ?? 0}
            occupation={occupationLabel(profile.occupation)}
            photoUrl={profile.photo_url}
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
            onOpenIncomingLikes={openIncoming}
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
        return (
          <SearchParamsScreen
            mode="look"
            zones={zones}
            updatesSearch={(searches ?? []).some(
              (s) => s.status === 'active' && s.zone.id === feed.draft.zoneId,
            )}
            value={feed.draft}
            onChange={(draft) => setFeed((f) => ({ ...f, draft }))}
            onSubmit={() => {
              if (isDraftReady(feed.draft)) void showFeed(feed.draft);
            }}
            onBack={closeForm}
            busy={feed.busy}
            error={feed.error}
            invalidFields={feed.invalidFields}
          />
        );
      }
      case 'zonePool':
        if (!poolZone) return tabScreen();
        return (
          <ZonePoolView
            zoneId={poolZone}
            pointsCount={active.find((s) => s.zone.id === poolZone)?.points_count ?? 0}
            // Бейдж «M новых» сервер только что погасил — перечитываем список,
            // чтобы клиент не показывал сгоревшую цифру.
            onLoaded={() => void reloadSearches().catch(() => undefined)}
            onBack={back}
            onOpenPerson={(id) =>
              openPerson({ userId: id, zoneId: poolZone ?? undefined, back: 'zonePool' })
            }
            onOpenPoints={() => toTab('feed')}
          />
        );
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
              // Лайк с точки-контекста — тот же отклик: карточка в ленте
              // обязана про него знать.
              const responded = target.pointId;
              if (responded) setRespondedPoints((list) => [...list, responded]);
              reloadLikes();
            }}
            onMatched={openMatch}
            onNeedSearch={(serverMessage) => askSearchForLike(target, serverMessage)}
            // Заблокированный стал невидимым: профиль по нему ответил бы 404,
            // а списки, где он стоял, обязаны перечитаться (§5). Карточка «к
            // себе» уходит вместе с автором, поэтому назад в неё не возвращаем:
            // экран показал бы отказ сервера вместо результата блокировки.
            onBlocked={() => {
              if (target.back === 'homePoint') toTab('feed');
              else setView(target.back);
              reloadLikes();
              reloadPoints();
              void refreshFeed();
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
            // счётчики пулов — пара снова видит друг друга (§2.3), а после
            // блока, наоборот, пропадает отовсюду (§5).
            onRemoved={() => {
              back();
              reloadLikes();
              reloadPoints();
              void reloadSearches().catch(() => undefined);
              void refreshFeed();
            }}
            onToFeed={() => toTab('feed')}
          />
        );
      case 'flatPool': {
        if (!flatPoint) return tabScreen();
        const pointId = flatPoint;
        return (
          <FlatPoolView
            key={pointId}
            pointId={pointId}
            mine={myPoints.created.some((p) => p.id === pointId)}
            onBack={back}
            onOpenPerson={(userId) => openPerson({ userId, pointId, back: 'flatPool' })}
            onChanged={() => {
              reloadPoints();
              void refreshFeed();
            }}
            onLeft={back}
            onShare={sharePoint}
          />
        );
      }
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
            // Отказ отклика оболочка кладёт в состояние ленты — тем же путём,
            // что и с карточки в выдаче: жест и обработчик у них один.
            error={feed.error}
            onBack={back}
            onWantRoom={(point) => void wantRoom(point)}
            onOpenProfile={(point) =>
              point.author &&
              openPerson({ userId: point.author.user_id, pointId: point.id, back: 'homePoint' })
            }
          />
        );
      }
      case 'incoming':
        return (
          <IncomingLikesScreen
            likes={incoming.map(toIncomingRow)}
            replyingId={replying}
            error={incomingError}
            onBack={back}
            // Контекст лайка тапабелен, но зонный пул открывается только со
            // своим поиском этой зоны — чужой пул сервер больше не отдаёт
            // (§2.3 «мы увиделись»). Иначе тап ведёт сразу в профиль
            // лайкнувшего. У квартиры такого условия нет: её карточку видят
            // все, а список — только вошедшие, и это состояние экрана.
            onOpenContext={(id) => {
              const like = incoming.find((l) => l.user_id === id);
              if (like?.point?.kind === 'flat') {
                openFlatPool(like.point.id);
                return;
              }
              const zone = like?.zone;
              if (zone && active.some((s) => s.zone.id === zone.id)) {
                openZonePool(zone.id);
              } else {
                openPerson({ userId: id, zoneId: zone?.id, back: 'incoming' });
              }
            }}
            onOpenProfile={(id) => {
              const like = incoming.find((l) => l.user_id === id);
              openPerson({
                userId: id,
                zoneId: like?.point ? undefined : like?.zone?.id,
                pointId: like?.point?.id,
                back: 'incoming',
              });
            }}
            onReply={(id) => void replyLike(id)}
            onToFeed={() => toTab('feed')}
          />
        );
      case 'flatForm':
        return (
          <FlatLinkFormScreen
            zones={zones}
            onBack={back}
            onCreated={(point) => {
              setCreatedPoint(point);
              reloadPoints();
              setView('pointCreated');
            }}
            // Дубль: своей точки не появилось, зато человек в чужом пуле —
            // ведём в ленту, где карточка уже зелёная.
            onEnteredPool={() => {
              reloadPoints();
              void refreshFeed();
              toTab('feed');
            }}
          />
        );
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
            onProfile={setProfile}
            // Назад — на таб «Профиль», а не «в никуда»: сюда приходят и по
            // диплинку пуша, с любого таба. Перечитываем себя: строка статуса
            // на профиле обязана знать про только что отправленное фото.
            onBack={() => {
              setEditTarget(null);
              toTab('profile');
            }}
          />
        );
      case 'pointCreated': {
        if (!createdPoint) return tabScreen();
        const share = pricePerPerson(createdPoint.price_total);
        return (
          <PointCreatedScreen
            title={pointTitle(createdPoint)}
            pricePerPerson={
              share !== null
                ? `${money(share)} с человека`
                : money(createdPoint.price_total ?? 0)
            }
            onToSearches={() => toTab('searches')}
          />
        );
      }
      default:
        return tabScreen();
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', minHeight: '100vh' }}>
      <div style={{ flex: '1 1 auto', minHeight: 0 }}>{screen()}</div>
      {/* Таб-бар рисует только оболочка: у экранов своего нет, иначе на табах
          их два и внутренний — мёртвый, без onChange. Пул тоже с табами:
          из него уходят обратно в свой таб, а не «в никуда». */}
      {(view === 'tab' || view === 'zonePool') && (
        <TabBar
          active={tab}
          unreadLikes={unreadLikes}
          hasNewInSearches={hasNewInSearches}
          onChange={toTab}
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
 * Промо-блок из ответа ленты (§6, четыре состояния).
 *
 * Порядок проверок важен: подписка на эту зону главнее занятых слотов
 * (подписавшемуся кнопка уже не нужна), занятые слоты — главнее пустоты
 * (звать «войди первым» некуда, если войти нельзя).
 */
function buildPromo(p: FeedPromo, active: SearchResponse[], slotsFull: boolean): PromoState {
  const zone = { zone: p.zone.name, zoneIn: p.zone.name_in, zoneGen: p.zone.name_gen };
  if (p.my_search_id) {
    // Слот — номер этого поиска среди активных по старшинству, а не позиция в
    // списке: сервер отдаёт новые первыми, и по нему каждый только что
    // сохранённый поиск оказывался бы «слотом 1». Первым занят тот слот,
    // который заняли раньше.
    const at = [...active]
      .sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at))
      .findIndex((s) => s.id === p.my_search_id);
    return {
      state: 'saved',
      ...zone,
      peopleCount: p.people,
      fullMatchCount: p.full_match,
      avatars: p.avatars,
      slot: at >= 0 ? at + 1 : p.slots_used,
      slotsTotal: p.slots_total,
    };
  }
  if (slotsFull || p.slots_used >= p.slots_total) {
    return {
      state: 'full',
      ...zone,
      peopleCount: p.people,
      fullMatchCount: p.full_match,
      avatars: p.avatars,
      slotsTotal: p.slots_total,
    };
  }
  if (p.people === 0) {
    return { state: 'empty', ...zone, peopleCount: 0, slot: p.slots_used + 1, slotsTotal: p.slots_total };
  }
  return {
    state: 'offer',
    ...zone,
    peopleCount: p.people,
    fullMatchCount: p.full_match,
    avatars: p.avatars,
    slot: p.slots_used + 1,
    slotsTotal: p.slots_total,
  };
}

/**
 * Отмечает вход в пул точки в уже показанной выдаче — до ответа сервера.
 *
 * Счётчик при этом не трогаем: он персональный и смотрящего не считает
 * (visible_people его не возвращает), так что «+1» врал бы до следующей выдачи.
 */
function withPoolMark(data: FeedResponse | null, id: string, entered: boolean): FeedResponse | null {
  if (!data) return data;
  return {
    ...data,
    points: data.points.map((p) => (p.id === id ? { ...p, in_pool: entered } : p)),
  };
}

/**
 * Карточка входящего лайка (§6).
 *
 * Контекста может не быть: зона выпала из справочника или лайк пришёл ответом,
 * у которого пула нет. Тогда мини-карточки просто нет — выдумывать ей место
 * пересечения нельзя.
 *
 * Метка точки приезжает готовой и уже от лица адресата: отклик на «к себе»
 * сервер подписывает «на твою комнату» (§2.2), и переписывать её здесь нечем.
 * Тапабельна она только у квартиры — у «к себе» пула нет, вести некуда.
 */
function toIncomingRow(l: IncomingLikeDTO): IncomingLike {
  const context = l.point
    ? { label: l.point.label, tappable: l.point.kind === 'flat' }
    : l.zone
      ? { label: `из пула ${l.zone.name_gen}`, tappable: true }
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
