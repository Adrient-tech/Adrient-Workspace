import { useCallback, useEffect, useId, useMemo, useRef, useState, type ReactNode } from "react";
import { Avatar, Box, Chip, ChipDelete, CircularProgress, Input, Typography } from "@mui/joy";
import CheckIcon from "@mui/icons-material/Check";

/** What every picker row needs; concrete pickers add their own fields. */
export interface PickerItem {
  id: string;
  label: string;
  /** Second line (email, address, description). */
  secondary?: string;
  /** Third line (department, count). */
  tertiary?: string;
  avatarUrl?: string;
}

export interface PickerPage<T> {
  items: T[];
  next_cursor?: string;
}

export interface AsyncPickerProps<T extends PickerItem> {
  /** Loads one page. `cursor` is undefined for the first page. */
  fetchPage: (q: string, cursor: string | undefined, signal: AbortSignal) => Promise<PickerPage<T>>;
  value: T[];
  onChange: (next: T[]) => void;
  multiple?: boolean;
  placeholder?: string;
  /** Accessible name of the field. */
  ariaLabel: string;
  disabled?: boolean;
  /** Chips shown before collapsing into "+N". */
  maxChips?: number;
  /** Key for the "recently used" shortcuts kept in this browser. */
  recentKey?: string;
  /** Pinned "Me" shortcut. */
  me?: T | null;
  testId?: string;
  emptyText?: string;
  /** Debounce before a typed query is sent. */
  debounceMs?: number;
  renderIcon?: (item: T) => ReactNode;
  size?: "sm" | "md";
  minWidth?: number | string;
}

export const ROW_HEIGHT = 62;
const VIEWPORT_ROWS = 6;
const OVERSCAN = 3;
const RECENT_MAX = 5;

function readRecent<T>(key?: string): T[] {
  if (!key) return [];
  try {
    return JSON.parse(localStorage.getItem(`adrient.picker.recent.${key}`) ?? "[]") as T[];
  } catch {
    return [];
  }
}
function writeRecent<T extends PickerItem>(key: string | undefined, items: T[]) {
  if (!key || items.length === 0) return;
  try {
    const old = readRecent<T>(key).filter((o) => !items.some((i) => i.id === o.id));
    localStorage.setItem(`adrient.picker.recent.${key}`, JSON.stringify([...items, ...old].slice(0, RECENT_MAX)));
  } catch {
    /* storage unavailable: recents are a convenience only */
  }
}

/** The index range a scroll position needs rendered (exported for tests). */
export function visibleRange(scrollTop: number, viewport: number, count: number) {
  const start = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const end = Math.min(count, Math.ceil((scrollTop + viewport) / ROW_HEIGHT) + OVERSCAN);
  return { start, end };
}

/**
 * Type-ahead picker over a server-paged list. Nothing is ever loaded in full:
 * each page comes from `fetchPage`, rows are windowed, and the next page loads
 * as the list scrolls near its end.
 */
