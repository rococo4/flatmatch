/* Фото с листалкой — карточка подбора, анкета человека, комната «к себе».

   Листалка — два узких края поверх фото: они гасят pointerdown, поэтому
   тянуть карточку с середины фото жест свайпа не мешает. Сегменты сверху
   показывают, сколько кадров и какой открыт. */
import { useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';

import { PhotoPlaceholder } from './primitives';

/** Доля ширины фото, отданная под тап-листалку с каждого края. */
const PAGER_ZONE = '26%';

const P = {
  frame: {
    position: 'relative',
    flex: '0 0 auto',
    width: '100%',
    aspectRatio: '4/5',
    // На низком экране фото ужимается, а не выдавливает всё остальное:
    // object-fit кадрирует, и лицо остаётся в кадре.
    maxHeight: '54vh',
    background: 'var(--chip-bg)',
    overflow: 'hidden',
  } as CSSProperties,
  image: { width: '100%', height: '100%', objectFit: 'cover', display: 'block' } as CSSProperties,
  segments: {
    position: 'absolute',
    top: 8,
    left: 10,
    right: 10,
    display: 'flex',
    gap: 4,
  } as CSSProperties,
  badges: {
    position: 'absolute',
    left: 10,
    right: 10,
    top: 18,
    display: 'flex',
    flexWrap: 'wrap',
    gap: 6,
  } as CSSProperties,
};

export function PhotoPager({
  urls,
  emptyLabel,
  ratio,
  maxHeight,
  badges,
  children,
}: {
  /** Первое фото — главное: с него открывается любая карточка (§ решений). */
  urls: string[];
  /** Что написать вместо фото: пустой набор — это «не одобрено» (§5). */
  emptyLabel: string;
  ratio?: string;
  maxHeight?: string;
  /** Бейджи поверх фото — зона пары, стадия комнаты. Рисует их вызывающий. */
  badges?: ReactNode;
  children?: ReactNode;
}) {
  const [at, setAt] = useState(0);
  const shown = Math.min(at, Math.max(urls.length - 1, 0));
  const many = urls.length > 1;
  const step = (delta: number) => setAt((i) => Math.min(urls.length - 1, Math.max(0, i + delta)));

  return (
    <div
      style={{
        ...P.frame,
        aspectRatio: ratio ?? P.frame.aspectRatio,
        maxHeight: maxHeight ?? P.frame.maxHeight,
      }}
    >
      {urls.length > 0 ? (
        <img src={urls[shown]} alt="" style={P.image} />
      ) : (
        <PhotoPlaceholder label={emptyLabel} style={{ width: '100%', height: '100%' }} />
      )}

      {many && (
        <>
          <div style={P.segments}>
            {urls.map((url, i) => (
              <span
                key={`${url}:${i}`}
                style={{
                  flex: '1 1 0',
                  height: 3,
                  borderRadius: 2,
                  background: i === shown ? '#fff' : 'rgba(255,255,255,.4)',
                }}
              />
            ))}
          </div>
          <div
            onPointerDown={(e) => e.stopPropagation()}
            onClick={() => step(-1)}
            style={{ position: 'absolute', left: 0, top: 0, bottom: 0, width: PAGER_ZONE }}
          />
          <div
            onPointerDown={(e) => e.stopPropagation()}
            onClick={() => step(1)}
            style={{ position: 'absolute', right: 0, top: 0, bottom: 0, width: PAGER_ZONE }}
          />
        </>
      )}

      {badges && <div style={P.badges}>{badges}</div>}
      {children}
    </div>
  );
}
