/* Квартира с рынка — §6 «Карточка квартиры».

   Один экран на два состояния членства, потому что это одна и та же квартира:
   не вошедший видит карточку с «в пуле N — увидишь после входа» и кнопкой
   входа, вошедший — тот же список лайкнувших с совпадениями анкеты и
   вердикт-чипами (если у смотрящего есть поиск) и красный выход с
   подтверждением (шторка живёт в EdgeStates).

   Данные приезжают из FlatPoolView: здесь только вёрстка. */
import { useState } from 'react';
import type { CSSProperties } from 'react';
import {
  Avatar,
  FactsBlock,
  GreyChips,
  LimitChip,
  PhotoPlaceholder,
  PoolCount,
  PoolEntryButton,
  S,
  VerdictChip,
  type Fact,
  type Verdict,
} from '../ui/primitives';

export type FlatPoolPerson = {
  id: string;
  name: string;
  age: number;
  /** Пусто = фото не одобрено: у аватара заглушка с инициалами (§5). */
  photoUrl?: string;
  habits: string;
  matches: number;
  verdicts: { verdict: Verdict; label: string }[];
  /** Ваш мэтч — бейдж вместо шеврона, лайкать уже нечего. */
  matched?: boolean;
};

/** Шапка-карточка: та же квартира, что в ленте, только развёрнутая. */
export type FlatPoolCard = {
  title: string; // «2-комнатная · м. Фрунзенская»
  /** Все фото точки; первое — обложка, остальные листаются свайпом. */
  photoUrls: string[];
  /** «45 000 ₽ с человека» — или цена за квартиру, если делить не на что. */
  price: string;
  /** «90 000 ₽ / мес за квартиру». Пусто — цена доли не считалась. */
  priceTotal?: string;
  facts: Fact[];
  restrictions: string[];
  /** «подтверждена вчера» / «добавлена 5 дней назад, не проверялась» (§8). */
  freshness: string;
  /** «на проверке» у своей ещё не одобренной точки; у чужих пусто. */
  status?: string;
  /** Точка на паузе: жёлтая плашка вместо строки статуса (§8). */
  gone?: { banner: string; restorable: boolean };
  /** «Открыть на Авито». Пусто — ссылки на оригинал у точки нет. */
  sourceLabel?: string;
};

/**
 * Состояние блока-вопроса «Объявление ещё живое?» (§8).
 *
 * `asked` — по ссылке уже сходили в этой сессии: до этого спрашивать не о чем.
 * `myVote` — голос, с которым пришли (или только что оставили); null — не
 * голосовал, показываем кнопки. `justVoted` отличает «Спасибо, отметили» сразу
 * после ответа от «Ты отметил: …» у пришедшего с готовым голосом.
 */
export type FlatVoteState = {
  asked: boolean;
  myVote: boolean | null;
  busy: boolean;
  justVoted?: boolean;
};

