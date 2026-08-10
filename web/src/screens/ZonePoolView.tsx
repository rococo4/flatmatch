/* Контейнер зонного пула: загрузка, разбор ответа и перевод его в пропсы экрана.

   ЕДИНСТВЕННОЕ место в коде, которое зовёт fetchZonePool. Так и должно
   остаться: этот GET мутирует pool_seen_at, то есть гасит бейдж «M новых».
   Любой префетч, refetch-on-focus или второй прогон эффекта в StrictMode
   незаметно съел бы бейдж — отсюда ref-guard по zoneId и отсутствие
   зависимостей, кроме самой зоны. */
import { useEffect, useRef, useState } from 'react';
import type { CSSProperties } from 'react';

import { ApiError, fetchZonePool, type PoolPersonDTO, type PoolResponse } from '../api/client';
import { chipLabel, habitsLine, sectionTitle, VERDICT_UI } from '../api/format';
import { ZonePoolScreen, type PoolPerson, type PoolSection } from './ZonePoolScreen';

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

export function ZonePoolView({
  zoneId,
  pointsCount,
  onLoaded,
  onBack,
  onOpenPerson,
  onOpenPoints,
}: {
  zoneId: string;
  /** Точек под этот поиск. Ноль прячет ссылку целиком (§12 шаг 5 ещё не пришёл). */
  pointsCount?: number;
  /**
   * Пул загрузился. Хозяин списка поисков перечитывает его после этого:
   * сервер уже сдвинул pool_seen_at, и клиентский бейдж «M новых» иначе врёт.
   */
  onLoaded?: () => void;
  onBack?: () => void;
  onOpenPerson?: (id: string) => void;
  onOpenPoints?: () => void;
}) {
  const [pool, setPool] = useState<PoolResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Свежий колбэк в ref: иначе он попал бы в зависимости эффекта, и каждый
  // ререндер хозяина стоил бы ещё одного гашения бейджа.
  const onLoadedRef = useRef(onLoaded);
  useEffect(() => {
    onLoadedRef.current = onLoaded;
  }, [onLoaded]);

  // Отметка ставится ДО запроса и не снимается в cleanup: второй прогон
  // эффекта в StrictMode должен стать no-op, а не вторым запросом.
  const loaded = useRef<string | null>(null);
  useEffect(() => {
    if (loaded.current === zoneId) return;
    loaded.current = zoneId;
    setPool(null);
    setError(null);
    fetchZonePool(zoneId)
      .then((data) => {
        setPool(data);
        onLoadedRef.current?.();
      })
      .catch((e: unknown) => {
        setError(e instanceof ApiError ? e.message : String(e));
      });
  }, [zoneId]);

  if (error || !pool) {
    return (
      <div style={V.screen}>
        <div style={V.header}>
          <span style={V.back} onClick={onBack}>
            ‹
          </span>
          <span style={{ fontSize: 17, fontWeight: 590, letterSpacing: '-0.2px' }}>Пул</span>
        </div>
        <div style={V.centered}>{error ?? 'Секунду…'}</div>
      </div>
    );
  }

  return (
    <ZonePoolScreen
      zone={pool.zone.name}
      zoneGen={pool.zone.name_gen}
      // Плашка — по подписке именно на эту зону, секции — по ranked: два
      // разных факта, и совпадают они не всегда.
      saved={Boolean(pool.my_search_id)}
      sections={toSections(pool)}
      pointsCount={pointsCount}
      onBack={onBack}
      onOpenPerson={onOpenPerson}
      onOpenPoints={onOpenPoints}
    />
  );
}

/**
 * Три секции ранжирования или одна общая.
 *
 * Пустые секции не рисуются: заголовок «ПЕРЕСЕКАЮТСЯ ЧАСТИЧНО · 0» — это шум,
 * а не информация. «Не совпали» приглушены и свёрнуты после первого (§6).
 */
function toSections(pool: PoolResponse): PoolSection[] {
  if (!pool.ranked) {
    if (pool.people.length === 0) return [];
    return [
      {
        id: 'all',
        title: `ЛЮДИ ИЩУТ ЗДЕСЬ · ${pool.people.length}`,
        people: pool.people.map(toPerson),
      },
    ];
  }
  return pool.sections
    .filter((s) => s.people.length > 0)
    .map((s) => ({
      id: s.id,
      title: sectionTitle(s.id, s.people.length),
      people: s.people.map(toPerson),
      dimmed: s.id === 'none',
      collapsedAfter: s.id === 'partial' ? 3 : s.id === 'none' ? 1 : undefined,
    }));
}

/**
 * Человек в пуле.
 *
 * Вердикты собираются здесь и только здесь: цвет берётся из VERDICT_UI, текст —
 * из параметров самого человека, поэтому серый чип честно показывает либо его
 * значение, либо «не указана». `verifiedBadge` сервер пока не шлёт — выдумывать
 * бейдж нельзя.
 */
function toPerson(p: PoolPersonDTO): PoolPerson {
  return {
    id: p.user_id,
    name: p.name,
    age: p.age,
    habits: habitsLine(p),
    searchZone: `ищет ${p.zone.name_in}`,
    matches: p.shared_answers,
    photoUrl: p.photo_url || undefined,
    verdicts: (p.chips ?? []).map((c) => ({
      verdict: VERDICT_UI[c.verdict],
      label: chipLabel(c, p.params),
    })),
  };
}
