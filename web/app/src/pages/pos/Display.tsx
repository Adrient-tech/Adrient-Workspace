import { useEffect, useState } from "react";
import { Box, Typography } from "@mui/joy";

export const DISPLAY_CHANNEL = "adrient-pos-display";

export interface DisplayState {
  kind: "idle" | "cart" | "thanks";
  store: string;
  currency: string;
  lines: { name: string; qty: number; total: number }[];
  subtotal: number;
  discount: number;
  tax: number;
  total: number;
  change: number;
  at: number;
}

export const idleDisplay = (store: string, currency: string): DisplayState => ({ kind: "idle", store, currency, lines: [], subtotal: 0, discount: 0, tax: 0, total: 0, change: 0, at: Date.now() });

/** Publishes the register's state to a customer-facing window on the same device. */
export function publishDisplay(s: DisplayState) {
  try {
    if (typeof BroadcastChannel === "undefined") return;
    const ch = new BroadcastChannel(DISPLAY_CHANNEL);
    ch.postMessage(s);
    ch.close();
  } catch {
    /* no display open: nothing to do */
  }
  try {
    localStorage.setItem(DISPLAY_CHANNEL, JSON.stringify(s)); // lets a display opened later show the current sale
  } catch {
    /* storage full or blocked */
  }
}

const fmt = (n: number, c: string) => {
  try {
    return new Intl.NumberFormat(undefined, { style: "currency", currency: c || "INR" }).format(n);
  } catch {
    return `${c} ${n.toFixed(2)}`;
  }
};

/** Customer-facing display at /pos/display: open it in a window on the second screen. It only shows what the register publishes; it never reads the API. */
export default function CustomerDisplay() {
  const [s, setS] = useState<DisplayState>(() => {
    try {
      const raw = localStorage.getItem(DISPLAY_CHANNEL);
      if (raw) return JSON.parse(raw) as DisplayState;
    } catch {
      /* ignore */
    }
    return idleDisplay("", "INR");
  });
  useEffect(() => {
    if (typeof BroadcastChannel === "undefined") return;
    const ch = new BroadcastChannel(DISPLAY_CHANNEL);
    ch.onmessage = (e: MessageEvent<DisplayState>) => setS(e.data);
    return () => ch.close();
  }, []);
  // "Thank you" goes back to the welcome screen after a few seconds.
  useEffect(() => {
    if (s.kind !== "thanks") return;
    const t = window.setTimeout(() => setS((x) => ({ ...x, kind: "idle" })), 8000);
    return () => window.clearTimeout(t);
  }, [s]);
  const m = (n: number) => fmt(n, s.currency);
  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "#0f172a", color: "#fff", p: { xs: 2, md: 6 }, display: "flex", flexDirection: "column" }} data-testid="pos-display">
      <Typography level="h2" sx={{ color: "#fff" }}>
        {s.store || "Welcome"}
      </Typography>
      {s.kind === "idle" && (
        <Box sx={{ flex: 1, display: "grid", placeItems: "center" }}>
          <Typography level="h1" sx={{ color: "#94a3b8" }} data-testid="pos-display-idle">
            Welcome
          </Typography>
        </Box>
      )}
      {s.kind === "thanks" && (
        <Box sx={{ flex: 1, display: "grid", placeItems: "center", textAlign: "center" }}>
          <Box>
            <Typography level="h1" sx={{ color: "#4ade80" }} data-testid="pos-display-thanks">
              Thank you!
            </Typography>
            {s.change > 0 && (
              <Typography level="h2" sx={{ color: "#fff", mt: 2 }}>
                Your change: {m(s.change)}
              </Typography>
            )}
          </Box>
        </Box>
      )}
      {s.kind === "cart" && (
        <>
          <Box sx={{ flex: 1, mt: 3 }} data-testid="pos-display-lines">
            {s.lines.map((l, i) => (
              <Box key={i} sx={{ display: "flex", justifyContent: "space-between", py: 1, borderBottom: "1px solid #334155", fontSize: { xs: 20, md: 28 } }}>
                <span>
                  {l.qty} × {l.name}
                </span>
                <span>{m(l.total)}</span>
              </Box>
            ))}
          </Box>
          <Box sx={{ mt: 3, fontSize: { xs: 18, md: 24 }, color: "#cbd5e1" }}>
            {s.discount > 0 && <Box sx={{ display: "flex", justifyContent: "space-between" }}><span>Discount</span><span>-{m(s.discount)}</span></Box>}
            {s.tax > 0 && <Box sx={{ display: "flex", justifyContent: "space-between" }}><span>Tax</span><span>{m(s.tax)}</span></Box>}
            <Box sx={{ display: "flex", justifyContent: "space-between", fontSize: { xs: 32, md: 56 }, color: "#fff", fontWeight: 700, mt: 1 }}>
              <span>Total</span>
              <span data-testid="pos-display-total">{m(s.total)}</span>
            </Box>
          </Box>
        </>
      )}
    </Box>
  );
}
