/* Контейнер «Это мэтч»: саммари «почему совпали» и единственный способ
   связаться во всём продукте (§3, §6).

   Мэтч приватен для пары и живёт, пока одна из сторон его не удалит (§2.3);
   контакт может быть снят сервером (бан пира) — тогда кнопки нет вовсе, а не
   ссылка в никуда.

   GET ничего не мутирует, поэтому ref-guard ему не нужен: второй прогон
   эффекта в StrictMode стоит лишнего запроса, а не испорченного состояния.
   А вот удаление и блок защищены флагом busy: второй тап ушёл бы вторым
   запросом за уже улетевшим действием. */
import { useEffect, useState } from 'react';
import type { CSSProperties } from 'react';
import { openTelegramLink } from '@telegram-apps/sdk-react';

import {
  ApiError,
  blockPerson,
  deleteMatch,
  fetchMatch,
  type MatchDetail,
} from '../api/client';
import { fieldLabel, overlapRow } from '../api/format';
import { launchedByTelegram } from '../telegram/mockEnv';
import { BlockPersonSheet, UnmatchSheet } from './EdgeStates';
import { MatchScreen } from './MatchScreen';

/** Контакт мэтча: подпись кнопки, куда она ведёт и нужна ли оговорка. */
type Contact = { label: string; url: string; hint?: string };

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

export function MatchView({
  matchId,
  onBack,
  onRemoved,
  onToFeed,
}: {
  matchId: string;
  /**
   * Выход, пока саммари не приехало.
   *
   * У готового экрана уход есть («К ленте»), а у загрузки и ошибки своего не
   * было бы: таб-бар оболочка на этом экране не рисует, и сорвавшийся запрос
   * запирал бы человека до перезапуска приложения.
   */
  onBack?: () => void;
  /**
   * Мэтча больше нет — удалили или заблокировали пира (§2.3, §5).
   *
   * Смотреть на экране нечего: запрос по этому id ответил бы 404, — уводит и
   * перечитывает списки оболочка.
   */
  onRemoved?: () => void;
  onToFeed?: () => void;
}) {
  const [match, setMatch] = useState<MatchDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  /** Открытая шторка удаления или блока: оба жеста необратимы и оба спрашивают. */
  const [sheet, setSheet] = useState<'unmatch' | 'block' | null>(null);
  const [busy, setBusy] = useState(false);
  /** Отказ сервера живёт в шторке: экран мэтча под ней остаётся целым. */
  const [sheetError, setSheetError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setMatch(null);
    setError(null);
    fetchMatch(matchId)
      .then((data) => {
        if (!cancelled) setMatch(data);
      })
      .catch((e: unknown) => {
        if (!cancelled) setError(message(e));
      });
    return () => {
      cancelled = true;
    };
  }, [matchId]);

  /** Удаление мэтча (§2.3): стирает и мэтч, и оба лайка пары. */
  const remove = async () => {
    if (busy) return;
    setBusy(true);
    setSheetError(null);
    try {
      await deleteMatch(matchId);
      setSheet(null);
      onRemoved?.();
    } catch (e: unknown) {
      setSheetError(message(e));
    } finally {
      setBusy(false);
    }
  };

  /** Блок из мэтча (§5): та же обоюдная невидимость, мэтч уходит вместе с ней. */
  const block = async () => {
    if (busy || !match) return;
    setBusy(true);
    setSheetError(null);
    try {
      await blockPerson(match.peer.user_id);
      setSheet(null);
      onRemoved?.();
    } catch (e: unknown) {
      setSheetError(message(e));
    } finally {
      setBusy(false);
    }
  };

  const openSheet = (which: 'unmatch' | 'block') => {
    setSheetError(null);
    setSheet(which);
  };

  if (error || !match) {
    return (
      <div style={V.screen}>
        <div style={V.header}>
          <span style={V.back} onClick={onBack}>
            ‹
          </span>
          <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Мэтч</span>
        </div>
        <div style={V.centered}>{error ?? 'Секунду…'}</div>
      </div>
    );
  }

  const contact = contactOf(match);

  return (
    <>
      <MatchScreen
        name={match.peer.name}
        age={match.peer.age}
        photoUrl={match.peer.photo_url || undefined}
        // Контекст исторический: зона или точка, на которой пересеклись, даже
        // если ни поиска, ни самой точки давно нет (§2.3). Взаимоисключимы —
        // лайк несёт один контекст; у ответного нет и его. Метка точки приезжает
        // готовой («на комнату Кати · м. Сокол»), зона — со своим предлогом
        // («на Арбате»): называть место механикой («в подборе») нечестно —
        // пересеклись они в районе, а не в интерфейсе.
        where={match.where ? `пересеклись ${match.where.name_in}` : match.where_point?.label}
        params={match.params.flatMap((o) => overlapRow(o) ?? [])}
        // Метки анкеты — строчные, как все чипы продукта: «сова», «не курю».
        profileMatches={match.profile.map((s) => fieldLabel(s.field, s.value))}
        contact={contact && { label: contact.label, hint: contact.hint }}
        onWrite={() => {
          if (contact) openContact(contact.url);
        }}
        onUnmatch={() => openSheet('unmatch')}
        onBlock={() => openSheet('block')}
        onToFeed={onToFeed}
      />
      {sheet === 'unmatch' && (
        <UnmatchSheet
          name={match.peer.name}
          busy={busy}
          error={sheetError}
          onCancel={() => setSheet(null)}
          onConfirm={() => void remove()}
        />
      )}
      {sheet === 'block' && (
        <BlockPersonSheet
          name={match.peer.name}
          // Блок из любого места стирает мэтч и лайки пары — одно правило, не
          // два (§5): здесь об этом надо сказать до тапа.
          note="Мэтч тоже удалится"
          busy={busy}
          error={sheetError}
          onCancel={() => setSheet(null)}
          onBlock={() => void block()}
        />
      )}
    </>
  );
}

/**
 * Куда писать: @username, иначе — прямая ссылка по числовому id.
 *
 * tg_id проверяется «больше нуля», а не «поле есть»: у скрытого контакта
 * сервер обнуляет оба поля, и `tg://user?id=0` открыл бы пустоту. Ссылка по
 * id срабатывает не всегда (приватность в Telegram) — потому и оговорка.
 */
function contactOf(m: MatchDetail): Contact | null {
  if (m.peer.username) {
    return { label: `Написать @${m.peer.username}`, url: `https://t.me/${m.peer.username}` };
  }
  if (m.peer.tg_id > 0) {
    return {
      label: 'Написать в Telegram',
      url: `tg://user?id=${m.peer.tg_id}`,
      hint: 'Если чат не открылся — у него скрыт профиль, он напишет тебе первым',
    };
  }
  return null;
}

/**
 * Открывает личку.
 *
 * t.me умеет открыть сам клиент — родным вызовом, но только в настоящем
 * клиенте: под моком разработки isAvailable() отвечает «да», а вызов уходит в
 * перехваченный postMessage, и по кнопке не происходило бы ничего.
 * `tg://user?id=…` родной вызов не принимает (он ждёт hostname t.me), поэтому
 * такую ссылку отдаём навигацией — клиент перехватит её сам.
 */
function openContact(url: string): void {
  if (launchedByTelegram() && url.startsWith('https://t.me/') && openTelegramLink.isAvailable()) {
    openTelegramLink(url);
    return;
  }
  if (url.startsWith('tg://')) {
    window.location.href = url;
    return;
  }
  window.open(url, '_blank', 'noopener');
}

function message(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}
