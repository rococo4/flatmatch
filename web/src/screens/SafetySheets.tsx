/* Жалоба и блокировка (§5) — один узел на все экраны с чужой анкетой.

   Жест один и тот же, откуда бы его ни звали (анкета из подбора, анкета
   лайкнувшего), и разъехаться он не должен: и текст предупреждения, и порядок
   «сначала разбор, потом невидимость» — часть правила, а не экрана.

   Какая шторка открыта, решает экран: под открытой он прячет свою нижнюю
   кнопку (родную MainButton поверх затемнения рисует сам Telegram). А вот
   полёт запроса и его отказ живут здесь: экрану про них знать нечего. */
import { useEffect, useState } from 'react';

import { ApiError, blockPerson, reportPerson, type ReportReason } from '../api/client';
import { BlockPersonSheet, ReportPersonSheet } from './EdgeStates';

export type SafetySheet = 'report' | 'block';

export function SafetySheets({
  userId,
  name,
  zoneId,
  pointId,
  open,
  onOpen,
  onClose,
  onBlocked,
}: {
  userId: string;
  name: string;
  /** Откуда жалуются: модератору без места непонятно, где эти двое встретились. */
  zoneId?: string;
  pointId?: string;
  open: SafetySheet | null;
  /** «Заблокировать {имя}» после жалобы ведёт во вторую шторку — через экран. */
  onOpen: (which: SafetySheet) => void;
  onClose: () => void;
  /**
   * Заблокирован (§5). Смотреть на экране больше нечего: цель стала невидимой,
   * и повторный запрос её анкеты ответил бы 404 — уводит оболочка.
   */
  onBlocked?: () => void;
}) {
  const [busy, setBusy] = useState(false);
  /** Отказ жалобы или блока: он живёт в шторке, а не под кнопкой лайка. */
  const [error, setError] = useState<string | null>(null);
  /** Жалоба ушла: шторка меняется на «Спасибо, разберёмся». */
  const [reported, setReported] = useState(false);

  // Смена шторки — это новый заход: прошлый отказ к нему не относится, а
  // «Спасибо, разберёмся» держится ровно до закрытия жалобы.
  useEffect(() => {
    setError(null);
    if (open === 'report') setReported(false);
  }, [open]);

  /**
   * Жалоба (§5). Контекст — то место, откуда открыта анкета; нет контекста —
   * уходит без него, а не с выдуманным.
   */
  const report = async (reason: ReportReason) => {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await reportPerson(userId, reason, { zoneId, pointId });
      setReported(true);
    } catch (e: unknown) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  /** Блокировка (§5): обоюдная невидимость, отменить её нечем. */
  const block = async () => {
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      await blockPerson(userId);
      onClose();
      onBlocked?.();
    } catch (e: unknown) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      {open === 'report' && (
        <ReportPersonSheet
          name={name}
          sent={reported}
          busy={busy}
          error={error}
          onSend={(reason) => void report(reason)}
          // «Заблокировать» после жалобы ведёт в ту же шторку с
          // подтверждением: невидимость необратима, и спросить надо один раз.
          onBlock={() => onOpen('block')}
          onClose={onClose}
        />
      )}
      {open === 'block' && (
        <BlockPersonSheet
          name={name}
          busy={busy}
          error={error}
          onCancel={onClose}
          onBlock={() => void block()}
        />
      )}
    </>
  );
}

function message(e: unknown): string {
  return e instanceof ApiError ? e.message : String(e);
}