const L = {
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
  body: {
    flex: '1 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: '12px 12px 20px',
  } as CSSProperties,
  personCard: {
    display: 'flex',
    gap: 12,
    padding: 12,
    borderRadius: 12,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
    cursor: 'pointer',
  } as CSSProperties,
  moreButton: {
    width: '100%',
    padding: 11,
    border: 'none',
    borderRadius: 10,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 14,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  /** Красная кнопка выхода стоит прямо на полотне, поэтому своя поверхность
   *  и рамка: --app-bg совпал бы с фоном экрана в обеих темах. */
  leaveButton: {
    width: '100%',
    padding: 13,
    border: '1px solid color-mix(in srgb, var(--tg-theme-destructive-text-color) 30%, transparent)',
    borderRadius: 11,
    background: 'color-mix(in srgb, var(--tg-theme-destructive-text-color) 10%, var(--card-bg))',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-destructive-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  sourceLink: {
    border: 'none',
    background: 'none',
    font: 'inherit',
    fontSize: 13,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
    padding: 0,
  } as CSSProperties,
  /** Жёлтая плашка паузы — тот же жёлтый, что у лимитов и вердикта «краем». */
  gonePlate: {
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    padding: '10px 12px',
    borderRadius: 10,
    background: 'var(--warning-bg)',
    color: 'var(--warning-fg)',
    fontSize: 14,
    fontWeight: 590,
  } as CSSProperties,
  /** Внутри жёлтой плашки: серый --chip-bg на жёлтом выглядел бы грязью. */
  restoreButton: {
    width: '100%',
    padding: 11,
    border: 'none',
    borderRadius: 10,
    background: 'var(--card-bg)',
    font: 'inherit',
    fontSize: 14,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  voteBlock: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    padding: '10px 12px',
    borderRadius: 10,
    background: 'var(--app-bg)',
  } as CSSProperties,
  voteButton: {
    flex: '1 1 0',
    padding: 10,
    border: 'none',
    borderRadius: 9,
    background: 'var(--card-bg)',
    font: 'inherit',
    fontSize: 14,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  changeLink: {
    border: 'none',
    background: 'none',
    font: 'inherit',
    fontSize: 13,
    fontWeight: 510,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
    padding: 0,
  } as CSSProperties,
};

export function FlatPoolScreen({
  flat,
  members,
  poolCount,
  mine,
  busy,
  error,
  vote,
  shownCount = 3,
  onBack,
  onOpenSource,
  onOpenPerson,
  onEnter,
  onLeave,
  onShare,
  onVote,
  onRestore,
  onChangeVote,
}: {
  flat: FlatPoolCard;
  /** null = смотрящий не в пуле: список ему закрыт, карточка — нет (§6). */
  members: FlatPoolPerson[] | null;
  poolCount: number;
  /** Своя точка: выхода из собственного пула нет — вернуться было бы некуда. */
  mine?: boolean;
  busy?: boolean;
  error?: string | null;
  /** Голосование о свежести: только у квартиры с рынка со ссылкой и активной (§8). */
  vote?: FlatVoteState;
  shownCount?: number;
  onBack?: () => void;
  onOpenSource?: () => void;
  onOpenPerson?: (id: string) => void;
  onEnter?: () => void;
  onLeave?: () => void;
  /** «Позвать в пул — поделиться в чат»: у любого участника, квартира ничья (§2.2). */
  onShare?: () => void;
  /** «Да» / «Уже сдана» из блока-вопроса. */
  onVote?: (alive: boolean) => void;
  /** «Проверил — она живая» под жёлтой плашкой — только у создателя (§8). */
  onRestore?: () => void;
  /** «изменить» у оставленного голоса — вернуть кнопки. */
  onChangeVote?: () => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const shown = expanded ? (members ?? []) : (members ?? []).slice(0, shownCount);
  const rest = (members?.length ?? 0) - shown.length;

  return (
    <div style={L.screen}>
      <div style={L.header}>
        <span
          style={{ fontSize: 22, lineHeight: 1, color: 'var(--tg-theme-accent-text-color)', cursor: 'pointer' }}
          onClick={onBack}
        >
          ‹
        </span>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>
          {members ? 'Пул квартиры' : 'Квартира'}
        </span>
      </div>

      <div style={L.body}>
        {error && (
          <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>
            {error}
          </span>
        )}

        <article style={S.card}>
          <div style={{ ...S.cardHeadBase, background: 'var(--type-flat-bg)', color: 'var(--type-flat-fg)' }}>
            <span style={{ fontSize: 13, fontWeight: 640, letterSpacing: '-0.1px' }}>Квартира с рынка</span>
            {/* Строка свежести длиннее «добавлена вчера» — с припиской про
                голос она может встать в две строки, и тогда край должен быть
                ровным. */}
            <span style={{ fontSize: 12, fontWeight: 510, opacity: 0.82, textAlign: 'right' }}>
              {flat.freshness}
            </span>
          </div>
          <div style={{ ...S.cardBody, gap: 12 }}>
            {flat.photoUrls.length > 0 ? (
              <div
                className="no-scrollbar"
                style={{
                  display: 'flex',
                  gap: 8,
                  overflowX: 'auto',
                  scrollSnapType: 'x mandatory',
                  borderRadius: 10,
                }}
              >
                {flat.photoUrls.map((url) => (
                  <img
                    key={url}
                    src={url}
                    alt=""
                    style={{
                      width: flat.photoUrls.length > 1 ? '88%' : '100%',
                      flex: '0 0 auto',
                      aspectRatio: '16/9',
                      objectFit: 'cover',
                      borderRadius: 10,
                      display: 'block',
                      scrollSnapAlign: 'start',
                    }}
                  />
                ))}
              </div>
            ) : (
              <PhotoPlaceholder label="скрин объявления" style={{ aspectRatio: '16/9', borderRadius: 10 }} />
            )}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
              <span style={S.title}>{flat.title}</span>
              <span style={{ fontSize: 20, fontWeight: 640, letterSpacing: '-0.4px' }}>{flat.price}</span>
              {flat.priceTotal && <span style={S.subtitle}>{flat.priceTotal}</span>}
            </div>
            <FactsBlock title="Квартира" facts={flat.facts}>
              <GreyChips items={flat.restrictions} />
            </FactsBlock>
            {/* Плашка паузы заменяет собой строку статуса: у скрытой голосами
                квартиры «похоже, сдана» — главная новость карточки. */}
            {flat.gone && (
              <div style={L.gonePlate}>
                <span>{flat.gone.banner}</span>
                {flat.gone.restorable && (
                  <button type="button" style={L.restoreButton} disabled={busy} onClick={onRestore}>
                    Проверил — она живая
                  </button>
                )}
              </div>
            )}
            {flat.status && (
              <div>
                <LimitChip>{flat.status}</LimitChip>
              </div>
            )}
            {flat.sourceLabel && (
              <button type="button" style={L.sourceLink} onClick={onOpenSource}>
                {flat.sourceLabel} ↗
              </button>
            )}
            {vote && (vote.asked || vote.myVote !== null) && (
              <VoteBlock vote={vote} onVote={onVote} onChangeVote={onChangeVote} />
            )}
          </div>
        </article>

        {members === null ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
            <PoolCount n={poolCount} kind="flat" hint="увидишь после входа" />
            <PoolEntryButton
              label="Интересно — войти в пул"
              caption="Вход бесплатный и безлимитный"
              onClick={busy ? undefined : onEnter}
            />
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <span style={S.sectionTitle}>
              В ПУЛЕ · {members.length} · ХОТЯТ СНЯТЬ ЭТУ КВАРТИРУ
            </span>

            {members.length === 0 && (
              <span style={{ fontSize: 14, color: 'var(--tg-theme-subtitle-text-color)', textWrap: 'pretty' }}>
                Пока ты один в пуле — пришлём пуш, когда войдёт кто-то ещё
              </span>
            )}

            {shown.map((p) => (
              <div key={p.id} style={L.personCard} onClick={() => onOpenPerson?.(p.id)}>
                <Avatar name={p.name} size={48} src={p.photoUrl} />
                <div style={{ flex: '1 1 auto', display: 'flex', flexDirection: 'column', gap: 6, minWidth: 0 }}>
                  <span style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <span style={{ fontSize: 16, fontWeight: 590, letterSpacing: '-0.2px' }}>
                      {p.name}, {p.age}
                    </span>
                    {p.matched && (
                      <span
                        style={{
                          fontSize: 11,
                          fontWeight: 640,
                          letterSpacing: '.04em',
                          textTransform: 'uppercase',
                          padding: '2px 7px',
                          borderRadius: 999,
                          background: 'var(--verdict-ok-bg)',
                          color: 'var(--verdict-ok-fg)',
                        }}
                      >
                        мэтч
                      </span>
                    )}
                  </span>
                  <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>{p.habits}</span>
                  <span style={{ fontSize: 13, fontWeight: 510 }}>{p.matches} совпадений анкеты</span>
                  {p.verdicts.length > 0 && (
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, paddingTop: 2 }}>
                      {p.verdicts.map((v) => (
                        <VerdictChip key={v.label} verdict={v.verdict}>
                          {v.label}
                        </VerdictChip>
                      ))}
                    </div>
                  )}
                </div>
                <span
                  style={{ flex: '0 0 auto', alignSelf: 'center', fontSize: 20, color: 'var(--tg-theme-hint-color)' }}
                >
                  ›
                </span>
              </div>
            ))}

            {rest > 0 && (
              <button type="button" style={L.moreButton} onClick={() => setExpanded(true)}>
                ещё {rest}
              </button>
            )}
          </div>
        )}

        {/* Из своего пула выхода нет: вход обратно требует активной точки, и
            создатель точки на проверке вернуться бы уже не смог. */}
        {members !== null && onShare && (
          <button type="button" style={S.secondaryButton} onClick={onShare}>
            Позвать в пул — поделиться в чат
          </button>
        )}
        {members !== null && !mine && (
          <div style={{ paddingTop: 4 }}>
            <button type="button" style={L.leaveButton} disabled={busy} onClick={onLeave}>
              Выйти из пула
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

/**
 * Блок-вопрос о свежести под кнопкой источника (§8).
 *
 * Возврат из браузера не ловим: блок появляется сразу после тапа по ссылке и
 * висит, пока не ответят или не уйдут с карточки. Пришедшему с готовым голосом
 * он показывается сразу — «Ты отметил: …», без ожидания тапа.
 */
function VoteBlock({
  vote,
  onVote,
  onChangeVote,
}: {
  vote: FlatVoteState;
  onVote?: (alive: boolean) => void;
  onChangeVote?: () => void;
}) {
  if (vote.myVote === null) {
    return (
      <div style={L.voteBlock}>
        <span style={{ fontSize: 14, fontWeight: 590 }}>Объявление ещё живое?</span>
        <div style={{ display: 'flex', gap: 8 }}>
          <button
            type="button"
            style={L.voteButton}
            disabled={vote.busy}
            onClick={() => onVote?.(true)}
          >
            Да
          </button>
          <button
            type="button"
            style={{ ...L.voteButton, color: 'var(--tg-theme-destructive-text-color)' }}
            disabled={vote.busy}
            onClick={() => onVote?.(false)}
          >
            Уже сдана
          </button>
        </div>
        <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
          Помоги остальным в пуле — сданные мы скрываем
        </span>
      </div>
    );
  }

  const line = vote.justVoted ? 'Спасибо, отметили' : `Ты отметил: ${vote.myVote ? 'живая' : 'сдана'}`;
  return (
    <div style={{ ...L.voteBlock, flexDirection: 'row', alignItems: 'center', gap: 6 }}>
      <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>{line} ·</span>
      <button type="button" style={L.changeLink} disabled={vote.busy} onClick={onChangeVote}>
        изменить
      </button>
    </div>
  );
}
