/* Сетка фото анкеты — шаг фото онбординга (§4) и экран «Анкета» (§6).
   До пяти кадров, первый — главный: его видят обложкой карточки в подборе.

   Сеть живёт внутри компонента, а не в экранах: сетка стоит в двух местах, все
   три операции отвечают профилем целиком, и разложенные по экранам они были бы
   одним кодом, написанным дважды.

   Порядок меняется зажатием и перетаскиванием — pointer events с захватом
   указателя, без библиотек (те же правила, что у жеста колоды). Захват
   обязателен: палец, ушедший за край плитки, иначе теряет события. touch-action
   у плиток снят целиком, потому что тащат и по вертикали — маленькое фото
   становится главным, — а pan-y отдал бы такой жест прокрутке страницы. */
import { useRef, useState } from 'react';
import type { CSSProperties, PointerEvent as ReactPointerEvent } from 'react';

import { addPhoto, ApiError, removePhoto, reorderPhotos } from '../api/client';
import type { Profile } from '../api/client';
import { LimitChip } from './primitives';

/** Потолок набора. Тот же лимит стоит на сервере — он отвечает `photo_limit`. */
const MAX_PHOTOS = 5;

/** Одни слова на обе стороны лимита: свою отсечку и отказ сервера. */
const LIMIT_MESSAGE = `Больше ${MAX_PHOTOS} фото не выйдет`;

/** Сколько держать плитку, прежде чем она поедет за пальцем. */
const HOLD_MS = 200;

/** Дрожание пальца, которое ещё не жест. */
const DRAG_SLOP = 8;

/** Куда ушёл запрос: у отказа должно быть одно место на компонент. */
type Busy = 'add' | 'remove' | 'order';

type Drag = { from: number; over: number; dx: number; dy: number };

const G = {
  head: { display: 'flex', alignItems: 'center', gap: 8 } as CSSProperties,
  title: { fontSize: 15, fontWeight: 590 } as CSSProperties,
  grid: { display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 8 } as CSSProperties,
  cell: {
    position: 'relative',
    aspectRatio: '1',
    borderRadius: 12,
    overflow: 'hidden',
    background: 'var(--chip-bg)',
    touchAction: 'none',
    userSelect: 'none',
    cursor: 'grab',
  } as CSSProperties,
  /** Главное фото — квадрат два на два: он же обложка карточки. */
  big: { gridColumn: 'span 2', gridRow: 'span 2' } as CSSProperties,
  image: {
    width: '100%',
    height: '100%',
    objectFit: 'cover',
    display: 'block',
    pointerEvents: 'none',
  } as CSSProperties,
  mainBadge: {
    position: 'absolute',
    left: 6,
    bottom: 6,
    padding: '3px 7px',
    borderRadius: 6,
    background: 'rgba(0,0,0,0.55)',
    color: '#fff',
    fontSize: 11,
    fontWeight: 590,
  } as CSSProperties,
  remove: {
    position: 'absolute',
    top: 5,
    right: 5,
    width: 22,
    height: 22,
    padding: 0,
    border: 'none',
    borderRadius: '50%',
    background: 'rgba(0,0,0,0.55)',
    font: 'inherit',
    fontSize: 13,
    lineHeight: 1,
    color: '#fff',
    cursor: 'pointer',
  } as CSSProperties,
  plus: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 4,
    background: 'transparent',
    border: '1.5px dashed color-mix(in srgb, var(--tg-theme-button-color) 55%, transparent)',
    color: 'var(--tg-theme-accent-text-color)',
    touchAction: 'auto',
  } as CSSProperties,
  caption: {
    fontSize: 12,
    lineHeight: 1.4,
    color: 'var(--tg-theme-hint-color)',
    textWrap: 'pretty',
  } as CSSProperties,
  error: { fontSize: 13, color: 'var(--tg-theme-destructive-text-color)' } as CSSProperties,
};

