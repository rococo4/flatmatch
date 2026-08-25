/* Пограничные состояния — §6 / §2.3.
   1) шторки блокировки, удаления мэтча и жалобы (§5, §2.3)
   2) шторка закрытия поиска с причиной
   3) гейт второго «живу» в форме «к себе»
   4) шторки правки анкеты: ответ вопроса, «о себе», возраст (§6, экран «Анкета») */
import { useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import type { CloseReason, ReportReason } from '../api/client';
import { reportReasonOptions } from '../api/format';
import { ABOUT_HINT, choiceRowStyle, type Choice } from './OnboardingScreens';

const E = {
  scrim: {
    // Шторка обязана лежать поверх всего, включая таб-бар: иначе кнопка,
    // которая её открыла, выглядит мёртвой — шторка уезжает в конец потока
    // и прячется под баром.
    position: 'fixed',
    inset: 0,
    zIndex: 20,
    display: 'flex',
    alignItems: 'flex-end',
    justifyContent: 'center',
    padding: 10,
    paddingBottom: 'max(10px, env(safe-area-inset-bottom))',
    background: 'rgba(0, 0, 0, 0.4)',
  } as CSSProperties,
  sheet: {
    width: '100%',
    // Тот же потолок ширины, что у экранов: на десктопе шторка во всю
    // ширину монитора выглядит сломанной.
    maxWidth: 520,
    borderRadius: 16,
    padding: '16px 14px 14px',
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
  } as CSSProperties,
  grabber: {
    alignSelf: 'center',
    width: 36,
    height: 4,
    borderRadius: 2,
    background: 'var(--chip-bg)',
  } as CSSProperties,
  stay: {
    flex: '1 1 0',
    padding: 13,
    border: 'none',
    borderRadius: 11,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  sheetRow: {
    width: '100%',
    padding: '13px 14px',
    border: 'none',
    borderRadius: 11,
    background: 'var(--app-bg)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-text-color)',
    textAlign: 'left',
    cursor: 'pointer',
  } as CSSProperties,
  leave: {
    flex: '1 1 0',
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
  /** Чип причины жалобы: выбранный обведён акцентом, а не залит. */
  reasonChip: (picked: boolean): CSSProperties => ({
    padding: '9px 12px',
    borderRadius: 10,
    border: picked ? '1.5px solid var(--tg-theme-button-color)' : '1px solid var(--sep)',
    background: picked ? 'var(--chip-bg)' : 'var(--app-bg)',
    font: 'inherit',
    fontSize: 14,
    fontWeight: 510,
    color: picked ? 'var(--tg-theme-accent-text-color)' : 'var(--tg-theme-text-color)',
    cursor: 'pointer',
  }),
  field: {
    width: '100%',
    padding: '12px 13px',
    borderRadius: 11,
    border: 'none',
    background: 'var(--app-bg)',
    font: 'inherit',
    fontSize: 16,
    fontWeight: 590,
    color: 'var(--tg-theme-text-color)',
    outline: 'none',
    boxSizing: 'border-box',
  } as CSSProperties,
  sheetTitle: {
    fontSize: 19,
    fontWeight: 640,
    letterSpacing: '-0.4px',
    textWrap: 'pretty',
  } as CSSProperties,
  sheetWhy: {
    fontSize: 13,
    color: 'var(--tg-theme-subtitle-text-color)',
    textWrap: 'pretty',
  } as CSSProperties,
  error: { fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' } as CSSProperties,
  send: (disabled: boolean): CSSProperties => ({
    flex: '1 1 0',
    padding: 13,
    border: 'none',
    borderRadius: 11,
    background: disabled ? 'var(--chip-bg)' : 'var(--tg-theme-button-color)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: disabled ? 'var(--tg-theme-hint-color)' : 'var(--tg-theme-button-text-color)',
    cursor: disabled ? 'default' : 'pointer',
  }),
};

/**
 * Общая оболочка шторок: затемнение на весь экран и панель снизу.
 *
 * Одна на все три шторки намеренно: они отличаются содержимым, а не поведением,
 * и по-разному всплывать не должны. Тап по затемнению — отказ (у каждой шторки
 * он свой и всегда безопасный: «остаться», «отмена», «не отвечать»).
 */
function SheetShell({ onDismiss, children }: { onDismiss?: () => void; children: ReactNode }) {
  return (
    <div style={E.scrim} onClick={onDismiss}>
      {/* Тап внутри панели не должен закрывать шторку. */}
      <div style={{ ...E.sheet, gap: 14 }} onClick={(e) => e.stopPropagation()}>
        {children}
      </div>
    </div>
  );
}

/**
 * Блокировка человека (§5).
 *
 * Про необратимость сказано прямо в шторке: разблокировки в продукте нет ни
 * кнопкой, ни ручкой API, и узнавать об этом после тапа человек не должен.
 */
export function BlockPersonSheet({
  name,
  note,
  busy,
  error,
  onCancel,
  onBlock,
}: {
  name: string;
  /**
   * Что ещё случится вместе с блоком — «Мэтч тоже удалится» из мэтча (§5).
   *
   * Второй строкой, а не вместо предупреждения: невидимость обоюдна везде,
   * и это правило одно для всех мест, откуда блокируют.
   */
  note?: string;
  busy?: boolean;
  /** Отказ сервера: шторка остаётся на месте, чтобы тап можно было повторить. */
  error?: string | null;
  onCancel?: () => void;
  onBlock?: () => void;
}) {
  return (
    <SheetShell onDismiss={busy ? undefined : onCancel}>
      <span style={E.grabber} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px', textWrap: 'pretty' }}>
          Заблокировать {name}?
        </span>
        <span
          style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)', textWrap: 'pretty' }}
        >
          Вы перестанете видеть друг друга везде — в пулах, лайках и пушах. Отменить нельзя.
        </span>
        {note && (
          <span
            style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)', textWrap: 'pretty' }}
          >
            {note}
          </span>
        )}
      </div>
      {error && (
        <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>{error}</span>
      )}
      <div style={{ display: 'flex', gap: 8 }}>
        {/* «Отмена» гаснет вместе с красной: пока запрос в пути, шторка не
            должна закрываться под уже улетевшей блокировкой. */}
        <button type="button" style={E.stay} disabled={busy} onClick={onCancel}>
          Отмена
        </button>
        <button
          type="button"
          style={{ ...E.leave, opacity: busy ? 0.6 : 1 }}
          disabled={busy}
          onClick={onBlock}
        >
          Заблокировать
        </button>
      </div>
    </SheetShell>
  );
}

/**
 * Удаление мэтча (§2.3).
 *
 * Про сгорание лайков сказано прямо: пара снова увидит друг друга в пулах, но
 * повторный лайк снова стоит слота — узнавать об этом после тапа поздно.
 * Второй стороне ничего не уходит, поэтому и спрашиваем только один раз.
 */
export function UnmatchSheet({
  name,
  busy,
  error,
  onCancel,
  onConfirm,
}: {
  name: string;
  busy?: boolean;
  /** Отказ сервера: шторка остаётся на месте, чтобы тап можно было повторить. */
  error?: string | null;
  onCancel?: () => void;
  onConfirm?: () => void;
}) {
  return (
    <SheetShell onDismiss={busy ? undefined : onCancel}>
      <span style={E.grabber} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
        <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px', textWrap: 'pretty' }}>
          Удалить мэтч с {name}?
        </span>
        <span
          style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)', textWrap: 'pretty' }}
        >
          Он пропадёт у обоих, лайки сгорят, и вы снова увидите друг друга в пулах. Отменить нельзя.
        </span>
      </div>
      {error && (
        <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>{error}</span>
      )}
      <div style={{ display: 'flex', gap: 8 }}>
        {/* «Отмена» гаснет вместе с красной: пока запрос в пути, шторка не
            должна закрываться под уже улетевшим удалением. */}
        <button type="button" style={E.stay} disabled={busy} onClick={onCancel}>
          Отмена
        </button>
        <button
          type="button"
          style={{ ...E.leave, opacity: busy ? 0.6 : 1 }}
          disabled={busy}
          onClick={onConfirm}
        >
          Удалить
        </button>
      </div>
    </SheetShell>
  );
}

/**
 * Жалоба модератору (§5): чипы причин, отправка, и только потом — блокировка.
 *
 * Жалоба и блок разведены намеренно: разбор нужен модератору, а невидимость —
 * пожаловавшемуся, и одно не подразумевает другое. Поэтому после отправки
 * шторка не закрывается сама, а предлагает второй жест отдельной кнопкой.
 */
export function ReportPersonSheet({
  name,
  sent,
  busy,
  error,
  onSend,
  onBlock,
  onClose,
}: {
  name: string;
  /** Жалоба ушла: вместо чипов — благодарность и предложение заблокировать. */
  sent?: boolean;
  busy?: boolean;
  error?: string | null;
  onSend?: (reason: ReportReason) => void;
  onBlock?: () => void;
  onClose?: () => void;
}) {
  const [reason, setReason] = useState<ReportReason | null>(null);

  if (sent) {
    return (
      <SheetShell onDismiss={onClose}>
        <span style={E.grabber} />
        <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px' }}>
          Спасибо, разберёмся
        </span>
        <button type="button" style={{ ...E.leave, flex: '0 0 auto' }} onClick={onBlock}>
          Заблокировать {name}
        </button>
        <button type="button" style={E.stay} onClick={onClose}>
          Готово
        </button>
      </SheetShell>
    );
  }

  return (
    <SheetShell onDismiss={busy ? undefined : onClose}>
      <span style={E.grabber} />
      <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px', textWrap: 'pretty' }}>
        Пожаловаться на {name}?
      </span>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
        {reportReasonOptions().map((r) => (
          <button
            key={r.value}
            type="button"
            style={E.reasonChip(reason === r.value)}
            disabled={busy}
            onClick={() => setReason(r.value)}
          >
            {r.label}
          </button>
        ))}
      </div>
      {error && (
        <span style={{ fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' }}>{error}</span>
      )}
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" style={E.stay} disabled={busy} onClick={onClose}>
          Отмена
        </button>
        <button
          type="button"
          style={E.send(reason === null || Boolean(busy))}
          disabled={reason === null || busy}
          onClick={() => reason && onSend?.(reason)}
        >
          Отправить
        </button>
      </div>
    </SheetShell>
  );
}

/** Причины закрытия поиска (§3). Порядок не менять: «нашёл здесь» — ГЛАВНАЯ МЕТРИКА. */
const CLOSE_REASONS: { reason: CloseReason; label: string }[] = [
  { reason: 'found_here', label: 'Нашёл здесь' },
  { reason: 'found_elsewhere', label: 'Нашёл в другом месте' },
  { reason: 'changed_mind', label: 'Передумал' },
];

/**
 * Закрытие поиска — только с причиной.
 *
 * Причина обязательна не ради статистики вообще, а ради одной цифры: «нашёл
 * здесь» и есть метрика успеха продукта, поэтому тихого «просто закрыть» тут нет.
 */
export function CloseSearchSheet({
  onPick,
  onCancel,
}: {
  onPick?: (reason: CloseReason) => void;
  onCancel?: () => void;
}) {
  return (
    <SheetShell onDismiss={onCancel}>
      <span style={E.grabber} />
      <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px', textWrap: 'pretty' }}>
        Почему закрываешь поиск?
      </span>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {CLOSE_REASONS.map((r) => (
          <button key={r.reason} type="button" style={E.sheetRow} onClick={() => onPick?.(r.reason)}>
            {r.label}
          </button>
        ))}
      </div>
      <button type="button" style={E.stay} onClick={onCancel}>
        Отмена
      </button>
    </SheetShell>
  );
}

/**
 * Ответ на вопрос анкеты из экрана правки (§6).
 *
 * Варианты — те же строки, что в онбординге: вопрос один, и переписывать его
 * для правки нельзя. Тап сохраняет сразу, поэтому кнопки «Сохранить» здесь нет,
 * а закрывает шторку ответ сервера, а не сам тап: иначе список показывал бы
 * выбор, которого в базе нет.
 */
export function ChoiceSheet({
  title,
  why,
  choices,
  value,
  pending,
  error,
  onPick,
  onCancel,
}: {
  title: string;
  /** Зачем спрашиваем — подпись вопроса из §4. Пусто у «Пола»: там объяснять нечего. */
  why?: string;
  choices: Choice[];
  /** Ответ с сервера. null — на вопрос ещё не отвечали. */
  value: string | null;
  /** Что улетело: галочка стоит на нём, пока сервер молчит. */
  pending?: string | null;
  error?: string | null;
  onPick?: (id: string) => void;
  onCancel?: () => void;
}) {
  const busy = Boolean(pending);
  const picked = pending ?? value;

  return (
    <SheetShell onDismiss={busy ? undefined : onCancel}>
      <span style={E.grabber} />
      <div style={{ display: 'flex', flexDirection: 'column', gap: 5 }}>
        <span style={E.sheetTitle}>{title}</span>
        {why && <span style={E.sheetWhy}>{why}</span>}
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 7, opacity: busy ? 0.6 : 1 }}>
        {choices.map((c) => (
          <div
            key={c.id}
            style={choiceRowStyle(picked === c.id)}
            onClick={busy ? undefined : () => onPick?.(c.id)}
          >
            {c.label}
            <span
              style={{
                marginLeft: 'auto',
                fontSize: 15,
                fontWeight: 700,
                opacity: picked === c.id ? 1 : 0,
              }}
            >
              ✓
            </span>
          </div>
        ))}
      </div>
      {error && <span style={E.error}>{error}</span>}
      <button type="button" style={E.stay} disabled={busy} onClick={onCancel}>
        Отмена
      </button>
    </SheetShell>
  );
}

/**
 * «О себе» — единственный свободный текст продукта (§4), ≤200 знаков.
 *
 * Кнопка «Сохранить», а не сохранение по букве: текст уходит на перепроверку
 * модератору (§5), и отправлять туда каждую правку по одному символу нельзя.
 */
export function AboutSheet({
  text,
  max = 200,
  busy,
  error,
  onSave,
  onCancel,
}: {
  text: string;
  max?: number;
  busy?: boolean;
  error?: string | null;
  onSave?: (next: string) => void;
  onCancel?: () => void;
}) {
  const [draft, setDraft] = useState(text);
  const empty = draft.trim().length === 0;

  return (
    <SheetShell onDismiss={busy ? undefined : onCancel}>
      <span style={E.grabber} />
      <span style={E.sheetTitle}>О себе</span>
      <textarea
        value={draft}
        maxLength={max}
        disabled={busy}
        onChange={(e) => setDraft(e.target.value)}
        style={{ ...E.field, minHeight: 104, fontWeight: 400, lineHeight: 1.45, resize: 'none' }}
      />
      <div style={{ display: 'flex', alignItems: 'flex-start', gap: 10 }}>
        <span style={{ ...E.sheetWhy, flex: '1 1 auto', minWidth: 0 }}>{ABOUT_HINT}</span>
        <span
          style={{
            flex: '0 0 auto',
            fontSize: 12,
            color: 'var(--tg-theme-hint-color)',
            fontVariantNumeric: 'tabular-nums',
          }}
        >
          {draft.length}/{max}
        </span>
      </div>
      {error && <span style={E.error}>{error}</span>}
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" style={E.stay} disabled={busy} onClick={onCancel}>
          Отмена
        </button>
        <button
          type="button"
          style={E.send(empty || Boolean(busy))}
          disabled={empty || busy}
          onClick={() => onSave?.(draft.trim())}
        >
          Сохранить
        </button>
      </div>
    </SheetShell>
  );
}

/** Возраст: то же поле, что на первом шаге анкеты, те же границы 16–100. */
export function AgeSheet({
  age,
  busy,
  error,
  onSave,
  onCancel,
}: {
  /** Пусто — возраст ещё не называли. */
  age?: number;
  busy?: boolean;
  error?: string | null;
  onSave?: (next: number) => void;
  onCancel?: () => void;
}) {
  const [draft, setDraft] = useState(age ? String(age) : '');
  const value = Number(draft);
  const ok = draft !== '' && value >= 16 && value <= 100;

  return (
    <SheetShell onDismiss={busy ? undefined : onCancel}>
      <span style={E.grabber} />
      <span style={E.sheetTitle}>Возраст</span>
      <input
        style={E.field}
        inputMode="numeric"
        placeholder="23"
        disabled={busy}
        value={draft}
        onChange={(e) => setDraft(e.target.value.replace(/\D/g, '').slice(0, 3))}
      />
      {error && <span style={E.error}>{error}</span>}
      <div style={{ display: 'flex', gap: 8 }}>
        <button type="button" style={E.stay} disabled={busy} onClick={onCancel}>
          Отмена
        </button>
        <button
          type="button"
          style={E.send(!ok || Boolean(busy))}
          disabled={!ok || busy}
          onClick={() => ok && onSave?.(value)}
        >
          Сохранить
        </button>
      </div>
    </SheetShell>
  );
}

/** Гейт стадии в форме «к себе»: вторая точка «живу здесь» невозможна. */
export function HomeStageGate({
  moving,
  onPickMoving,
}: {
  /** Выбрано «заезжаю»: рядом есть третий ответ гейта, и выбор может уйти к нему. */
  moving: boolean;
  onPickMoving?: () => void;
}) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: 11,
          padding: '13px 12px',
          borderRadius: 12,
          background: 'var(--card-bg)',
          border: '1px solid var(--sep)',
          opacity: 0.55,
          cursor: 'not-allowed',
        }}
      >
        <span
          style={{
            flex: '0 0 auto',
            width: 20,
            height: 20,
            borderRadius: '50%',
            boxSizing: 'border-box',
            border: '1.5px solid color-mix(in srgb, var(--tg-theme-hint-color) 60%, transparent)',
          }}
        />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
          <span style={{ fontSize: 15, fontWeight: 590 }}>Уже живу здесь</span>
          <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
            ты уже зовёшь к себе там, где живёшь
          </span>
        </div>
      </div>

      <div
        onClick={onPickMoving}
        style={{
          display: 'flex',
          alignItems: 'flex-start',
          gap: 11,
          padding: '13px 12px',
          borderRadius: 12,
          background: 'var(--card-bg)',
          border: moving ? '1.5px solid var(--type-home-fg)' : '1px solid var(--sep)',
          cursor: 'pointer',
        }}
      >
        <span
          style={{
            flex: '0 0 auto',
            width: 20,
            height: 20,
            borderRadius: '50%',
            boxSizing: 'border-box',
            border: moving
              ? '6px solid var(--type-home-fg)'
              : '1.5px solid color-mix(in srgb, var(--tg-theme-hint-color) 70%, transparent)',
            background: moving ? 'var(--card-bg)' : 'transparent',
            marginTop: 1,
          }}
        />
        <div style={{ display: 'flex', flexDirection: 'column', gap: 3, minWidth: 0 }}>
          <span style={{ fontSize: 15, fontWeight: 590 }}>Договорился и заезжаю</span>
          <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
            квартира есть, ищу соседа со старта
          </span>
          <span
            style={{
              marginTop: 2,
              alignSelf: 'flex-start',
              padding: '3px 8px',
              borderRadius: 7,
              background: 'var(--type-home-bg)',
              color: 'var(--type-home-fg)',
              fontSize: 12,
              fontWeight: 590,
            }}
          >
            заезжает · ищет со-съёмщика со старта
          </span>
        </div>
      </div>
    </div>
  );
}
