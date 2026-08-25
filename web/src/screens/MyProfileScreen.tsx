/* Профиль свой — §6. Статус анкеты, верификация, мэтчи.
   Входящие лайки живут своей вкладкой (§4 дизайна редизайна), и второго входа
   к ним отсюда нет: бейдж на «Лайках» и есть их место. */
import type { CSSProperties } from 'react';
import type { ModerationStatus, Profile } from '../api/client';
import { ABOUT_REJECTED, PHOTO_REJECTED } from '../api/format';
import { Avatar, S } from '../ui/primitives';

/**
 * Строка «Моих мэтчей».
 *
 * Контакта здесь нет намеренно: @username живёт только на экране самого
 * мэтча (§3), сюда он и не приезжает — строка ведёт туда тапом.
 */
export type MyMatch = {
  /** id мэтча: по нему открывается саммари с контактом. */
  id: string;
  name: string;
  age: number;
  /** Пусто = фото не одобрено: у аватара заглушка с инициалами (§5). */
  photoUrl?: string;
  /** Зона контекста: «Хамовники». Пусто — контекста не осталось. */
  context?: string;
  when: string; // «мэтч сегодня»
};

/** Что чинить по тапу в строку статуса: шаг анкеты решает оболочка. */
export type FixTarget = 'photo' | 'about' | 'draft';

/** Строка статуса анкеты: что не так и куда это чинить. Пусто — всё в порядке. */
type Notice = { title: string; aside?: string; fix?: FixTarget };

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
    gap: 14,
    padding: '14px 12px 20px',
  } as CSSProperties,
  block: {
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    padding: 12,
    borderRadius: 14,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
  } as CSSProperties,
  statusBanner: {
    display: 'flex',
    alignItems: 'center',
    gap: 9,
    padding: '10px 11px',
    borderRadius: 11,
    background: 'var(--warning-bg)',
    color: 'var(--warning-fg)',
  } as CSSProperties,
  secondary: {
    width: '100%',
    padding: 12,
    border: 'none',
    borderRadius: 10,
    background: 'var(--app-bg)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
  okChip: {
    marginLeft: 'auto',
    flex: '0 0 auto',
    padding: '4px 9px',
    borderRadius: 8,
    background: 'var(--verdict-ok-bg)',
    color: 'var(--verdict-ok-fg)',
    fontSize: 13,
    fontWeight: 590,
  } as CSSProperties,
};

