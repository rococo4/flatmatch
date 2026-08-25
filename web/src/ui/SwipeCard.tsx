/* Жест карточки подбора: тянем пальцем, отпускаем — карточка либо улетает,
   либо возвращается на место (§6).

   Библиотеки здесь нет и не будет: весь жест — это pointer events с захватом
   указателя, а полёт и возврат — одна CSS-транзишн. Захват (setPointerCapture)
   обязателен: без него палец, ушедший за край карточки, теряет события, и
   карточка залипает на середине броска.

   Полётом владеет НЕ карточка, а экран: он ставит `fly` и решает, что делать
   дальше (лайк может не пройти — тогда карточка возвращается сама, стоит
   только снять `fly`). Здесь только рисование и распознавание жеста. */
import { useRef, useState } from 'react';
import type { CSSProperties, PointerEvent as ReactPointerEvent, ReactNode } from 'react';

export type SwipeDir = 'left' | 'right';

/** Порог броска — доля ширины карточки. Меньше — случайные лайки. */
const THROW_RATIO = 0.35;

/** Наклон на пороге, градусы. */
const MAX_TILT = 8;

/** Смещение улетевшей карточки — заведомо за край экрана. */
const FLIGHT_RATIO = 1.6;

/** Дрожание пальца, которое всё ещё считается тапом, а не броском. */
const TAP_SLOP = 6;

export function SwipeCard({
  fly,
  lockRight,
  onFling,
  onTap,
  children,
}: {
  /** Бросок, начатый экраном (кнопкой или ответом на жест). */
  fly: SwipeDir | null;
  /** Лайк недоступен (анкета на проверке): бросок вправо возвращается назад. */
  lockRight?: boolean;
  /** Жест дошёл до порога — экран решает, что с этим делать. */
  onFling?: (dir: SwipeDir) => void;
  /** Тап без броска: открыть карточку целиком. */
  onTap?: () => void;
  children: ReactNode;
}) {
  const [dx, setDx] = useState(0);
  const [dragging, setDragging] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  const start = useRef<{ x: number; y: number } | null>(null);
  const moved = useRef(0);

  const width = () => box.current?.offsetWidth ?? 320;

  const down = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (fly) return;
    start.current = { x: e.clientX, y: e.clientY };
    moved.current = 0;
    setDragging(true);
    e.currentTarget.setPointerCapture(e.pointerId);
  };

  const move = (e: ReactPointerEvent<HTMLDivElement>) => {
    const from = start.current;
    if (!from) return;
    const shift = e.clientX - from.x;
    moved.current = Math.max(moved.current, Math.abs(shift), Math.abs(e.clientY - from.y));
    setDx(shift);
  };

  /** Общий хвост жеста: карточка возвращается на место, захват отпускается. */
  const release = (e: ReactPointerEvent<HTMLDivElement>) => {
    start.current = null;
    setDragging(false);
    setDx(0);
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId);
    }
  };

  // Жест отменила система (звонок, шторка, второй палец) — это НЕ бросок,
  // сколько бы карточку ни утащили: отменённый жест обязан вернуться.
  const cancel = (e: ReactPointerEvent<HTMLDivElement>) => {
    if (!start.current) return;
    release(e);
  };

  const up = (e: ReactPointerEvent<HTMLDivElement>) => {
    const from = start.current;
    if (!from) return;
    release(e);

    const shift = e.clientX - from.x;
    const threshold = width() * THROW_RATIO;
    if (shift <= -threshold) {
      onFling?.('left');
      return;
    }
    if (shift >= threshold) {
      // Заперт вправо — карточка просто возвращается: отказ должен быть
      // виден плашкой сверху, а не «сработавшим» лайком.
      if (!lockRight) onFling?.('right');
      return;
    }
    if (moved.current < TAP_SLOP) onTap?.();
  };

  const shift = fly ? (fly === 'left' ? -1 : 1) * width() * FLIGHT_RATIO : dx;
  const tilt = fly
    ? (fly === 'left' ? -1 : 1) * MAX_TILT
    : clamp(dx / (width() * THROW_RATIO), -1, 1) * MAX_TILT;

  const style: CSSProperties = {
    touchAction: 'pan-y',
    transform: `translateX(${shift}px) rotate(${tilt}deg)`,
    opacity: fly ? 0 : 1,
    // Пока тянем — никаких транзишнов: карточка обязана идти за пальцем.
    // Отпустили — возврат с лёгким перелётом, полёт — ровным разгоном.
    transition: dragging
      ? 'none'
      : fly
        ? 'transform 220ms cubic-bezier(.2,.6,.3,1), opacity 220ms linear'
        : 'transform 260ms cubic-bezier(.2,1.2,.4,1)',
    willChange: 'transform',
    cursor: 'grab',
  };

  return (
    <div
      ref={box}
      style={style}
      onPointerDown={down}
      onPointerMove={move}
      onPointerUp={up}
      onPointerCancel={cancel}
    >
      {children}
    </div>
  );
}

function clamp(v: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, v));
}