export function PhotoGrid({
  profile,
  onProfile,
  onAdded,
  reviewLabel = 'уйдут на проверку',
}: {
  profile: Profile;
  /** Ответ сервера — истина: набор и его статус приезжают профилем целиком. */
  onProfile: (next: Profile) => void;
  /**
   * Фото добавлено — единственное изменение набора, которое зовёт модератора:
   * удаление и перестановка одобренного набора статус не трогают (§5).
   */
  onAdded?: () => void;
  /** «на проверке» вместо «уйдут на проверку» там, где карточка уже в очереди. */
  reviewLabel?: string;
}) {
  const [busy, setBusy] = useState<Busy | null>(null);
  const [error, setError] = useState<string | null>(null);
  /** Порядок до ответа сервера: перетаскивание обязано отдаваться сразу. */
  const [moved, setMoved] = useState<string[] | null>(null);
  const [drag, setDrag] = useState<Drag | null>(null);

  const input = useRef<HTMLInputElement>(null);
  const cells = useRef<(HTMLDivElement | null)[]>([]);
  const press = useRef<{ index: number; x: number; y: number; timer: number } | null>(null);
  /**
   * Тот же жест в ref.
   *
   * Состояние здесь только рисует: обработчик отпускания читает его из своего
   * рендера, а между ним и последним движением рендер мог не случиться —
   * тогда бросок терялся бы, а плитка оставалась висеть на полпути.
   */
  const gesture = useRef<Drag | null>(null);

  const urls = moved ?? profile.photo_urls;
  const room = MAX_PHOTOS - urls.length;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
      <div style={G.head}>
        <span style={G.title}>
          Фото · {urls.length} из {MAX_PHOTOS}
        </span>
        {urls.length > 0 && profile.photo_status === 'pending' && (
          <LimitChip small>{reviewLabel}</LimitChip>
        )}
      </div>

      <div style={G.grid}>
        {urls.map((url, i) => (
          <div
            key={url}
            ref={(el) => {
              cells.current[i] = el;
            }}
            style={cellStyle(i, drag)}
            onPointerDown={(e) => down(e, i)}
            onPointerMove={move}
            onPointerUp={up}
            onPointerCancel={cancel}
          >
            <img src={url} alt="" style={G.image} />
            {i === 0 && <span style={G.mainBadge}>главное</span>}
            <button
              type="button"
              aria-label="Убрать фото"
              disabled={busy !== null}
              // Крестик не начинает перетаскивание: иначе тап по нему на
              // касании читался бы как зажатая плитка.
              onPointerDown={(e) => e.stopPropagation()}
              onClick={() => void remove(i)}
              style={G.remove}
            >
              ×
            </button>
          </div>
        ))}

        {room > 0 && (
          <label
            style={{
              ...G.cell,
              ...(urls.length === 0 ? G.big : null),
              ...G.plus,
              cursor: busy === 'add' ? 'progress' : 'pointer',
            }}
          >
            <input
              ref={input}
              type="file"
              accept="image/jpeg,image/png,image/webp"
              multiple
              disabled={busy !== null}
              style={{ display: 'none' }}
              onChange={(e) => void add(e.target.files)}
            />
            <span style={{ fontSize: 26, lineHeight: 1 }}>＋</span>
            {(busy === 'add' || urls.length === 0) && (
              <span style={{ fontSize: 13, fontWeight: 510 }}>
                {busy === 'add' ? 'Загружаю…' : 'Загрузить фото'}
              </span>
            )}
          </label>
        )}
      </div>

      <span style={G.caption}>
        Первое — главное, его видят в подборе. Зажми и перетащи, чтобы поменять порядок. Новые фото
        уйдут на проверку
      </span>
      {error && <span style={G.error}>{error}</span>}
    </div>
  );

  function down(e: ReactPointerEvent<HTMLDivElement>, index: number) {
    if (busy !== null || urls.length < 2) return;
    const timer = window.setTimeout(() => {
      if (!gesture.current) apply({ from: index, over: index, dx: 0, dy: 0 });
    }, HOLD_MS);
    press.current = { index, x: e.clientX, y: e.clientY, timer };
    e.currentTarget.setPointerCapture(e.pointerId);
  }

  function move(e: ReactPointerEvent<HTMLDivElement>) {
    const start = press.current;
    if (!start) return;

    const dx = e.clientX - start.x;
    const dy = e.clientY - start.y;
    const held = gesture.current;
    // Мышью тащат сразу, без удержания: курсор ничего не прокручивает, и
    // ждать от него полсекунды незачем.
    if (!held && Math.hypot(dx, dy) < DRAG_SLOP) return;

    window.clearTimeout(start.timer);
    const from = held?.from ?? start.index;
    apply({ from, over: cellAt(e.clientX, e.clientY) ?? held?.over ?? from, dx, dy });
  }

  function up(e: ReactPointerEvent<HTMLDivElement>) {
    if (!press.current) return;
    const held = gesture.current;
    release(e);
    if (held && held.over !== held.from) void reorder(held.from, held.over);
  }

  // Жест отменила система (звонок, шторка, второй палец) — это не перестановка,
  // сколько бы плитку ни утащили.
  function cancel(e: ReactPointerEvent<HTMLDivElement>) {
    if (!press.current) return;
    release(e);
  }

  function apply(next: Drag | null) {
    gesture.current = next;
    setDrag(next);
  }

  function release(e: ReactPointerEvent<HTMLDivElement>) {
    if (press.current) window.clearTimeout(press.current.timer);
    press.current = null;
    apply(null);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId);
    }
  }

  /** Под каким кадром сейчас палец. Пусто — жест ушёл за пределы сетки. */
  function cellAt(x: number, y: number): number | null {
    for (let i = 0; i < urls.length; i++) {
      const box = cells.current[i]?.getBoundingClientRect();
      if (box && x >= box.left && x <= box.right && y >= box.top && y <= box.bottom) return i;
    }
    return null;
  }

  async function add(files: FileList | null) {
    if (!files?.length || busy !== null) return;
    setBusy('add');
    setError(null);
    try {
      // По одному файлу за запрос: сервер добавляет в конец, и порядок выбора
      // становится порядком набора.
      for (const file of Array.from(files).slice(0, room)) {
        onProfile(await addPhoto(file));
        onAdded?.();
      }
      if (files.length > room) setError(LIMIT_MESSAGE);
    } catch (e) {
      setError(messageOf(e));
    } finally {
      setBusy(null);
      // Сбрасываем значение: иначе повторный выбор того же файла после
      // отклонённой загрузки не даст события вовсе.
      if (input.current) input.current.value = '';
    }
  }

  async function remove(index: number) {
    // Последнее фото отправленной анкеты сервер снять не даст (`last_photo`):
    // фото обязательно (§4). Предсказывать его правило здесь нечем — «анкету
    // хоть раз отправляли» знает он, а не набор, — поэтому кнопка живая у
    // каждого кадра, а отказ становится строкой под сеткой.
    await call('remove', () => removePhoto(index));
  }

  async function reorder(from: number, to: number) {
    // Порядок — старые индексы в новом порядке: ровно это ждёт сервер.
    const order = urls.map((_, i) => i);
    const [taken] = order.splice(from, 1);
    order.splice(to, 0, taken);
    setMoved(order.map((i) => urls[i]));
    await call('order', () => reorderPhotos(order));
  }

  async function call(at: Busy, request: () => Promise<Profile>) {
    setBusy(at);
    setError(null);
    try {
      onProfile(await request());
    } catch (e) {
      setError(messageOf(e));
    } finally {
      setBusy(null);
      // Показанный наперёд порядок уступает ответу сервера — и своему же,
      // когда запрос не прошёл.
      setMoved(null);
    }
  }
}

