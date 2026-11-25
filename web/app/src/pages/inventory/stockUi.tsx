import { useCallback, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { Alert, Box, Chip, CircularProgress, Sheet, Typography } from "@mui/joy";
import { getProduct, listProducts, listProductsPage } from "./api";
import { AsyncPicker, type PickerItem, type PickerPage } from "../../components/AsyncPicker";
import type { InvProduct } from "./types";

/** Loads data with a reload() and error state; re-runs when `deps` change. */
export function useLoad<T>(fn: () => Promise<T>, deps: unknown[]) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const seq = useRef(0);
  const fnRef = useRef(fn);
  fnRef.current = fn;
  const reload = useCallback(async () => {
    const my = ++seq.current;
    setLoading(true);
    try {
      const v = await fnRef.current();
      if (my === seq.current) {
        setData(v);
        setError("");
      }
    } catch (e) {
      if (my === seq.current) setError((e as Error).message);
    } finally {
      if (my === seq.current) setLoading(false);
    }
  }, []);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { void reload(); }, deps);
  return { data, error, loading, reload, setData };
}

/** Runs an action, showing its error. */
export function useAction() {
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    setBusy(true);
    setError("");
    try {
      return await fn();
    } catch (e) {
      setError((e as Error).message);
      return undefined;
    } finally {
      setBusy(false);
    }
  }, []);
  return { error, setError, busy, run };
}

export function ErrorBar({ error, onClose }: { error: string; onClose?: () => void }) {
  if (!error) return null;
  return (
    <Alert color="danger" variant="soft" sx={{ my: 1 }} role="alert" endDecorator={onClose ? <Chip size="sm" onClick={onClose}>Dismiss</Chip> : undefined}>
      {error}
    </Alert>
  );
}

export function Loading({ show }: { show: boolean }) {
  return show ? (
    <Box sx={{ display: "flex", justifyContent: "center", py: 3 }}>
      <CircularProgress size="sm" />
    </Box>
  ) : null;
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <Sheet variant="soft" sx={{ p: 3, textAlign: "center", borderRadius: "md" }}>
      <Typography level="body-md">{children}</Typography>
    </Sheet>
  );
}

export function Title({ children, right }: { children: ReactNode; right?: ReactNode }) {
  return (
    <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
      <Typography level="title-lg" sx={{ flex: 1, minWidth: 160 }}>
        {children}
      </Typography>
      {right}
    </Box>
  );
}

export const right = { textAlign: "right" } as const;

let productCache: { at: number; list: InvProduct[] } | null = null;

/** Flat list of products (variants as rows) for pickers; cached for 30 seconds. */
export async function loadProducts(force = false): Promise<InvProduct[]> {
  if (!force && productCache && Date.now() - productCache.at < 30000) return productCache.list;
  const list = await listProducts({ flat: true });
  productCache = { at: Date.now(), list };
  return list;
}

export function productLabel(p: InvProduct): string {
  return `${p.name}${p.variant_label ? ` / ${p.variant_label}` : ""}${p.sku ? ` [${p.sku}]` : ""}`;
}

/** Product picker with type-ahead, searched on the server a page at a time. */
export function ProductPicker({
  value,
  onChange,
  placeholder = "Product",
  size = "sm",
  testId,
}: {
  value: string;
  onChange: (id: string, p?: InvProduct) => void;
  placeholder?: string;
  size?: "sm" | "md";
  testId?: string;
}) {
  const [picked, setPicked] = useState<InvProduct | null>(null);
  const [known, setKnown] = useState<InvProduct | null>(null);
  useEffect(() => {
    if (!value || picked?.id === value || known?.id === value) return;
    let live = true;
    getProduct(value)
      .then((p) => live && setKnown(p))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [value, picked, known]);
  const fetchPage = useCallback(async (q: string, cursor: string | undefined): Promise<PickerPage<PickerItem & { p: InvProduct }>> => {
    const r = await listProductsPage({ q, flat: true, limit: 30 }, cursor ?? "");
    return { items: r.items.filter((p) => !p.archived).map((p) => ({ id: p.id, label: productLabel(p), secondary: p.category || undefined, p })), next_cursor: r.next || undefined };
  }, []);
  const cur = picked?.id === value ? picked : known?.id === value ? known : null;
  const shown = value ? [{ id: value, label: cur ? productLabel(cur) : "…", p: (cur ?? { id: value }) as InvProduct }] : [];
  return (
    <AsyncPicker<PickerItem & { p: InvProduct }>
      fetchPage={fetchPage}
      value={shown}
      onChange={(n) => {
        setPicked(n[0]?.p ?? null);
        onChange(n[0]?.id ?? "", n[0]?.p);
      }}
      placeholder={placeholder}
      ariaLabel={placeholder}
      testId={testId}
      size={size}
      minWidth={200}
      recentKey="inv-products"
      renderIcon={() => null}
      emptyText="No products match."
    />
  );
}

export function dateTime(s: string | null | undefined): string {
  if (!s) return "";
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

export function dateOnly(s: string | null | undefined): string {
  if (!s) return "";
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? s : d.toLocaleDateString(undefined, { dateStyle: "medium" });
}
