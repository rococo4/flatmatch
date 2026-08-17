/* Прохождение анкеты: 12 экранов §4, draft → отправка → pending.
   Экраны живут в OnboardingScreens.tsx и ничего не знают про сеть — здесь
   состояние, порядок и разговор с API. */
import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  backButton,
  hideBackButton,
  onBackButtonClick,
  showBackButton,
} from '@telegram-apps/sdk-react';

import { ApiError, patchProfile, submitProfile, uploadPhoto } from '../api/client';
import type { Profile, Questionnaire } from '../api/client';
import {
  AboutStep,
  BasicsStep,
  ChoiceStep,
  OnboardingDone,
  PetsStep,
  PhotoStep,
  QUESTION_STEPS,
  TOTAL_STEPS,
  type Basics,
} from './OnboardingScreens';

/**
 * Порядок экранов. Индекс здесь — это onboarding_step в базе (0..11), а номер
 * внутри самого экрана — человеческий «шаг N из 12». Расхождение на единицу
 * намеренное: база считает с нуля, люди с единицы.
 */
const SCREENS = [
  'basics',
  ...QUESTION_STEPS.slice(0, 3).map((q) => q.field), // занятость, режим, курение
  'pets',
  ...QUESTION_STEPS.slice(3).map((q) => q.field), // шум, гости, уборка, алкоголь, траты
  'about',
  'photo',
] as const;

/* Шаги правки: сюда ведут строка реджекта в профиле и кнопки пушей «Поправить»
   и «Загрузить» (§5). Индексы считаются из порядка экранов, а не пишутся
   числами: вставленный вопрос сдвинул бы их молча. */
export const ABOUT_STEP = SCREENS.indexOf('about');
export const PHOTO_STEP = SCREENS.indexOf('photo');

const GENDERS = { f: 'female', m: 'male' } as const;

/** Обратное направление — чтобы вернувшийся увидел свои ответы, а не пустые экраны. */
function basicsFrom(profile: Profile): Basics {
  return {
    age: profile.age ? String(profile.age) : '',
    gender: profile.gender === 'female' ? 'f' : profile.gender === 'male' ? 'm' : null,
    pref: profile.gender_pref ?? null,
  };
}

