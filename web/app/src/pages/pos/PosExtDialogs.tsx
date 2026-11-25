import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, Box, Button, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Sheet, Table, Textarea, Typography } from "@mui/joy";
import { emailReceipt, getGiftCard, getZReport, verifyPin, voidSale } from "./extApi";
import type { GiftCard, ZReport } from "./extApi";
import { defaultStore, flushOutbox, httpSender } from "./outbox";
import type { OutboxEntry, OutboxStore } from "./outbox";
import { methodLabel, money } from "./logic";
import { ErrorAlert, useErr } from "./ui";

// ---- offline outbox ----

let sharedStore: OutboxStore | null = null;
const store = () => (sharedStore ??= defaultStore());

export interface OutboxState {
  entries: OutboxEntry[];
  online: boolean;
  syncing: boolean;
  lastSynced: number; // count of sales sent by the last run
  flush: () => Promise<void>;
  enqueue: (e: OutboxEntry) => Promise<void>;
  discard: (key: string) => Promise<void>;
  retry: (e: OutboxEntry) => Promise<void>;
}

/** Offline outbox: queue sales, send them when the connection is back, expose the state to the UI. */
export function useOutbox(onSynced?: () => void): OutboxState {
  const [entries, setEntries] = useState<OutboxEntry[]>([]);
  const [online, setOnline] = useState(typeof navigator === "undefined" ? true : navigator.onLine);
  const [syncing, setSyncing] = useState(false);
  const [lastSynced, setLastSynced] = useState(0);
  const busy = useRef(false);
  const cb = useRef(onSynced);
  cb.current = onSynced;

  const reload = useCallback(async () => setEntries(await store().list().catch(() => [])), []);
  const flush = useCallback(async () => {
    if (busy.current) return;
    busy.current = true;
    setSyncing(true);
    try {
      const r = await flushOutbox(store(), httpSender);
      setLastSynced(r.sent);
      if (r.sent > 0) cb.current?.();
    } finally {
      busy.current = false;
      setSyncing(false);
      reload();
    }
  }, [reload]);

  useEffect(() => {
    reload().then(() => flush());
    const on = () => {
      setOnline(true);
      flush();
    };
    const off = () => setOnline(false);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    const t = window.setInterval(() => flush(), 20000);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
      window.clearInterval(t);
    };
  }, [flush, reload]);

  return {
    entries,
    online,
    syncing,
    lastSynced,
    flush,
    enqueue: async (e) => {
      await store().put(e);
      await reload();
    },
    discard: async (key) => {
      await store().remove(key);
      await reload();
    },
    retry: async (e) => {
      await store().put({ ...e, status: "queued", error: "" });
      await reload();
      flush();
    },
  };
}

