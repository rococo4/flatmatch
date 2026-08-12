/* Оболочка приложения: тема, табы §6 и переходы между экранами.

   Живыми данными питаются анкета (§12 шаг 2), зоны, поиски, лента и зонный
   пул (§12 шаг 3), профиль человека, лайки и мэтчи (§12 шаг 4). Точки ждут
   своего шага и пока отрисованы на DEMO_*-данных из макета: вёрстка и
   переходы настоящие, содержимое подставное.

   Здесь же живёт состояние ленты — намеренно выше экранов: переключение табов
   не должно ронять выдачу и перезапрашивать её. И здесь единственный владелец
   списка поисков: после любой мутации он перечитывается с сервера целиком,
   локальных «оптимистичных» правок нет — счётчики и номера слотов считает
   только сервер. Входящие лайки и мэтчи — по тому же правилу: бейдж на табе
   считается по одному списку, который перечитывается после каждого лайка,
   откуда бы его ни поставили. */
import { useCallback, useEffect, useState } from 'react';
import { isMiniAppDark, useSignal } from '@telegram-apps/sdk-react';

import {
  ApiError,
  closeSearch,
  createSearch,
  fetchFeed,
  fetchIncoming,
  fetchMatches,
  fetchMe,
  fetchSearches,
  fetchZones,
  postLike,
  reactivateSearch,
  updateSearch,
  type CloseReason,
  type FeedPromo,
  type FeedResponse,
  type IncomingLikeDTO,
  type MatchDTO,
  type Profile,
  type SearchResponse,
  type Zone,
} from './api/client';
import {
  closeReasonLabel,
  dayMonth,
  habitsLine,
  occupationLabel,
  paramChips,
  plural,
  relativeDay,
} from './api/format';
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
import { DEMO_FLAT_POOL, FlatPoolScreen } from './screens/FlatPoolScreen';
import { IncomingLikesScreen, type IncomingLike } from './screens/IncomingLikesScreen';
import {
  DEMO_MARKET_FLATS,
  DEMO_MY_HOMES,
  MySearchesScreen,
  type ArchivedSearch,
  type SavedSearch,
} from './screens/MySearchesScreen';
import { MyProfileScreen, type MyMatch } from './screens/MyProfileScreen';
import { FlatLinkFormScreen, HomePointFormScreen } from './screens/PointFormScreens';
import { PointCreatedScreen } from './screens/PointCreatedScreen';
import { CloseSearchSheet, LeavePoolSheet, StillLiveSheet } from './screens/EdgeStates';

type View =
  | 'tab'
  | 'searchForm'
  | 'zonePool'
  | 'person'
  | 'match'
  | 'flatPool'
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
 * `zoneId` — пул, из которого пришли: он и строка «пересеклись», и контекст
 * лайка. `back` — куда возвращает стрелка: из пула уходят обратно в пул, а не
 * «в никуда».
 */
