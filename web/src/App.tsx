/* Оболочка приложения: тема, табы §6 и переходы между экранами.

   Живыми данными сейчас питается только анкета — под неё есть бэкенд (§12 шаг 2).
   Остальные экраны отрисованы на своих DEMO_*-данных из макета: вёрстка и
   переходы настоящие, содержимое подставное. Оживают по мере готовности
   таблиц зон, поисков, точек и лайков. */
import { useCallback, useEffect, useState } from 'react';
import { isMiniAppDark, useSignal } from '@telegram-apps/sdk-react';

import { ApiError, fetchMe, type Profile } from './api/client';
import { TabBar, type Tab } from './ui/TabBar';
import { OnboardingFlow } from './screens/OnboardingFlow';
import { DEMO_FEED, DEMO_PARAMS, DEMO_PROMO, FeedScreen } from './screens/FeedScreen';
import { DEMO_POOL, ZonePoolScreen } from './screens/ZonePoolScreen';
import { DEMO_ABOUT, DEMO_MATCHES, PersonProfileScreen } from './screens/PersonProfileScreen';
import {
  DEMO_MATCH_PARAMS,
  DEMO_MATCH_PROFILE,
  DEMO_MATCH_WHERE,
  DEMO_NEXT_STEPS,
  MatchScreen,
} from './screens/MatchScreen';
import { DEMO_FLAT_POOL, FlatPoolScreen } from './screens/FlatPoolScreen';
import { DEMO_INCOMING, IncomingLikesScreen } from './screens/IncomingLikesScreen';
import {
  DEMO_MARKET_FLATS,
  DEMO_MY_HOMES,
  DEMO_SEARCHES,
  MySearchesScreen,
} from './screens/MySearchesScreen';
import { DEMO_MY_MATCHES, MyProfileScreen } from './screens/MyProfileScreen';
import { FlatLinkFormScreen, HomePointFormScreen } from './screens/PointFormScreens';
import { PointCreatedScreen } from './screens/PointCreatedScreen';
import { LeavePoolSheet, StillLiveSheet } from './screens/EdgeStates';

type View =
  | 'tab'
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