export function OfflineBar({ ob, currency }: { ob: OutboxState; currency: string }) {
  const queued = ob.entries.filter((e) => e.status === "queued");
  const failed = ob.entries.filter((e) => e.status === "failed");
  const [open, setOpen] = useState(false);
  if (ob.online && !queued.length && !failed.length) return null;
  return (
    <Box sx={{ mb: 1.5 }} data-testid="pos-offline-bar">
      <Alert color={failed.length ? "danger" : ob.online ? "warning" : "neutral"} sx={{ alignItems: "center", flexWrap: "wrap", gap: 1 }}>
        <Typography level="body-sm" sx={{ flex: 1 }} data-testid="pos-offline-text">
          {!ob.online ? "You are offline. Sales are saved on this device and will sync when the connection returns. " : ""}
          {queued.length > 0 && `${queued.length} sale(s) waiting to sync. `}
          {failed.length > 0 && `${failed.length} sale(s) were rejected and need attention.`}
        </Typography>
        {queued.length > 0 && ob.online && (
          <Button size="sm" loading={ob.syncing} onClick={ob.flush} data-testid="pos-sync-now">
            Sync now
          </Button>
        )}
        {(queued.length > 0 || failed.length > 0) && (
          <Button size="sm" variant="plain" onClick={() => setOpen(true)} data-testid="pos-outbox-open">
            Details
          </Button>
        )}
      </Alert>
      {open && (
        <Modal open onClose={() => setOpen(false)}>
          <ModalDialog sx={{ width: "min(640px, 96vw)", maxHeight: "90vh", overflow: "auto" }} data-testid="pos-outbox-dialog">
            <ModalClose />
            <Typography level="title-lg">Sales saved on this device</Typography>
            <Typography level="body-sm">Each sale has its own key, so syncing twice never creates a duplicate.</Typography>
            <Table size="sm">
              <tbody>
                {ob.entries.map((e) => (
                  <tr key={e.key}>
                    <td>{new Date(e.created_at).toLocaleString()}</td>
                    <td>
                      {e.summary.lines} item(s) · {money(e.summary.total, currency)}
                    </td>
                    <td>
                      <Chip size="sm" color={e.status === "failed" ? "danger" : "warning"} variant="soft">
                        {e.status === "failed" ? "Rejected" : "Waiting"}
                      </Chip>
                      {e.error && <Typography level="body-xs">{e.error}</Typography>}
                    </td>
                    <td style={{ textAlign: "right" }}>
                      {e.status === "failed" && (
                        <Button size="sm" variant="plain" onClick={() => ob.retry(e)}>
                          Retry
                        </Button>
                      )}
                      <Button
                        size="sm"
                        variant="plain"
                        color="danger"
                        onClick={() => {
                          if (window.confirm("Discard this sale? The customer's payment will not be recorded.")) ob.discard(e.key);
                        }}
                      >
                        Discard
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

// ---- manager PIN ----

/** Asks for a manager PIN. onApproved receives the PIN so the server can re-check it on the action itself. */
export function PinDialog({
  title,
  scope,
  onClose,
  onApproved,
}: {
  title: string;
  scope: "void" | "refund" | "discount";
  onClose: () => void;
  onApproved: (pin: string, approver: string) => void;
}) {
  const [pin, setPin] = useState("");
  const [busy, setBusy] = useState(false);
  const { err, run } = useErr();
  const go = async () => {
    setBusy(true);
    const r = await run(() => verifyPin(pin, scope));
    setBusy(false);
    if (r) onApproved(pin, r.approver.name);
    else setPin("");
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 340 }} data-testid="pos-pin-dialog">
        <ModalClose />
        <Typography level="title-lg">{title}</Typography>
        <Typography level="body-sm">A manager types their PIN to approve.</Typography>
        <Input
          type="password"
          autoFocus
          placeholder="Manager PIN"
          value={pin}
          onChange={(e) => setPin(e.target.value.replace(/\D/g, "").slice(0, 8))}
          onKeyDown={(e) => e.key === "Enter" && pin.length >= 4 && go()}
          slotProps={{ input: { inputMode: "numeric", autoComplete: "off", "data-testid": "pos-pin-input" } }}
        />
        <ErrorAlert err={err} />
        <Button onClick={go} loading={busy} disabled={pin.length < 4} data-testid="pos-pin-ok">
          Approve
        </Button>
      </ModalDialog>
    </Modal>
  );
}

// ---- void ----

export function VoidDialog({ saleId, number, registerId, needsPin, onClose, onDone }: { saleId: string; number: string; registerId: string; needsPin: boolean; onClose: () => void; onDone: () => void }) {
  const [reason, setReason] = useState("");
  const [pin, setPin] = useState("");
  const [busy, setBusy] = useState(false);
  const { err, run } = useErr();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 420 }} data-testid="pos-void-dialog">
        <ModalClose />
        <Typography level="title-lg">Void sale #{number}</Typography>
        <Typography level="body-sm">The whole sale is cancelled, the goods go back to stock and the payment is returned. The reason is kept on record.</Typography>
        <FormControl>
          <FormLabel>Reason</FormLabel>
          <Textarea minRows={2} value={reason} onChange={(e) => setReason(e.target.value)} slotProps={{ textarea: { "data-testid": "pos-void-reason" } }} />
        </FormControl>
        {needsPin && (
          <FormControl>
            <FormLabel>Manager PIN</FormLabel>
            <Input type="password" value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, "").slice(0, 8))} slotProps={{ input: { inputMode: "numeric", autoComplete: "off", "data-testid": "pos-void-pin" } }} />
          </FormControl>
        )}
        <ErrorAlert err={err} />
        <Button
          color="danger"
          loading={busy}
          disabled={reason.trim().length < 3 || (needsPin && pin.length < 4)}
          data-testid="pos-void-go"
          onClick={async () => {
            setBusy(true);
            const r = await run(() => voidSale(saleId, { reason, pin, register_id: registerId }));
            setBusy(false);
            if (r) onDone();
          }}
        >
          Void sale
        </Button>
      </ModalDialog>
    </Modal>
  );
}

