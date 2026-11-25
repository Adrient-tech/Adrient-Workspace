import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import { Box, Button, IconButton, Input, Option, Select, Sheet, Skeleton, Table, Typography } from "@mui/joy";
import ArrowUpwardIcon from "@mui/icons-material/ArrowUpward";
import ArrowDownwardIcon from "@mui/icons-material/ArrowDownward";
import ChevronLeftIcon from "@mui/icons-material/ChevronLeft";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";
import FileDownloadOutlinedIcon from "@mui/icons-material/FileDownloadOutlined";

export interface Column<T> {
  key: string;
  header: string;
  render: (row: T) => ReactNode;
  /** Sorted on the server by this key. */
  sortable?: boolean;
  width?: number | string;
}

export interface FilterDef {
  key: string;
  label: string;
  /** "select" needs options; "date" is a date input; "text" a text input; "custom" renders `render`. */
  type: "select" | "date" | "text" | "custom";
  options?: { value: string; label: string }[];
  render?: (value: string, set: (v: string) => void) => ReactNode;
}

/** What the table sends to `load` and to the export URL. */
export interface TableQuery {
  limit: number;
  /** Keyset cursor of the page to load (cursor mode). */
  cursor?: string;
  /** 1-based page (page mode). */
  page: number;
  sort: string;
  order: "asc" | "desc";
  q: string;
  filters: Record<string, string>;
}

export interface TablePage<T> {
  items: T[];
  next_cursor?: string;
  total_estimate?: number;
  total_capped?: boolean;
}

export interface DataTableProps<T> {
  columns: Column<T>[];
  rowKey: (row: T) => string;
  load: (q: TableQuery, signal: AbortSignal) => Promise<TablePage<T>>;
  /**
   * cursor: previous/next over keyset pages (stable for feeds, no page jump).
   * page:   the server takes `page`, so the user can jump to any page.
   * feed:   infinite scroll; pages append as the list scrolls.
   */
  mode?: "cursor" | "page" | "feed";
  pageSizes?: number[];
  defaultSort?: string;
  defaultOrder?: "asc" | "desc";
  filters?: FilterDef[];
  searchPlaceholder?: string;
  /** Show the free-text search box (default true). */
  search?: boolean;
  /** Returns the URL of a streaming CSV export with the same filters. */
  exportUrl?: (q: TableQuery) => string;
  /** Prefix for the URL parameters when a screen holds several tables. */
  urlPrefix?: string;
  emptyText?: string;
  testId?: string;
  /** Increment to force a reload (after a mutation). */
  reloadKey?: number;
  onRowClick?: (row: T) => void;
  /** Rows above this count are windowed. */
  virtualAbove?: number;
  maxHeight?: number;
}

export const ROW_H = 40;

/** "1–50 of 12,340" / "1–50 of 10,000+" / "1–50" when the total is unknown. */
export function rangeLabel(from: number, to: number, total?: number, capped?: boolean): string {
  if (to === 0) return "0 results";
  const base = `${from.toLocaleString("en-US")}–${to.toLocaleString("en-US")}`;
  if (total === undefined) return base;
  return `${base} of ${total.toLocaleString("en-US")}${capped ? "+" : ""}`;
}

/** Window of rows to render (exported for tests). */
export function rowWindow(scrollTop: number, viewport: number, count: number, overscan = 5) {
  return {
    start: Math.max(0, Math.floor(scrollTop / ROW_H) - overscan),
    end: Math.min(count, Math.ceil((scrollTop + viewport) / ROW_H) + overscan),
  };
}