export function MyProfileScreen({
  name,
  age,
  occupation,
  photoUrl,
  status,
  photoStatus,
  aboutStatus,
  emailKind,
  matches,
  onEdit,
  onFix,
  onOpenMatch,
}: {
  name: string;
  age: number;
  occupation: string;
  /** Своё фото: показывается только одобренное, иначе заглушка (§5). */
  photoUrl?: string;
  status: Profile['status'];
  photoStatus: ModerationStatus;
  aboutStatus: ModerationStatus;
  /** «Учебная почта» студенту, «Рабочая почта» работающему */
  emailKind: string;
  matches: MyMatch[];
  onEdit?: () => void;
  /** Тап в строку статуса: увести чинить ровно то, что не прошло. */
  onFix?: (target: FixTarget) => void;
  onOpenMatch?: (id: string) => void;
}) {
  const notice = noticeOf(status, photoStatus, aboutStatus);
  const fix = notice?.fix;

  return (
    <div style={L.screen}>
      <div style={L.header}>
        <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Профиль</span>
      </div>

      <div style={L.body}>
        <div style={{ ...L.block, gap: 12, padding: '14px 12px' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 14 }}>
            <Avatar name={name} size={72} src={photoStatus === 'ok' ? photoUrl : undefined} />
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4, minWidth: 0 }}>
              <span style={{ fontSize: 20, fontWeight: 640, letterSpacing: '-0.4px' }}>
                {name}, {age}
              </span>
              <span style={{ fontSize: 14, color: 'var(--tg-theme-subtitle-text-color)' }}>
                {occupation}
              </span>
            </div>
          </div>

          {notice && (
            // Строка с правкой — кнопка, а не текст: реджект без дороги назад
            // оставляет человека вне выдачи навсегда (§5).
            <div
              role={fix ? 'button' : undefined}
              style={{ ...L.statusBanner, cursor: fix ? 'pointer' : 'default' }}
              onClick={fix ? () => onFix?.(fix) : undefined}
            >
              <span style={{ flex: '1 1 auto', fontSize: 13, fontWeight: 590, lineHeight: 1.35 }}>
                {notice.title}
              </span>
              {notice.aside && (
                <span style={{ flex: '0 0 auto', fontSize: 12, opacity: 0.85 }}>{notice.aside}</span>
              )}
              {fix && <span style={{ flex: '0 0 auto', fontSize: 17, lineHeight: 1 }}>›</span>}
            </div>
          )}

          <button type="button" style={L.secondary} onClick={onEdit}>
            Редактировать
          </button>
        </div>

        <div style={L.block}>
          <span style={S.blockTitle}>Верификация</span>

          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0 }}>
              <span style={{ fontSize: 15, fontWeight: 510 }}>{emailKind}</span>
              <span style={{ fontSize: 12, color: 'var(--tg-theme-hint-color)' }}>
                бейдж вуза в карточках и подборе
              </span>
            </div>
            {/* Ручки верификации ещё нет: кнопка появится вместе с ней, мёртвой
                висеть не должна. */}
            <span
              style={{
                marginLeft: 'auto',
                flex: '0 0 auto',
                fontSize: 13,
                color: 'var(--tg-theme-hint-color)',
              }}
            >
              скоро
            </span>
          </div>

          <div style={{ height: 1, background: 'var(--sep)' }} />

          <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <span style={{ fontSize: 15, fontWeight: 510 }}>Telegram</span>
            <span style={L.okChip}>✓ есть</span>
          </div>
        </div>

        {/* Счётчика у пустой секции нет: «· 0» — шум, а строка под ней говорит
            то же самое словами и заодно называет ближайший путь к первому
            мэтчу (§6, пустые состояния). */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <span style={S.sectionTitle}>
            МОИ МЭТЧИ{matches.length > 0 ? ` · ${matches.length}` : ''}
          </span>
          {matches.length === 0 && (
            <span
              style={{
                fontSize: 14,
                color: 'var(--tg-theme-subtitle-text-color)',
                textWrap: 'pretty',
              }}
            >
              Мэтчей пока нет — ответный лайк на входящий даёт первый
            </span>
          )}
          {matches.map((m) => (
            <div
              key={m.id}
              style={{
                ...L.block,
                flexDirection: 'row',
                alignItems: 'center',
                gap: 12,
                cursor: 'pointer',
              }}
              onClick={() => onOpenMatch?.(m.id)}
            >
              <Avatar name={m.name} size={44} src={m.photoUrl} />
              <div style={{ display: 'flex', flexDirection: 'column', gap: 2, minWidth: 0 }}>
                <span style={{ fontSize: 15, fontWeight: 590 }}>
                  {m.name}, {m.age}
                  {m.context ? ` · ${m.context}` : ''}
                </span>
                <span style={{ fontSize: 13, color: 'var(--tg-theme-hint-color)' }}>{m.when}</span>
              </div>
              <span
                style={{
                  marginLeft: 'auto',
                  fontSize: 17,
                  color: 'var(--tg-theme-hint-color)',
                  lineHeight: 1,
                }}
              >
                ›
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

/**
 * Правило строки статуса (§5, §6).
 *
 * Статус аккаунта и вердикты по фото и «о себе» ходят врозь, и схлопывать их в
 * один нельзя: не прошедшего проверку реджект возвращает в draft, а одобренный
 * остаётся active — с заглушкой вместо фото и той же строкой. Порядок веток —
 * по цене правки: фото обязательно (§4), без него человека не видно нигде.
 */
function noticeOf(
  status: Profile['status'],
  photo: ModerationStatus,
  about: ModerationStatus,
): Notice | null {
  if (photo === 'rejected') return { title: PHOTO_REJECTED, fix: 'photo' };
  if (about === 'rejected') return { title: ABOUT_REJECTED, fix: 'about' };
  if (status === 'draft') return { title: 'заполни анкету', fix: 'draft' };
  if (status === 'pending') return { title: 'анкета на проверке', aside: 'обычно до 2 часов' };
  if (status === 'active') {
    // Перезалитое фото и переписанный текст ждут перепроверки поодиночке —
    // статус у человека при этом не меняется (§5).
    if (photo === 'pending') return { title: 'новое фото на проверке' };
    if (about === 'pending') return { title: 'текст на проверке' };
  }
  return null;
}