export function App() {
  const isDark = useSignal(isMiniAppDark);
  const [state, setState] = useState<Loading>({ kind: 'loading' });
  const [tab, setTab] = useState<Tab>('feed');
  const [view, setView] = useState<View>('tab');
  const [sheet, setSheet] = useState<'leavePool' | 'stillLive' | null>(null);
  // Держит финальную развилку §4 на экране после отправки: статус к этому
  // моменту уже не draft, и без флага оболочка увела бы в ленту сразу.
  const [finishing, setFinishing] = useState(false);

  // Токены дизайна переключают тему по data-theme на <html>, а не по классу на
  // корне приложения: под ними живут и шторки, которые рисуются выше по дереву.
  useEffect(() => {
    document.documentElement.dataset.theme = isDark ? 'dark' : 'light';
  }, [isDark]);

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
    return () => {
      cancelled = true;
    };
  }, []);

  const setProfile = useCallback((profile: Profile) => setState({ kind: 'ready', profile }), []);

  const toTab = useCallback((next: Tab) => {
    setTab(next);
    setView('tab');
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

  const unreadLikes = DEMO_INCOMING.length;
  const back = () => setView('tab');

  const tabScreen = () => {
    switch (tab) {
      case 'feed':
        return (
          <FeedScreen
            zone="Хамовники"
            params={DEMO_PARAMS}
            promo={DEMO_PROMO}
            items={DEMO_FEED}
            unreadLikes={unreadLikes}
            hasNewInSearches
            onOpenPool={() => setView('zonePool')}
            onEnterFlatPool={() => setView('flatPool')}
            onOpenSource={() => setSheet('stillLive')}
            onWantRoom={() => setView('person')}
            onOpenProfile={() => setView('person')}
          />
        );
      case 'searches':
        return (
          <MySearchesScreen
            searches={DEMO_SEARCHES}
            searchSlotsTotal={3}
            flats={DEMO_MARKET_FLATS}
            flatSlotsTotal={5}
            homes={DEMO_MY_HOMES}
            homeSlotsTotal={2}
            unreadLikes={unreadLikes}
            onOpenPool={() => setView('zonePool')}
            onOpenSearch={() => setView('zonePool')}
            onOpenFlatPool={() => setView('flatPool')}
            onOpenIncomingLikes={() => setView('incoming')}
            onCreateFlatPoint={() => setView('flatForm')}
            onCreateHomePoint={() => setView('homeForm')}
          />
        );
      case 'profile':
        return (
          <MyProfileScreen
            name={profile.name}
            age={profile.age ?? 0}
            occupation={occupationLabel(profile)}
            status={profile.status === 'active' ? 'active' : 'pending'}
            emailKind={profile.occupation === 'study' ? 'edu' : 'work'}
            incomingLikes={unreadLikes}
            matches={DEMO_MY_MATCHES}
            onEdit={() => setView('editProfile')}
            onOpenIncomingLikes={() => setView('incoming')}
          />
        );
    }
  };

  const screen = () => {
    switch (view) {
      case 'zonePool':
        return (
          <ZonePoolScreen
            zone="Хамовники"
            zoneGen="Хамовниках"
            sections={DEMO_POOL}
            pointsCount={7}
            unreadLikes={unreadLikes}
            onBack={back}
            onOpenPerson={() => setView('person')}
            onOpenPoints={() => toTab('feed')}
          />
        );
      case 'person':
        return (
          <PersonProfileScreen
            name="Аня"
            age={22}
            occupation="Студент"
            verifiedBadge="ВШЭ"
            lastSeen="была сегодня"
            zone="пересеклись в пуле Хамовников"
            matches={DEMO_MATCHES}
            matchesTotal={6}
            about={DEMO_ABOUT}
            searchFacts={[
              { label: 'Бюджет', value: '40–45к с человека' },
              { label: 'Заезд', value: 'с 1 сентября' },
            ]}
            likesLeft={3}
            likesTotal={5}
            onBack={back}
            onLike={() => setView('match')}
          />
        );
      case 'match':
        return (
          <MatchScreen
            name="Аня"
            username="anya"
            where={DEMO_MATCH_WHERE}
            params={DEMO_MATCH_PARAMS}
            profileMatches={DEMO_MATCH_PROFILE}
            nextSteps={DEMO_NEXT_STEPS}
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
            onOpenPerson={() => setView('person')}
            onLeave={() => setSheet('leavePool')}
          />
        );
      case 'incoming':
        return (
          <IncomingLikesScreen
            likes={DEMO_INCOMING}
            unreadLikes={unreadLikes}
            onBack={back}
            onOpenContext={() => setView('zonePool')}
            onOpenProfile={() => setView('person')}
            onReply={() => setView('match')}
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
      {view === 'tab' && (
        <TabBar active={tab} unreadLikes={unreadLikes} hasNewInSearches onChange={toTab} />
      )}

      {sheet === 'leavePool' && (
        <LeavePoolSheet
          pointTitle="Двушки на Соколе"
          onStay={() => setSheet(null)}
          onLeave={() => setSheet(null)}
        />
      )}
      {sheet === 'stillLive' && (
        <StillLiveSheet onYes={() => setSheet(null)} onRented={() => setSheet(null)} />
      )}
    </div>
  );
}

const OCCUPATIONS: Record<string, string> = {
  study: 'Учусь',
  office: 'Офис',
  hybrid: 'Гибрид',
  remote: 'Удалёнка',
  shift: 'Сменный график',
};

function occupationLabel(profile: Profile): string {
  return profile.occupation ? (OCCUPATIONS[profile.occupation] ?? '') : '';
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