export function DataTable<T>(props: DataTableProps<T>) {
  const {
    columns,
    rowKey,
    load,
    mode = "cursor",
    pageSizes = [25, 50, 100],
    defaultSort = "",
    defaultOrder = "desc",
    filters = [],
    searchPlaceholder = "Search",
    search = true,
    exportUrl,
    urlPrefix = "",
    emptyText = "Nothing to show",
    testId,
    reloadKey = 0,
    onRowClick,
    virtualAbove = 60,
    maxHeight = 560,
  } = props;
  const [sp, setSp] = useSearchParams();
  const k = (n: string) => urlPrefix + n;

  const size = useMemo(() => {
    const n = Number(sp.get(k("size")));
    return pageSizes.includes(n) ? n : pageSizes[0];
  }, [sp, pageSizes]); // eslint-disable-line react-hooks/exhaustive-deps
  const sort = sp.get(k("sort")) ?? defaultSort;
  const order = (sp.get(k("order")) as "asc" | "desc" | null) ?? defaultOrder;
  const qUrl = sp.get(k("q")) ?? "";
  const page = Math.max(1, Number(sp.get(k("page"))) || 1);
  const cursor = sp.get(k("cur")) || undefined;
  const filterVals = useMemo(() => {
    const o: Record<string, string> = {};
    for (const f of filters) {
      const v = sp.get(k("f_" + f.key));
      if (v) o[f.key] = v;
    }
    return o;
  }, [sp, filters]); // eslint-disable-line react-hooks/exhaustive-deps

  const setParams = useCallback(
    (patch: Record<string, string | undefined>, resetPaging = false) => {
      setSp(
        (old) => {
          const n = new URLSearchParams(old);
          if (resetPaging) {
            n.delete(k("page"));
            n.delete(k("cur"));
          }
          for (const [key, v] of Object.entries(patch)) {
            if (v === undefined || v === "") n.delete(k(key));
            else n.set(k(key), v);
          }
          return n;
        },
        { replace: true },
      );
    },
    [setSp, urlPrefix], // eslint-disable-line react-hooks/exhaustive-deps
  );

  const [qText, setQText] = useState(qUrl);
  useEffect(() => setQText(qUrl), [qUrl]);
  useEffect(() => {
    if (qText === qUrl) return;
    const t = setTimeout(() => setParams({ q: qText }, true), 300);
    return () => clearTimeout(t);
  }, [qText, qUrl, setParams]);

  const [rows, setRows] = useState<T[]>([]);
  const [next, setNext] = useState<string | undefined>();
  const [total, setTotal] = useState<number | undefined>();
  const [capped, setCapped] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  // Cursor history of this session so "Previous" works in cursor mode.
  const trail = useRef<(string | undefined)[]>([]);
  const gen = useRef(0);
  const ctl = useRef<AbortController | null>(null);

  const query: TableQuery = useMemo(
    () => ({ limit: size, cursor, page, sort, order, q: qUrl, filters: filterVals }),
    [size, cursor, page, sort, order, qUrl, filterVals],
  );
  const queryKey = JSON.stringify([size, sort, order, qUrl, filterVals, mode === "feed" ? "" : cursor, mode === "page" ? page : 0, reloadKey]);

  const run = useCallback(
    async (q: TableQuery, append: boolean) => {
      ctl.current?.abort();
      const c = new AbortController();
      ctl.current = c;
      const mine = ++gen.current;
      setLoading(true);
      setError("");
      try {
        const r = await load(q, c.signal);
        if (mine !== gen.current) return;
        setRows((old) => (append ? [...old, ...r.items] : r.items));
        setNext(r.next_cursor || undefined);
        if (r.total_estimate !== undefined) {
          setTotal(r.total_estimate);
          setCapped(!!r.total_capped);
        } else if (!append) {
          setTotal(undefined);
        }
      } catch (e) {
        if (mine !== gen.current || (e as Error).name === "AbortError") return;
        setError((e as Error).message || "Could not load");
      } finally {
        if (mine === gen.current) setLoading(false);
      }
    },
    [load],
  );

  useEffect(() => {
    void run(query, false);
    return () => ctl.current?.abort();
    // queryKey captures every input of `query`
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryKey, run]);

  const toggleSort = (key: string) => {
    if (sort === key) setParams({ order: order === "asc" ? "desc" : "asc" }, true);
    else setParams({ sort: key, order: "asc" }, true);
  };

  const goNext = () => {
    if (!next) return;
    if (mode === "page") setParams({ page: String(page + 1) });
    else {
      trail.current.push(cursor);
      setParams({ cur: next, page: String(page + 1) });
    }
  };
  const goPrev = () => {
    if (mode === "page") setParams({ page: String(Math.max(1, page - 1)) });
    else setParams({ cur: trail.current.pop() ?? "", page: String(Math.max(1, page - 1)) });
  };

  // Infinite scroll for feeds.
  const scroller = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const onScroll = () => {
    const el = scroller.current;
    if (!el) return;
    setScrollTop(el.scrollTop);
    if (mode === "feed" && next && !loading && el.scrollTop + el.clientHeight >= el.scrollHeight - ROW_H * 4) {
      void run({ ...query, cursor: next }, true);
    }
  };

  const windowed = rows.length > virtualAbove;
  const win = windowed ? rowWindow(scrollTop, maxHeight, rows.length) : { start: 0, end: rows.length };
  const baseIndex = mode === "feed" ? 0 : (page - 1) * size;
  const from = rows.length ? baseIndex + 1 : 0;
  const to = baseIndex + rows.length;
  const lastPage = total !== undefined && !capped ? Math.max(1, Math.ceil(total / size)) : undefined;
  const hasPrev = page > 1;

  const csvHref = exportUrl ? exportUrl(query) : undefined;

  return (
    <Box data-testid={testId} sx={{ display: "flex", flexDirection: "column", gap: 1, minWidth: 0 }}>
      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, alignItems: "center" }}>
        {search && (
        <Input
          size="sm"
          placeholder={searchPlaceholder}
          value={qText}
          onChange={(e) => setQText(e.target.value)}
          sx={{ minWidth: 220 }}
          slotProps={{ input: { "aria-label": searchPlaceholder, "data-testid": "table-search" } }}
        />
        )}
        {filters.map((f) =>
          f.type === "select" ? (
            <Select
              key={f.key}
              size="sm"
              placeholder={f.label}
              value={filterVals[f.key] ?? ""}
              onChange={(_, v) => setParams({ ["f_" + f.key]: (v as string) || undefined }, true)}
              slotProps={{ button: { "aria-label": f.label } }}
              sx={{ minWidth: 130 }}
            >
              <Option value="">{f.label}: all</Option>
              {(f.options ?? []).map((o) => (
                <Option key={o.value} value={o.value}>
                  {o.label}
                </Option>
              ))}
            </Select>
          ) : f.type === "custom" && f.render ? (
            <Box key={f.key}>{f.render(filterVals[f.key] ?? "", (v) => setParams({ ["f_" + f.key]: v || undefined }, true))}</Box>
          ) : (
            <Input
              key={f.key}
              size="sm"
              type={f.type === "date" ? "date" : "text"}
              value={filterVals[f.key] ?? ""}
              onChange={(e) => setParams({ ["f_" + f.key]: e.target.value || undefined }, true)}
              startDecorator={<Typography level="body-xs">{f.label}</Typography>}
              slotProps={{ input: { "aria-label": f.label, title: f.label } }}
              sx={{ width: f.type === "date" ? 260 : 200 }}
            />
          ),
        )}
        <Box sx={{ flex: 1 }} />
        {csvHref && (
          <Button size="sm" variant="outlined" component="a" href={csvHref} startDecorator={<FileDownloadOutlinedIcon />} download>
            Export CSV
          </Button>
        )}
      </Box>

      <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "hidden" }}>
        <Box ref={scroller} onScroll={onScroll} sx={{ maxHeight: windowed || mode === "feed" ? maxHeight : "none", overflow: "auto" }} data-testid="table-scroll">
          <Table
            size="sm"
            stickyHeader
            aria-busy={loading}
            sx={{ "--TableCell-height": `${ROW_H}px`, tableLayout: "fixed", minWidth: 560, "& td": { whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" } }}
          >
            <thead>
              <tr>
                {columns.map((c) => (
                  <th
                    key={c.key}
                    style={{ width: c.width }}
                    aria-sort={sort === c.key ? (order === "asc" ? "ascending" : "descending") : undefined}
                  >
                    {c.sortable ? (
                      <Button
                        size="sm"
                        variant="plain"
                        color="neutral"
                        onClick={() => toggleSort(c.key)}
                        endDecorator={sort === c.key ? order === "asc" ? <ArrowUpwardIcon fontSize="small" /> : <ArrowDownwardIcon fontSize="small" /> : null}
                        sx={{ px: 0.5, fontWeight: 700 }}
                      >
                        {c.header}
                      </Button>
                    ) : (
                      c.header
                    )}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {loading && rows.length === 0 &&
                Array.from({ length: Math.min(size, 8) }).map((_, i) => (
                  <tr key={`sk-${i}`} data-testid="table-skeleton">
                    {columns.map((c) => (
                      <td key={c.key}>
                        <Skeleton variant="text" width="70%" />
                      </td>
                    ))}
                  </tr>
                ))}
              {windowed && win.start > 0 && (
                <tr aria-hidden style={{ height: win.start * ROW_H }}>
                  <td colSpan={columns.length} style={{ padding: 0, border: 0 }} />
                </tr>
              )}
              {rows.slice(win.start, win.end).map((row) => (
                <tr
                  key={rowKey(row)}
                  onClick={onRowClick ? () => onRowClick(row) : undefined}
                  style={{ height: ROW_H, cursor: onRowClick ? "pointer" : undefined, opacity: loading ? 0.6 : 1 }}
                >
                  {columns.map((c) => (
                    <td key={c.key}>{c.render(row)}</td>
                  ))}
                </tr>
              ))}
              {windowed && win.end < rows.length && (
                <tr aria-hidden style={{ height: (rows.length - win.end) * ROW_H }}>
                  <td colSpan={columns.length} style={{ padding: 0, border: 0 }} />
                </tr>
              )}
            </tbody>
          </Table>
        </Box>
        {!loading && !error && rows.length === 0 && (
          <Typography level="body-sm" sx={{ p: 3, textAlign: "center", color: "text.tertiary" }} data-testid="table-empty">
            {qUrl || Object.keys(filterVals).length ? "No results match these filters." : emptyText}
          </Typography>
        )}
        {error && (
          <Box role="alert" sx={{ p: 2, display: "flex", gap: 1, alignItems: "center" }}>
            <Typography level="body-sm" color="danger" sx={{ flex: 1 }}>
              {error}
            </Typography>
            <Button size="sm" variant="outlined" onClick={() => void run(query, false)}>
              Retry
            </Button>
          </Box>
        )}
      </Sheet>

      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
        <Typography level="body-xs" data-testid="table-range" aria-live="polite">
          {rangeLabel(from, to, mode === "feed" ? undefined : total, capped)}
          {mode === "feed" && total !== undefined ? ` of ${total.toLocaleString("en-US")}${capped ? "+" : ""}` : ""}
        </Typography>
        <Box sx={{ flex: 1 }} />
        {mode !== "feed" ? (
          <>
            <Typography level="body-xs">Rows</Typography>
            <Select
              size="sm"
              value={size}
              onChange={(_, v) => v && setParams({ size: String(v) }, true)}
              slotProps={{ button: { "aria-label": "Rows per page" } }}
            >
              {pageSizes.map((n) => (
                <Option key={n} value={n}>
                  {n}
                </Option>
              ))}
            </Select>
            {mode === "page" && lastPage !== undefined && lastPage > 1 && (
              <Input
                size="sm"
                type="number"
                value={page}
                onChange={(e) => {
                  const n = Math.min(lastPage, Math.max(1, Number(e.target.value) || 1));
                  setParams({ page: String(n) });
                }}
                slotProps={{ input: { min: 1, max: lastPage, "aria-label": "Go to page" } }}
                endDecorator={<Typography level="body-xs">/ {lastPage.toLocaleString("en-US")}</Typography>}
                sx={{ width: 120 }}
              />
            )}
            <IconButton size="sm" variant="outlined" aria-label="Previous page" disabled={!hasPrev || loading} onClick={goPrev}>
              <ChevronLeftIcon />
            </IconButton>
            <IconButton size="sm" variant="outlined" aria-label="Next page" disabled={!next || loading} onClick={goNext}>
              <ChevronRightIcon />
            </IconButton>
          </>
        ) : (
          loading && rows.length > 0 && <Typography level="body-xs">Loading more…</Typography>
        )}
      </Box>
    </Box>
  );
}