// ---- gift card lookup (used by the payment dialog) ----

export function GiftCardLookup({ onUse, remaining, currency }: { onUse: (code: string, amount: number) => void; remaining: number; currency: string }) {
  const [code, setCode] = useState("");
  const [card, setCard] = useState<GiftCard | null>(null);
  const { err, run, setErr } = useErr();
  const m = (n: number) => money(n, currency);
  const find = async () => {
    setCard(null);
    const c = await run(() => getGiftCard(code.trim()));
    if (c) {
      if (c.status !== "active") setErr("This card is disabled.");
      else if (c.expires_on && c.expires_on < new Date().toISOString().slice(0, 10)) setErr(`This card expired on ${c.expires_on}.`);
      else if (c.balance <= 0) setErr("This card has no balance left.");
      else setCard(c);
    }
  };
  return (
    <Box sx={{ display: "grid", gap: 0.75 }} data-testid="pos-gift-lookup">
      <Box sx={{ display: "flex", gap: 1 }}>
        <Input placeholder="Gift card code" value={code} onChange={(e) => setCode(e.target.value.toUpperCase())} onKeyDown={(e) => e.key === "Enter" && code && find()} sx={{ flex: 1 }} slotProps={{ input: { "data-testid": "pos-gift-code" } }} />
        <Button variant="outlined" disabled={!code.trim()} onClick={find} data-testid="pos-gift-find">
          Check
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {card && (
        <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <Typography level="body-sm">
            Balance <b data-testid="pos-gift-balance">{m(card.balance)}</b>
          </Typography>
          <Button size="sm" data-testid="pos-gift-use" onClick={() => onUse(card.code, Math.min(card.balance, remaining))}>
            Use {m(Math.min(card.balance, remaining))}
          </Button>
        </Box>
      )}
    </Box>
  );
}

// ---- email receipt (server-side) ----

export function EmailReceiptDialog({ saleId, defaultTo, serverEmail, onClose, fallbackHref }: { saleId: string; defaultTo: string; serverEmail: boolean; onClose: () => void; fallbackHref: string }) {
  const [to, setTo] = useState(defaultTo);
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  const { err, run } = useErr();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 380 }} data-testid="pos-email-dialog">
        <ModalClose />
        <Typography level="title-lg">Email receipt</Typography>
        {!serverEmail && <Alert color="warning">Server email is not set up. Your own mail app will open instead.</Alert>}
        <Input type="email" placeholder="customer@example.com" value={to} onChange={(e) => setTo(e.target.value)} slotProps={{ input: { "data-testid": "pos-email-to" } }} />
        <ErrorAlert err={err} />
        {done && (
          <Typography color="success" level="body-sm" data-testid="pos-email-sent">
            Receipt sent to {to}.
          </Typography>
        )}
        {serverEmail ? (
          <Button
            loading={busy}
            disabled={!to.includes("@")}
            data-testid="pos-email-send"
            onClick={async () => {
              setBusy(true);
              const r = await run(() => emailReceipt(saleId, to));
              setBusy(false);
              if (r) setDone(true);
            }}
          >
            Send
          </Button>
        ) : (
          <Button component="a" href={fallbackHref}>
            Open mail app
          </Button>
        )}
      </ModalDialog>
    </Modal>
  );
}

// ---- Z report ----