type PersonTarget = { userId: string; zoneId?: string; back: View };

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
  const [sheet, setSheet] = useState<'leavePool' | 'stillLive' | null>(null);
  // Держит финальную развилку §4 на экране после отправки: статус к этому
  // моменту уже не draft, и без флага оболочка увела бы в ленту сразу.
  const [finishing, setFinishing] = useState(false);

  const [zones, setZones] = useState<Zone[] | null>(null);
  const [searches, setSearches] = useState<SearchResponse[] | null>(null);
  const [feed, setFeed] = useState<FeedState>(EMPTY_FEED);
  const [formMode, setFormMode] = useState<FormMode | null>(null);
  const [poolZone, setPoolZone] = useState<string | null>(null);
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
  }, [reloadLikes]);

  const setProfile = useCallback((profile: Profile) => setState({ kind: 'ready', profile }), []);

  const toTab = useCallback((next: Tab) => {
    setTab(next);
    setView('tab');
  }, []);

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

  if (state.kind === 'loading') return <Centered>Секунду…</Centered>;
  if (state.kind === 'failed') return <Failure error={state.error} />;

  const { profile } = state;

  // Анкета не дозаполнена — весь остальной продукт подождёт. В draft можно
  // листать ленту, но входом по умолчанию должен быть онбординг (§5).
  if (profile.status === 'draft' || view === 'editProfile' || finishing) {
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

  const reloadSearches = async (): Promise<SearchResponse[]> => {
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
  };

  /** Форма параметров просмотра: «Найти» с входа и «Изменить параметры» с выдачи. */
  const openLookForm = (draft: SearchDraft) => {
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
          const res = await postLike(target.userId, target.zoneId);
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

  const openMatch = (id: string) => {
    setMatchId(id);
    setView('match');
  };

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
      const res = await postLike(userId, incoming.find((l) => l.user_id === userId)?.zone?.id);
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
            // Точек ещё нет (§12 шаг 5), поэтому и переходов с их карточек в
            // профиль нет: открывать живой профиль без id человека нечем.
            items={[]}
            error={feed.error}
            onEditParams={() => openLookForm(feed.draft)}
            onJoinPool={() => void joinPool()}
            onOpenPool={() => openZonePool(promo.zone.id)}
            onFreeSlot={() => toTab('searches')}
            onEnterFlatPool={() => setView('flatPool')}
            onOpenSource={() => setSheet('stillLive')}
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
            flats={DEMO_MARKET_FLATS}
            flatSlotsTotal={5}
            homes={DEMO_MY_HOMES}
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
            onOpenFlatPool={() => setView('flatPool')}
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
            status={profile.status === 'active' ? 'active' : 'pending'}
            emailKind={profile.occupation === 'study' ? 'edu' : 'work'}
            incomingLikes={unreadLikes}
            matches={matches.map(toMyMatch)}
            onEdit={() => setView('editProfile')}
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
            onBack={() => setView(target.back)}
            onLiked={reloadLikes}
            onMatched={openMatch}
            onNeedSearch={(serverMessage) => askSearchForLike(target, serverMessage)}
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
            onToFeed={() => toTab('feed')}
          />
        );
      case 'flatPool':
        return (
          <FlatPoolScreen
            flat={{
              title: '2-комнатная · Хамовники',
              pricePerPerson: '45 000 ₽',
              metro: 'Фрунзенская · 12 мин пешком',
              source: 'Авито',
              postedAt: '27 июля',
              addedAt: 'добавлено 20 июля',
            }}
            people={DEMO_FLAT_POOL}
            poolCount={6}
            shownCount={3}
            onBack={back}
            onOpenSource={() => setSheet('stillLive')}
            // Люди в этом пуле — из макета: их id ненастоящие, и живой профиль
            // по ним не открыть. Переход появится вместе с точками (§12 шаг 5).
            onLeave={() => setSheet('leavePool')}
          />
        );
      case 'incoming':
        return (
          <IncomingLikesScreen
            likes={incoming.map(toIncomingRow)}
            replyingId={replying}
            error={incomingError}
            onBack={back}
            // Контекст лайка тапабелен, но пул открывается только со своим
            // поиском этой зоны — чужой пул сервер больше не отдаёт (§2.3
            // «мы увиделись»). Иначе тап ведёт сразу в профиль лайкнувшего.
            onOpenContext={(id) => {
              const zone = incoming.find((l) => l.user_id === id)?.zone;
              if (zone && active.some((s) => s.zone.id === zone.id)) {
                openZonePool(zone.id);
              } else {
                openPerson({ userId: id, zoneId: zone?.id, back: 'incoming' });
              }
            }}
            onOpenProfile={(id) =>
              openPerson({
                userId: id,
                zoneId: incoming.find((l) => l.user_id === id)?.zone?.id,
                back: 'incoming',
              })
            }
            onReply={(id) => void replyLike(id)}
            onToFeed={() => toTab('feed')}
          />
        );
      case 'flatForm':
        return <FlatLinkFormScreen onBack={back} onSubmit={() => setView('pointCreated')} />;
      case 'homeForm':
        return <HomePointFormScreen onBack={back} onSubmit={() => setView('pointCreated')} />;
      case 'pointCreated':
        return (
          <PointCreatedScreen
            title="2-комнатная · Хамовники"
            pricePerPerson="45 000 ₽"
            onToSearches={() => toTab('searches')}
          />
        );
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
      {sheet === 'leavePool' && (
        <LeavePoolSheet
          pointTitle="Двушки на Соколе"
          onStay={() => setSheet(null)}
          onLeave={() => setSheet(null)}
        />
      )}
      {sheet === 'stillLive' && (
        <StillLiveSheet
          onYes={() => setSheet(null)}
          onRented={() => setSheet(null)}
          onDismiss={() => setSheet(null)}
        />
      )}
    </div>
  );
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
 * Карточка входящего лайка (§6).
 *
 * Контекста может не быть: зона выпала из справочника или лайк пришёл ответом,
 * у которого пула нет. Тогда мини-карточки просто нет — выдумывать ей место
 * пересечения нельзя.
 */
function toIncomingRow(l: IncomingLikeDTO): IncomingLike {
  return {
    id: l.user_id,
    name: l.name,
    age: l.age,
    photoUrl: l.photo_url || undefined,
    habits: habitsLine(l),
    when: relativeDay(l.at),
    notSearching: l.not_searching,
    contextZone: l.zone ? `из пула ${l.zone.name_gen}` : undefined,
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
  const hint =
    error.status === 401
      ? 'Проверь BOT_TOKEN у api и VITE_MOCK_INIT_DATA в web/.env.local — см. README.'
      : error.message;

  return (
    <Centered>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8, textAlign: 'center' }}>
        <span style={{ fontSize: 18, fontWeight: 640, color: 'var(--tg-theme-text-color)' }}>
          Не получилось войти
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