function cellStyle(i: number, drag: Drag | null): CSSProperties {
  const dragged = drag?.from === i;
  const target = drag != null && drag.over === i && drag.from !== i;
  return {
    ...G.cell,
    ...(i === 0 ? G.big : null),
    transform: dragged ? `translate(${drag.dx}px, ${drag.dy}px) scale(1.04)` : 'none',
    // Пока тащим — никаких транзишнов: плитка обязана идти за пальцем.
    transition: dragged ? 'none' : 'transform 160ms ease',
    zIndex: dragged ? 2 : undefined,
    opacity: dragged ? 0.92 : 1,
    outline: target ? '2px solid var(--tg-theme-button-color)' : 'none',
    outlineOffset: target ? 2 : 0,
  };
}

/**
 * Отказ по-русски.
 *
 * Двум своим кодам сетка даёт свои слова: сообщения сервера на эти два —
 * английские, а читает их человек. Остальное идёт как есть: незнакомый отказ
 * честнее показать словами сервера, чем общей отговоркой.
 */
function messageOf(e: unknown): string {
  if (!(e instanceof ApiError)) return 'Не получилось сохранить — проверь связь';
  switch (e.code) {
    case 'photo_limit':
      return LIMIT_MESSAGE;
    case 'last_photo':
      return 'Фото обязательно — сначала загрузи новое';
    default:
      return e.message;
  }
}