export function ZReportDialog({ shiftId, currency, onClose }: { shiftId: string; currency: string; onClose: () => void }) {
  const [z, setZ] = useState<ZReport | null>(null);
  const { err, run } = useErr();
  useEffect(() => {
    run(() => getZReport(shiftId)).then((r) => r && setZ(r));
  }, [shiftId, run]);
  const m = (n: number) => money(n, currency);
  const row = { display: "flex", justifyContent: "space-between", py: 0.25 } as const;
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(460px, 96vw)", maxHeight: "94vh", overflow: "auto", bgcolor: "#f3f4f6" }} data-testid="pos-z-dialog">
        <ModalClose />
        <style>{`@media print { body * { visibility: hidden !important; } .pos-z, .pos-z * { visibility: visible !important; } .pos-z { position: absolute; left: 0; top: 0; width: 80mm; box-shadow: none !important; border: 0 !important; } .pos-noprint { display: none !important; } @page { size: 80mm auto; margin: 3mm; } }`}</style>
        <ErrorAlert err={err} />
        {z && (
          <>
            <Box className="pos-noprint">
              <Button size="sm" onClick={() => window.print()} data-testid="pos-z-print">
                Print Z report
              </Button>
            </Box>
            <Sheet className="pos-z" data-testid="pos-z-sheet" sx={{ p: 2, fontFamily: "ui-monospace, Menlo, monospace", fontSize: 13, bgcolor: "#fff", color: "#000" }}>
              <Box sx={{ textAlign: "center", fontWeight: 700, fontSize: 15 }}>Z REPORT {z.closed ? "" : "(shift still open)"}</Box>
              <Box sx={{ textAlign: "center" }}>
                {z.store ? `${z.store} · ` : ""}
                {z.register}
              </Box>
              <Box sx={{ textAlign: "center", mb: 1 }}>
                {z.opened_at.slice(0, 16).replace("T", " ")} to {z.closed_at ? z.closed_at.slice(0, 16).replace("T", " ") : "now"}
              </Box>
              <Box sx={row}>
                <span>Sales ({z.sales})</span>
                <span data-testid="pos-z-gross">{m(z.gross)}</span>
              </Box>
              <Box sx={row}>
                <span>of which tax</span>
                <span>{m(z.tax)}</span>
              </Box>
              <Box sx={row}>
                <span>Discounts given</span>
                <span>{m(z.discounts)}</span>
              </Box>
              <Box sx={row}>
                <span>Refunds ({z.refund_count})</span>
                <span>-{m(z.refunds)}</span>
              </Box>
              <Box sx={row}>
                <span>Voids ({z.voids})</span>
                <span>{m(z.void_total)}</span>
              </Box>
              <Box sx={{ ...row, fontWeight: 700, borderTop: "1px dashed #999", mt: 0.5 }}>
                <span>Net sales</span>
                <span data-testid="pos-z-net">{m(z.net)}</span>
              </Box>
              <Box sx={{ mt: 1, fontWeight: 700 }}>Takings by method</Box>
              {z.by_method.map((x) => (
                <Box key={x.method} sx={row}>
                  <span>
                    {methodLabel(x.method)} ({x.count})
                  </span>
                  <span>{m(x.amount)}</span>
                </Box>
              ))}
              {z.refund_by_method.length > 0 && <Box sx={{ mt: 1, fontWeight: 700 }}>Refunded by method</Box>}
              {z.refund_by_method.map((x) => (
                <Box key={x.method} sx={row}>
                  <span>{methodLabel(x.method)}</span>
                  <span>-{m(x.amount)}</span>
                </Box>
              ))}
              <Box sx={{ mt: 1, fontWeight: 700 }}>Cash drawer</Box>
              <Box sx={row}>
                <span>Opening float</span>
                <span>{m(z.opening_cash)}</span>
              </Box>
              <Box sx={row}>
                <span>Cash sales</span>
                <span>{m(z.cash_sales)}</span>
              </Box>
              <Box sx={row}>
                <span>Cash refunds</span>
                <span>-{m(z.cash_refunds)}</span>
              </Box>
              <Box sx={row}>
                <span>Paid in / out</span>
                <span>{m(z.paid_in_out)}</span>
              </Box>
              <Box sx={{ ...row, fontWeight: 700, borderTop: "1px dashed #999" }}>
                <span>Expected cash</span>
                <span data-testid="pos-z-expected">{m(z.expected_cash)}</span>
              </Box>
              {z.closed && (
                <>
                  <Box sx={row}>
                    <span>Counted</span>
                    <span>{m(z.counted_cash)}</span>
                  </Box>
                  <Box sx={{ ...row, fontWeight: 700, color: Math.abs(z.variance) > 0.004 ? "#B91C1C" : undefined }}>
                    <span>Variance</span>
                    <span data-testid="pos-z-variance">{m(z.variance)}</span>
                  </Box>
                </>
              )}
            </Sheet>
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}

