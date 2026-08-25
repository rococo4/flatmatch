/* Шторка фильтров подбора — «Каким должен быть сосед» (§6).

   Фильтры не правило видимости (§4), а необязательное сужение колоды по
   анкете: пустой ряд значит «неважно», поэтому варианта «любой» ни в одном
   ряду нет — его роль играет пустота.

   Живут на клиенте: сервер их не помнит, а человек между заходами — да,
   поэтому набор лежит в localStorage. Счётчик «Показать · N» живой: черновик
   уходит тем же запросом колоды и показывает её `total`, а не длину какого-то
   отфильтрованного списка на клиенте — считает всегда сервер. */
import { useEffect, useRef, useState } from 'react';
import type { CSSProperties } from 'react';

import {
  fetchDeck,
  NO_DECK_FILTERS,
  type DeckFilters,
  type DeckOccupation,
  type DeckPets,
  type DeckSchedule,
  type DeckSmoking,
  type DeckWho,
} from '../api/client';
import {
  deckOccupationOptions,
  deckPetsOptions,
  deckScheduleOptions,
  deckSmokingOptions,
  deckWhoOptions,
} from '../api/format';
import { S } from '../ui/primitives';

const STORAGE_KEY = 'deck_filters_v1';

/** Пауза перед запросом счётчика: возраст набирают по цифре. */
const COUNT_DELAY_MS = 250;

const F = {
  scrim: {
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
    maxWidth: 520,
    maxHeight: '86vh',
    display: 'flex',
    flexDirection: 'column',
    borderRadius: 16,
    background: 'var(--card-bg)',
    border: '1px solid var(--sep)',
    overflow: 'hidden',
  } as CSSProperties,
  head: {
    display: 'flex',
    flexDirection: 'column',
    gap: 4,
    padding: '14px 14px 10px',
  } as CSSProperties,
  list: {
    flex: '1 1 auto',
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
    padding: '4px 14px 14px',
    overflowY: 'auto',
  } as CSSProperties,
  foot: {
    flex: '0 0 auto',
    display: 'flex',
    gap: 8,
    padding: '10px 14px 14px',
    borderTop: '1px solid var(--sep)',
  } as CSSProperties,
  field: { display: 'flex', flexDirection: 'column', gap: 8 } as CSSProperties,
  label: { fontSize: 15, fontWeight: 590 } as CSSProperties,
  chipRow: { display: 'flex', flexWrap: 'wrap', gap: 7 } as CSSProperties,
  input: {
    width: '100%',
    padding: '11px 12px',
    borderRadius: 10,
    border: 'none',
    background: 'var(--app-bg)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-text-color)',
    boxSizing: 'border-box',
  } as CSSProperties,
  reset: {
    flex: '0 0 auto',
    padding: '13px 16px',
    border: 'none',
    borderRadius: 11,
    background: 'var(--chip-bg)',
    font: 'inherit',
    fontSize: 15,
    fontWeight: 590,
    color: 'var(--tg-theme-accent-text-color)',
    cursor: 'pointer',
  } as CSSProperties,
};

/** Черновик шторки: возраст — сырые строки инпутов, «пусто» это не ноль. */
type Draft = Omit<DeckFilters, 'age_min' | 'age_max'> & { ageMin: string; ageMax: string };