export function OnboardingFlow({
  profile,
  initialStep,
  onProfile,
  onSubmitted,
  onFinished,
}: {
  profile: Profile;
  /**
   * С какого экрана начать. Пусто — с брошенного (закладка в базе, §11).
   * Задаётся правкой: вердикт модератора зовёт чинить конкретное — фото или
   * «о себе», — а не проходить анкету заново.
   */
  initialStep?: number;
  /** Сервер отвечает профилем целиком на каждый вызов — он и есть истина. */
  onProfile: (next: Profile) => void;
  /**
   * Анкета ушла на проверку. Нужен отдельно от onProfile: после отправки статус
   * перестаёт быть draft, и без этого сигнала оболочка сменила бы экран раньше,
   * чем человек увидит финальную развилку §4.
   */
  onSubmitted: () => void;
  onFinished: () => void;
}) {
  const [step, setStep] = useState(() =>
    Math.min(Math.max(initialStep ?? profile.onboarding_step, 0), SCREENS.length - 1),
  );
  const [basics, setBasics] = useState<Basics>(() => basicsFrom(profile));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  const goBack = useCallback(() => setStep((s) => Math.max(0, s - 1)), []);

  // Родная кнопка «назад» Telegram вместо своей в шапке: на первом экране её
  // быть не должно, иначе она уводит из приложения на середине анкеты.
  useEffect(() => {
    if (!backButton.isMounted()) return;
    if (step > 0 && !done && showBackButton.isAvailable()) {
      showBackButton();
      return onBackButtonClick(goBack);
    }
    if (hideBackButton.isAvailable()) hideBackButton();
    return undefined;
  }, [step, done, goBack]);

  /** Сохраняет ответ и переходит дальше. Шаг в базе двигается тем же запросом. */
  const save = useCallback(
    async (patch: Questionnaire, nextStep: number) => {
      setBusy(true);
      setError(null);
      try {
        onProfile(await patchProfile({ ...patch, onboarding_step: nextStep }));
        setStep(nextStep);
      } catch (e) {
        setError(messageOf(e));
      } finally {
        setBusy(false);
      }
    },
    [onProfile],
  );

  const answer = useCallback(
    (field: string, value: string) => void save({ [field]: value } as Questionnaire, step + 1),
    [save, step],
  );

  const screen = SCREENS[step];

  const canLeaveBasics = useMemo(() => {
    const age = Number(basics.age);
    return Boolean(basics.gender && basics.pref) && age >= 16 && age <= 100;
  }, [basics]);

  if (done) {
    return (
      <Frame>
        <OnboardingDone onSetupSearch={onFinished} onHostFlow={onFinished} onJustLook={onFinished} />
      </Frame>
    );
  }

  return (
    <Frame>
      {/* Своя кнопка возврата, а не только родная у Telegram: та живёт в чужой
          шапке, её не видно при разработке в браузере, и на неё нельзя
          сослаться словами. Здесь она на каждом шаге, кроме первого. */}
      {step > 0 && (
        <button
          type="button"
          onClick={goBack}
          disabled={busy}
          style={{
            alignSelf: 'flex-start',
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            padding: '6px 10px 6px 6px',
            border: 'none',
            borderRadius: 9,
            background: 'transparent',
            font: 'inherit',
            fontSize: 15,
            fontWeight: 510,
            color: 'var(--tg-theme-accent-text-color)',
            cursor: busy ? 'default' : 'pointer',
            opacity: busy ? 0.4 : 1,
          }}
        >
          <span style={{ fontSize: 20, lineHeight: 1 }}>‹</span>
          Назад
        </button>
      )}

      {screen === 'basics' && (
        <BasicsStep
          value={basics}
          onChange={setBasics}
          canGoNext={canLeaveBasics && !busy}
          onNext={() =>
            void save(
              {
                age: Number(basics.age),
                gender: basics.gender ? GENDERS[basics.gender] : undefined,
                gender_pref: basics.pref ?? undefined,
              },
              1,
            )
          }
        />
      )}

      {screen === 'pets' && (
        <PetsStep
          own={profile.pets_own ?? null}
          others={profile.pets_ok ?? null}
          canGoNext={Boolean(profile.pets_own && profile.pets_ok) && !busy}
          onChange={({ own, others }) =>
            void save(
              {
                pets_own: (own ?? undefined) as Questionnaire['pets_own'],
                pets_ok: (others ?? undefined) as Questionnaire['pets_ok'],
              },
              // Оба подвопроса на одном экране, поэтому ответ на любой из них
              // сохраняется, но шаг не двигает — дальше уводит кнопка.
              step,
            )
          }
          onNext={() => setStep(step + 1)}
        />
      )}

      {screen === 'about' && (
        <>
          <AboutStep text={profile.about ?? ''} onChange={(text) => onProfile({ ...profile, about: text })} />
          <NextButton
            disabled={!profile.about?.trim() || busy}
            onClick={() => void save({ about: profile.about }, step + 1)}
          />
        </>
      )}

      {screen === 'photo' && (
        <>
          <PhotoStep
            previewUrl={profile.photo_url ?? null}
            uploading={busy}
            error={error}
            onPick={(file) => void upload(file)}
          />
          <NextButton
            label="Отправить на проверку"
            disabled={!profile.photo_url || busy}
            onClick={() => void submit()}
          />
        </>
      )}

      {isQuestion(screen) && (
        <ChoiceStep
          {...questionOf(screen)}
          value={(profile[screen] as string | undefined) ?? null}
          busy={busy}
          onNext={(id) => answer(screen, id)}
        />
      )}

      {error && screen !== 'photo' && (
        <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)', padding: '0 4px' }}>
          {error}
        </span>
      )}
    </Frame>
  );

  async function upload(file: File) {
    setBusy(true);
    setError(null);
    try {
      onProfile(await uploadPhoto(file));
    } catch (e) {
      setError(messageOf(e));
    } finally {
      setBusy(false);
    }
  }

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      onProfile(await submitProfile());
      onSubmitted();
      setDone(true);
    } catch (e) {
      // Сервер называет недостающие поля — отправляем человека на первое из них,
      // иначе «анкета не заполнена» не говорит, куда идти.
      if (e instanceof ApiError && e.code === 'profile_incomplete' && e.fields.length) {
        const back = SCREENS.indexOf(e.fields[0] as (typeof SCREENS)[number]);
        if (back >= 0) setStep(back);
      }
      setError(messageOf(e));
    } finally {
      setBusy(false);
    }
  }
}

function Frame({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        // По центру экрана, а не прижато к верху: на телефоне карточка иначе
        // висит вверху с пустотой под собой на пол-экрана.
        justifyContent: 'center',
        gap: 14,
        minHeight: '100vh',
        // Потолок ширины — для десктопа: приложение рисуется под телефон, и
        // растянутая на весь монитор карточка выглядит сломанной.
        width: '100%',
        maxWidth: 520,
        margin: '0 auto',
        padding: '18px 14px 26px',
        background: 'var(--app-bg)',
        color: 'var(--tg-theme-text-color)',
      }}
    >
      {children}
    </div>
  );
}

function NextButton({
  label = 'Дальше',
  disabled,
  onClick,
}: {
  label?: string;
  disabled: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      style={{
        width: '100%',
        padding: 13,
        border: 'none',
        borderRadius: 11,
        background: 'var(--tg-theme-button-color)',
        font: 'inherit',
        fontSize: 16,
        fontWeight: 590,
        color: 'var(--tg-theme-button-text-color)',
        opacity: disabled ? 0.4 : 1,
        cursor: disabled ? 'default' : 'pointer',
      }}
    >
      {label}
    </button>
  );
}

type QuestionField = (typeof QUESTION_STEPS)[number]['field'];

function isQuestion(screen: (typeof SCREENS)[number]): screen is QuestionField {
  return QUESTION_STEPS.some((q) => q.field === screen);
}

function questionOf(field: QuestionField) {
  const q = QUESTION_STEPS.find((x) => x.field === field)!;
  return { step: q.step, title: q.title, why: q.why, choices: q.choices, total: TOTAL_STEPS };
}

function messageOf(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return 'Не получилось сохранить — проверь связь';
}
