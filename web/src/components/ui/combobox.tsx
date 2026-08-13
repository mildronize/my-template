/**
 * A select you can type into.
 *
 * Built from the Popover and Input already in this repo rather than adding
 * `cmdk`. The list is a filtered array and the keyboard handling is twenty
 * lines — a dependency would be more surface than the thing it replaces, and
 * this file is easier to read than the decision to add one.
 *
 * THE SEARCH FIELD IS ALWAYS THERE. It briefly was not: it hid itself below
 * eight options, on my reasoning that a search box over four fixed statuses
 * is a keystroke tax. That was a defensible design and it was not what was
 * asked for — with 7 projects, 4 statuses and 3 groups, every select except
 * assignee fell under the threshold, so "all select boxes can search" shipped
 * as "one of them can". มายด์ said it twice, once in the task and once after
 * using it. The second time is the answer.
 *
 * Kept as a named constant rather than deleted, so the option is visible and
 * so re-introducing it has to be a deliberate edit rather than a default
 * someone drifts back into.
 */

import { useMemo, useRef, useState, type ReactNode } from "react";
import { Popover, PopoverContent, PopoverTrigger } from "~/components/ui/popover";
import { Input } from "~/components/ui/input";
import { Button } from "~/components/ui/button";

/** Show the search field once there are at least this many options. 1 = always. */
export const SEARCH_THRESHOLD = 1;

export interface ComboboxOption {
  value: string;
  label: string;
}

export function Combobox({
  value,
  options,
  onChange,
  placeholder = "Select…",
  className,
  triggerClassName,
  triggerLabel,
}: {
  value: string | undefined;
  options: ComboboxOption[];
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  triggerClassName?: string;
  /**
   * Overrides the trigger's text. Exists so the status select can keep
   * rendering its badge instead of degrading to a plain word — converting
   * every select should not cost the ones that already said something extra.
   */
  triggerLabel?: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);

  const showSearch = options.length >= SEARCH_THRESHOLD;

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    // Substring rather than fuzzy, deliberately. Fuzzy matching on a list of
    // crew handles produces confident wrong answers — the failure mode is a
    // match that looks intentional.
    return options.filter((o) => o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q));
  }, [options, query]);

  const selected = options.find((o) => o.value === value);

  function commit(v: string) {
    onChange(v);
    setOpen(false);
    setQuery("");
    setActive(0);
  }

  return (
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setQuery("");
          setActive(0);
        }
      }}
    >
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className={triggerClassName ?? "h-9 justify-between font-normal"}
        >
          <span className={selected ? "" : "text-[var(--sea-ink-soft)]"}>
            {triggerLabel ?? selected?.label ?? placeholder}
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className={className ?? "w-56 p-1"} align="start">
        {showSearch && (
          <Input
            autoFocus
            value={query}
            placeholder="Search…"
            className="mb-1 h-8"
            onChange={(e) => {
              setQuery(e.target.value);
              setActive(0);
            }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") {
                e.preventDefault();
                setActive((i) => Math.min(i + 1, filtered.length - 1));
              } else if (e.key === "ArrowUp") {
                e.preventDefault();
                setActive((i) => Math.max(i - 1, 0));
              } else if (e.key === "Enter") {
                e.preventDefault();
                const pick = filtered[active];
                if (pick) commit(pick.value);
              }
            }}
          />
        )}
        <div ref={listRef} className="max-h-64 overflow-y-auto">
          {filtered.length === 0 ? (
            // Says what it searched, not just that it found nothing — an
            // empty list with no explanation reads as "there is nothing",
            // which is a different and false statement.
            <p className="px-2 py-3 text-xs text-[var(--sea-ink-soft)]">
              Nothing matches “{query}” · {options.length} available
            </p>
          ) : (
            filtered.map((o, i) => (
              <button
                key={o.value}
                type="button"
                onMouseEnter={() => setActive(i)}
                onClick={() => commit(o.value)}
                className={`block w-full rounded px-2 py-1.5 text-left text-sm ${
                  i === active ? "bg-[var(--chip-bg)]" : ""
                } ${o.value === value ? "font-medium" : ""}`}
              >
                {o.label}
              </button>
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