export function FiltersSheet({
  value,
  onApply,
  onClose,
}: {
  value: DeckFilters;
  onApply: (next: DeckFilters) => void;
  onClose: () => void;
}) {
  const [draft, setDraft] = useState<Draft>(() => toDraft(value));
  const [count, setCount] = useState<number | null>(null);

  const filters = fromDraft(draft);
  // Свежий черновик в ref: эффект пересобирается по его СОДЕРЖИМОМУ, а не по
  // ссылке — иначе счётчик перезапрашивался бы на каждый рендер шторки.
  const filtersRef = useRef(filters);
  filtersRef.current = filters;

  // Счётчик считает сервер тем же запросом колоды. Ответ на устаревший
  // черновик выбрасываем: пока летел запрос, чипы могли перещёлкнуть.
  const query = JSON.stringify(filters);
  useEffect(() => {
    let cancelled = false;
    const timer = window.setTimeout(() => {
      void fetchDeck(filtersRef.current)
        .then((deck) => {
          if (!cancelled) setCount(deck.total);
        })
        .catch(() => {
          // Счётчик не приехал — кнопка останется без цифры, а не соврёт.
          if (!cancelled) setCount(null);
        });
    }, COUNT_DELAY_MS);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [query]);

  const digits = (raw: string) => raw.replace(/\D/g, '').slice(0, 2);

  return (
    <div style={F.scrim} onClick={onClose}>
      <div style={F.sheet} onClick={(e) => e.stopPropagation()}>
        <div style={F.head}>
          <span style={{ fontSize: 19, fontWeight: 640, letterSpacing: '-0.4px' }}>
            Каким должен быть сосед
          </span>
          <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>
            пусто = неважно
          </span>
        </div>

        <div style={F.list} className="no-scrollbar">
          <Row<DeckWho>
            label="Кого показывать"
            options={deckWhoOptions()}
            picked={draft.who}
            onToggle={(who) => setDraft({ ...draft, who })}
          />

          <div style={F.field}>
            <span style={F.label}>Возраст</span>
            <div style={{ display: 'flex', gap: 8 }}>
              <div style={{ flex: '1 1 0', display: 'flex', flexDirection: 'column', gap: 5, minWidth: 0 }}>
                <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>от</span>
                <input
                  style={F.input}
                  inputMode="numeric"
                  placeholder="неважно"
                  value={draft.ageMin}
                  onChange={(e) => setDraft({ ...draft, ageMin: digits(e.target.value) })}
                />
              </div>
              <div style={{ flex: '1 1 0', display: 'flex', flexDirection: 'column', gap: 5, minWidth: 0 }}>
                <span style={{ fontSize: 13, color: 'var(--tg-theme-subtitle-text-color)' }}>до</span>
                <input
                  style={F.input}
                  inputMode="numeric"
                  placeholder="неважно"
                  value={draft.ageMax}
                  onChange={(e) => setDraft({ ...draft, ageMax: digits(e.target.value) })}
                />
              </div>
            </div>
          </div>

          <Row<DeckPets>
            label="Животные в квартире"
            options={deckPetsOptions()}
            picked={draft.pets}
            onToggle={(pets) => setDraft({ ...draft, pets })}
          />
          <Row<DeckSmoking>
            label="Курение"
            options={deckSmokingOptions()}
            picked={draft.smoking}
            onToggle={(smoking) => setDraft({ ...draft, smoking })}
          />
          <Row<DeckSchedule>
            label="Режим дня"
            options={deckScheduleOptions()}
            picked={draft.schedule}
            onToggle={(schedule) => setDraft({ ...draft, schedule })}
          />
          <Row<DeckOccupation>
            label="Чем занят"
            options={deckOccupationOptions()}
            picked={draft.occupation}
            onToggle={(occupation) => setDraft({ ...draft, occupation })}
          />
        </div>

        <div style={F.foot}>
          <button type="button" style={F.reset} onClick={() => setDraft(toDraft(NO_DECK_FILTERS))}>
            Сбросить
          </button>
          <button
            type="button"
            style={{ ...S.filledButton, flex: '1 1 auto', width: 'auto' }}
            onClick={() => onApply(filters)}
          >
            Показать{count !== null ? ` · ${count}` : ''}
          </button>
        </div>
      </div>
    </div>
  );
}

/** Ряд мультивыбора: повторный тап снимает, пустой ряд значит «неважно». */
function Row<T extends string>({
  label,
  options,
  picked,
  onToggle,
}: {
  label: string;
  options: { value: T; label: string }[];
  picked: T[];
  onToggle: (next: T[]) => void;
}) {
  return (
    <div style={F.field}>
      <span style={F.label}>{label}</span>
      <div style={F.chipRow}>
        {options.map((o) => {
          const active = picked.includes(o.value);
          return (
            <span
              key={o.value}
              onClick={() => onToggle(toggle(picked, o.value, options))}
              style={{
                padding: '10px 14px',
                borderRadius: 10,
                background: active
                  ? 'color-mix(in srgb, var(--tg-theme-button-color) 12%, transparent)'
                  : 'var(--app-bg)',
                border: active
                  ? '1.5px solid var(--tg-theme-button-color)'
                  : '1.5px solid transparent',
                fontSize: 15,
                fontWeight: active ? 590 : 400,
                color: active ? 'var(--tg-theme-accent-text-color)' : 'var(--tg-theme-text-color)',
                cursor: 'pointer',
              }}
            >
              {o.label}
            </span>
          );
        })}
      </div>
    </div>
  );
}

/**
 * Фильтры прошлого захода.
 *
 * Всё чтение — в try/catch и с проверкой значений: в хранилище лежит то, что
 * писала прошлая версия приложения, и неизвестный код сервер отбил бы 400 на
 * первом же запросе колоды.
 */
export function loadDeckFilters(): DeckFilters {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return NO_DECK_FILTERS;
    const saved = JSON.parse(raw) as Record<string, unknown>;
    return {
      who: known(saved.who, deckWhoOptions()),
      age_min: age(saved.age_min),
      age_max: age(saved.age_max),
      pets: known(saved.pets, deckPetsOptions()),
      smoking: known(saved.smoking, deckSmokingOptions()),
      schedule: known(saved.schedule, deckScheduleOptions()),
      occupation: known(saved.occupation, deckOccupationOptions()),
    };
  } catch {
    return NO_DECK_FILTERS;
  }
}

export function saveDeckFilters(f: DeckFilters): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(f));
  } catch {
    // Приватный режим или переполненное хранилище: фильтры просто не переживут
    // перезапуск — ронять из-за этого экран нечего.
  }
}

/** Порядок держим по ряду чипов, а не по порядку тапов: набор едет в запрос. */
function toggle<T extends string>(picked: T[], value: T, options: { value: T }[]): T[] {
  const next = picked.includes(value) ? picked.filter((v) => v !== value) : [...picked, value];
  return options.map((o) => o.value).filter((v) => next.includes(v));
}

function known<T extends string>(raw: unknown, options: { value: T }[]): T[] {
  if (!Array.isArray(raw)) return [];
  return options.map((o) => o.value).filter((v) => raw.includes(v));
}

function age(raw: unknown): number | null {
  return typeof raw === 'number' && Number.isFinite(raw) && raw > 0 ? Math.trunc(raw) : null;
}

function toDraft(f: DeckFilters): Draft {
  const { age_min, age_max, ...rest } = f;
  return {
    ...rest,
    ageMin: age_min != null ? String(age_min) : '',
    ageMax: age_max != null ? String(age_max) : '',
  };
}

function fromDraft(d: Draft): DeckFilters {
  const { ageMin, ageMax, ...rest } = d;
  return {
    ...rest,
    age_min: ageMin ? Number(ageMin) : null,
    age_max: ageMax ? Number(ageMax) : null,
  };
}