export function AsyncPicker<T extends PickerItem>(props: AsyncPickerProps<T>) {
  const {
    fetchPage,
    value,
    onChange,
    multiple,
    placeholder,
    ariaLabel,
    disabled,
    maxChips = 3,
    recentKey,
    me,
    testId,
    emptyText = "No matches",
    debounceMs = 250,
    renderIcon,
    size = "sm",
    minWidth = 240,
  } = props;
  const uid = useId();
  const listId = `${uid}-list`;
  const rootRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [query, setQuery] = useState("");
  const [rows, setRows] = useState<T[]>([]);
  const [cursor, setCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [active, setActive] = useState(0);
  const [scrollTop, setScrollTop] = useState(0);
  const [expanded, setExpanded] = useState(false);
  const gen = useRef(0);
  const abort = useRef<AbortController | null>(null);

  // Debounce the typed text into the query that is sent to the server.
  useEffect(() => {
    const t = setTimeout(() => setQuery(text.trim()), text ? debounceMs : 0);
    return () => clearTimeout(t);
  }, [text, debounceMs]);

  const load = useCallback(
    async (q: string, cur: string | undefined) => {
      abort.current?.abort();
      const ctl = new AbortController();
      abort.current = ctl;
      const mine = ++gen.current;
      setLoading(true);
      setError("");
      try {
        const page = await fetchPage(q, cur, ctl.signal);
        if (mine !== gen.current) return;
        setRows((old) => (cur ? [...old, ...page.items.filter((i) => !old.some((o) => o.id === i.id))] : page.items));
        setCursor(page.next_cursor || undefined);
      } catch (e) {
        if (mine !== gen.current || (e as Error).name === "AbortError") return;
        setError((e as Error).message || "Could not load");
      } finally {
        if (mine === gen.current) setLoading(false);
      }
    },
    [fetchPage],
  );

  // New query (or opening the list) restarts from the first page.
  useEffect(() => {
    if (!open) return;
    setActive(0);
    setScrollTop(0);
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
    void load(query, undefined);
  }, [open, query, load]);

  useEffect(() => () => abort.current?.abort(), []);

  // Close when focus or a click leaves the widget.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  // Shortcuts (Me, recents) only show for an empty query.
  const shortcuts = useMemo(() => {
    if (query) return [] as T[];
    const out: T[] = [];
    if (me) out.push(me);
    for (const r of readRecent<T>(recentKey)) if (!out.some((o) => o.id === r.id)) out.push(r);
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, me, recentKey, open]);
  const shortcutCount = shortcuts.length;
  const all = useMemo(
    () => [...shortcuts, ...rows.filter((r) => !shortcuts.some((s) => s.id === r.id))],
    [shortcuts, rows],
  );
  const selectedIds = useMemo(() => new Set(value.map((v) => v.id)), [value]);

  const viewport = Math.min(Math.max(all.length, 1), VIEWPORT_ROWS) * ROW_HEIGHT;
  const { start, end } = visibleRange(scrollTop, viewport, all.length);

  const choose = (item: T) => {
    if (multiple) {
      const next = selectedIds.has(item.id) ? value.filter((v) => v.id !== item.id) : [...value, item];
      onChange(next);
      if (!selectedIds.has(item.id)) writeRecent(recentKey, [item]);
    } else {
      onChange([item]);
      writeRecent(recentKey, [item]);
      setOpen(false);
      setText("");
    }
  };

  const onScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    setScrollTop(el.scrollTop);
    if (cursor && !loading && el.scrollTop + el.clientHeight >= el.scrollHeight - ROW_HEIGHT * 3) {
      void load(query, cursor);
    }
  };

  const reveal = (i: number) => {
    const el = scrollRef.current;
    if (!el) return;
    if (i * ROW_HEIGHT < el.scrollTop) el.scrollTop = i * ROW_HEIGHT;
    else if ((i + 1) * ROW_HEIGHT > el.scrollTop + el.clientHeight) el.scrollTop = (i + 1) * ROW_HEIGHT - el.clientHeight;
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (!open) {
        setOpen(true);
        return;
      }
      const next = Math.max(0, Math.min(all.length - 1, active + (e.key === "ArrowDown" ? 1 : -1)));
      setActive(next);
      reveal(next);
      if (next >= all.length - 3 && cursor && !loading) void load(query, cursor);
    } else if (e.key === "Enter" && open && all[active]) {
      e.preventDefault();
      choose(all[active]);
    } else if (e.key === "Escape" && open) {
      e.preventDefault();
      setOpen(false);
    } else if (e.key === "Backspace" && !text && multiple && value.length > 0) {
      onChange(value.slice(0, -1));
    } else if (e.key === "Home" && open) {
      setActive(0);
      reveal(0);
    }
  };

  const shown = multiple && !expanded ? value.slice(0, maxChips) : value;
  const hidden = multiple ? value.length - shown.length : 0;
  const single = !multiple ? value[0] : undefined;

  return (
    <Box ref={rootRef} sx={{ position: "relative", minWidth, flex: 1 }} data-testid={testId ? `${testId}-root` : undefined}>
      {multiple && value.length > 0 && (
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mb: 0.5 }} data-testid="picker-chips">
          {shown.map((v) => (
            <Chip
              key={v.id}
              size="sm"
              variant="soft"
              startDecorator={<Avatar size="sm" src={v.avatarUrl} sx={{ "--Avatar-size": "18px", fontSize: 10 }}>{v.label.slice(0, 1)}</Avatar>}
              endDecorator={
                <ChipDelete
                  aria-label={`Remove ${v.label}`}
                  disabled={disabled}
                  onDelete={() => onChange(value.filter((x) => x.id !== v.id))}
                />
              }
            >
              {v.label}
            </Chip>
          ))}
          {hidden > 0 && (
            <Chip size="sm" variant="outlined" onClick={() => setExpanded(true)} slotProps={{ action: { "aria-label": `Show ${hidden} more selected` } }}>
              +{hidden}
            </Chip>
          )}
          {expanded && value.length > maxChips && (
            <Chip size="sm" variant="outlined" onClick={() => setExpanded(false)}>
              Show less
            </Chip>
          )}
        </Box>
      )}
      <Input
        size={size}
        disabled={disabled}
        placeholder={single && !open ? single.label : placeholder}
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          if (!open) setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={onKeyDown}
        endDecorator={loading ? <CircularProgress size="sm" sx={{ "--CircularProgress-size": "16px" }} /> : null}
        slotProps={{
          input: {
            role: "combobox",
            "aria-label": ariaLabel,
            "aria-expanded": open,
            "aria-controls": listId,
            "aria-autocomplete": "list",
            "aria-activedescendant": open && all[active] ? `${uid}-opt-${active}` : undefined,
            "data-testid": testId,
            autoComplete: "off",
          },
        }}
      />
      {open && (
        <Box
          sx={{
            position: "absolute",
            zIndex: 1300,
            left: 0,
            right: 0,
            mt: 0.5,
            bgcolor: "background.popup",
            border: "1px solid",
            borderColor: "neutral.outlinedBorder",
            borderRadius: "md",
            boxShadow: "md",
            minWidth: 280,
          }}
        >
          <Box
            ref={scrollRef}
            id={listId}
            role="listbox"
            aria-multiselectable={multiple || undefined}
            aria-label={ariaLabel}
            onScroll={onScroll}
            data-testid="picker-list"
            sx={{ maxHeight: VIEWPORT_ROWS * ROW_HEIGHT, overflowY: "auto", position: "relative" }}
          >
            <Box sx={{ height: all.length * ROW_HEIGHT, position: "relative" }}>
              {all.slice(start, end).map((item, k) => {
                const i = start + k;
                const sel = selectedIds.has(item.id);
                return (
                  <Box
                    key={`${i < shortcutCount ? "s" : "r"}-${item.id}`}
                    id={`${uid}-opt-${i}`}
                    role="option"
                    aria-selected={sel}
                    data-testid="picker-option"
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => choose(item)}
                    onMouseMove={() => active !== i && setActive(i)}
                    sx={{
                      position: "absolute",
                      top: i * ROW_HEIGHT,
                      left: 0,
                      right: 0,
                      height: ROW_HEIGHT,
                      px: 1.25,
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                      cursor: "pointer",
                      bgcolor: i === active ? "background.level1" : "transparent",
                    }}
                  >
                    {renderIcon ? (
                      renderIcon(item)
                    ) : (
                      <Avatar size="sm" src={item.avatarUrl}>
                        {item.label.slice(0, 1).toUpperCase()}
                      </Avatar>
                    )}
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                      <Typography level="body-sm" noWrap sx={{ fontWeight: 600 }}>
                        {item.label}
                        {i < shortcutCount && me && item.id === me.id ? " (me)" : ""}
                      </Typography>
                      {item.secondary && (
                        <Typography level="body-xs" noWrap>
                          {item.secondary}
                        </Typography>
                      )}
                      {item.tertiary && (
                        <Typography level="body-xs" noWrap sx={{ color: "text.tertiary" }}>
                          {item.tertiary}
                        </Typography>
                      )}
                    </Box>
                    {sel && <CheckIcon fontSize="small" color="success" />}
                  </Box>
                );
              })}
            </Box>
          </Box>
          {error && (
            <Box sx={{ p: 1, display: "flex", alignItems: "center", gap: 1 }} role="alert">
              <Typography level="body-xs" color="danger" sx={{ flex: 1 }}>
                {error}
              </Typography>
              <Chip size="sm" variant="outlined" onClick={() => void load(query, cursor)}>
                Retry
              </Chip>
            </Box>
          )}
          {!error && !loading && all.length === 0 && (
            <Typography level="body-sm" sx={{ p: 1.5, color: "text.tertiary" }} data-testid="picker-empty">
              {emptyText}
            </Typography>
          )}
          {loading && all.length > 0 && (
            <Typography level="body-xs" sx={{ p: 0.75, textAlign: "center", color: "text.tertiary" }}>
              Loading more…
            </Typography>
          )}
        </Box>
      )}
    </Box>
  );
}
