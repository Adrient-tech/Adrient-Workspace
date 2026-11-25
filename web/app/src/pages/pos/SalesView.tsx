import { usePermissions } from "../../lib/permissions";
import { useCallback, useEffect, useState } from "react";
import { DataTable, type Column, type TablePage, type TableQuery } from "../../components/DataTable";
import {
  Box,
  Button,
  Checkbox,
  Chip,
  DialogActions,
  FormControl,
  FormLabel,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Sheet,
  Typography } from "@mui/joy";
import { getSale } from "./api";
import { getExt, getExtCaps, refundWithPin } from "./extApi";
import type { ExtSettings } from "./extApi";
import { EmailReceiptDialog, VoidDialog } from "./PosExtDialogs";
import { methodLabel, money, qtyText, round2, statusLabel } from "./logic";
import type { PosRegister, PosSale, PosSettings } from "./types";
import { ErrorAlert, Receipt, emailReceiptHref, useErr } from "./ui";

export function SalesView({
  registers,
  settings,
  registerId,
  version,
  onChanged,
}: {
  registers: PosRegister[];
  settings: PosSettings;
  registerId: string;
  version: number;
  onChanged: () => void;
}) {
  const [status, setStatus] = useState("");
  const [query, setQuery] = useState("");
  const [qd, setQd] = useState("");
  const [open, setOpen] = useState("");
  const [tick, setTick] = useState(0);
  const m = (n: number) => money(n, settings.currency);
  useEffect(() => {
    const t = setTimeout(() => setQd(query), 250);
    return () => clearTimeout(t);
  }, [query]);
  // Keyset-paged on the server: GET /api/v1/pos/x/sales/page.
  const load = useCallback(
    async (t: TableQuery, signal: AbortSignal): Promise<TablePage<PosSale>> => {
      const p = new URLSearchParams({ limit: String(t.limit) });
      if (t.cursor) p.set("cursor", t.cursor);
      if (status) p.set("status", status);
      if (qd) p.set("q", qd);
      const r = await fetch(`/api/v1/pos/x/sales/page?${p}`, { credentials: "same-origin", signal });
      if (!r.ok) throw new Error((await r.text()).trim() || `HTTP ${r.status}`);
      const d = (await r.json()) as { items: PosSale[]; next_cursor?: string };
      return { items: d.items ?? [], next_cursor: d.next_cursor };
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [status, qd, version, tick],
  );
  const columns: Column<PosSale>[] = [
    { key: "receipt", header: "Receipt", width: 100, render: (s) => (s.status === "held" ? "Held" : `#${String(s.number)}`) },
    { key: "date", header: "Date", width: 190, render: (s) => new Date(s.completed_at || s.created_at).toLocaleString() },
    { key: "customer", header: "Customer", render: (s) => s.customer_name },
    { key: "register", header: "Register", render: (s) => s.register_name },
    {
      key: "status",
      header: "Status",
      width: 130,
      render: (s) => (
        <Chip size="sm" variant="soft" color={s.status === "completed" ? "success" : s.status === "held" ? "neutral" : "warning"}>
          {statusLabel(s.status)}
        </Chip>
      ),
    },
    { key: "total", header: "Total", width: 130, render: (s) => <Box sx={{ textAlign: "right" }}>{m(s.total)}</Box> },
  ];

  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, mb: 2, flexWrap: "wrap" }}>
        <Input placeholder="Search receipt #, customer, note" value={query} onChange={(e) => setQuery(e.target.value)} sx={{ flex: 1, minWidth: 220 }} slotProps={{ input: { "data-testid": "pos-sales-search" } }} />
        <Select value={status || "all"} onChange={(_, v) => setStatus(v === "all" ? "" : (v as string))} sx={{ minWidth: 150 }}>
          <Option value="all">All sales</Option>
          <Option value="completed">Completed</Option>
          <Option value="refunded">Refunded</Option>
          <Option value="held">Held</Option>
        </Select>
      </Box>
      <DataTable<PosSale>
        testId="pos-sales-table"
        columns={columns}
        rowKey={(s) => s.id}
        load={load}
        mode="cursor"
        search={false}
        urlPrefix="sales_"
        onRowClick={(s) => setOpen(s.id)}
        emptyText="No sales yet."
      />
      {open && (
        <SaleDialog
          id={open}
          registers={registers}
          registerId={registerId}
          settings={settings}
          onClose={() => setOpen("")}
          onChanged={() => {
            setTick((n) => n + 1);
            onChanged();
          }}
        />
      )}
    </Box>
  );
}

function SaleDialog({
  id,
  registers,
  registerId,
  settings,
  onClose,
  onChanged,
}: {
  id: string;
  registers: PosRegister[];
  registerId: string;
  settings: PosSettings;
  onClose: () => void;
  onChanged: () => void;
}) {
  const [sale, setSale] = useState<PosSale | null>(null);
  const [refunding, setRefunding] = useState(false);
  const [qty, setQty] = useState<Record<string, number>>({});
  const [method, setMethod] = useState("cash");
  const [restock, setRestock] = useState(true);
  const [regId, setRegId] = useState(registerId);
  const [ext, setExt] = useState<ExtSettings | null>(null);
  const [serverEmail, setServerEmail] = useState(false);
  const [pin, setPin] = useState("");
  const [voiding, setVoiding] = useState(false);
  const [emailing, setEmailing] = useState(false);
  const { err, run } = useErr();
  const { can } = usePermissions();
  const m = (n: number) => money(n, settings.currency);
  useEffect(() => {
    getSale(id).then(setSale).catch(() => setSale(null));
    getExt().then(setExt).catch(() => setExt(null));
    getExtCaps().then((c) => setServerEmail(c.email)).catch(() => undefined);
  }, [id]);
  if (!sale) return null;
  const canRefund = (sale.status === "completed" || sale.status === "partially_refunded") && (can("pos:refund") || can("pos:sell"));
  const canVoid = sale.status === "completed" && can("pos:sell");
  const needPin = ext?.refund_needs_pin ?? true;
  const picked = sale.lines
    .map((l) => ({ line: l, q: qty[l.id] ?? 0 }))
    .filter((x) => x.q > 0);
  const est = round2(
    picked.reduce((a, { line, q }) => {
      const left = round2(line.qty - line.refunded_qty);
      return a + (q >= left ? line.line_total - line.refunded_amount : (line.line_total * q) / line.qty);
    }, 0),
  );
  const doRefund = async () => {
    const r = await run(() =>
      refundWithPin(sale.id, {
        register_id: regId,
        lines: picked.map(({ line, q }) => ({ line_id: line.id, qty: q })),
        method,
        restock,
        pin,
      }),
    );
    if (r) {
      setPin("");
      setSale(r.sale);
      setRefunding(false);
      setQty({});
      onChanged();
    }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 460, maxWidth: "96vw", maxHeight: "94vh", overflow: "auto" }} data-testid="pos-sale-dialog">
        <ModalClose />
        <Typography level="title-lg">
          {sale.status === "held" ? "Held sale" : `Sale #${String(sale.number)}`} <Chip size="sm">{statusLabel(sale.status)}</Chip>
        </Typography>
        {sale.status !== "held" && <Receipt sale={sale} settings={settings} />}
        {sale.refunded_total > 0 && <Typography level="body-sm">Refunded so far: {m(sale.refunded_total)}</Typography>}
        {refunding && (
          <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md" }} data-testid="pos-refund-form">
            <Typography level="title-sm" sx={{ mb: 1 }}>
              Choose what comes back
            </Typography>
            {sale.lines.map((l) => {
              const left = round2(l.qty - l.refunded_qty);
              return (
                <Box key={l.id} sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }}>
                  <Box sx={{ flex: 1 }}>
                    {l.name} <Typography level="body-xs" component="span">({qtyText(left)} left)</Typography>
                  </Box>
                  <Input
                    type="number"
                    size="sm"
                    disabled={left <= 0}
                    value={qty[l.id] ?? ""}
                    placeholder="0"
                    onChange={(e) => setQty({ ...qty, [l.id]: Math.min(left, Math.max(0, parseFloat(e.target.value) || 0)) })}
                    sx={{ width: 80 }}
                    slotProps={{ input: { min: 0, max: left, step: "any", "data-testid": "pos-refund-qty" } }}
                  />
                  <Button size="sm" variant="plain" disabled={left <= 0} onClick={() => setQty({ ...qty, [l.id]: left })}>
                    All
                  </Button>
                </Box>
              );
            })}
            <Box sx={{ display: "flex", gap: 1, mt: 1, flexWrap: "wrap" }}>
              <FormControl sx={{ flex: 1, minWidth: 130 }}>
                <FormLabel>Refund to</FormLabel>
                <Select value={method} onChange={(_, v) => v && setMethod(v)}>
                  <Option value="cash">Cash</Option>
                  <Option value="card">Card</Option>
                  <Option value="other">Other</Option>
                  {sale.customer_id && <Option value="store_credit">Store credit</Option>}
                </Select>
              </FormControl>
              <FormControl sx={{ flex: 1, minWidth: 130 }}>
                <FormLabel>Register</FormLabel>
                <Select value={regId} onChange={(_, v) => v && setRegId(v)}>
                  {registers.map((r) => (
                    <Option key={r.id} value={r.id}>
                      {r.name}
                    </Option>
                  ))}
                </Select>
              </FormControl>
            </Box>
            <Checkbox sx={{ mt: 1 }} label="Return items to stock" checked={restock} onChange={(e) => setRestock(e.target.checked)} />
            {needPin && (
              <FormControl sx={{ mt: 1 }}>
                <FormLabel>Manager PIN</FormLabel>
                <Input type="password" value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, "").slice(0, 8))} slotProps={{ input: { inputMode: "numeric", autoComplete: "off", "data-testid": "pos-refund-pin" } }} />
              </FormControl>
            )}
            <ErrorAlert err={err} />
            <DialogActions>
              <Button variant="plain" onClick={() => setRefunding(false)}>
                Cancel
              </Button>
              <Button color="danger" disabled={!picked.length || (needPin && pin.length < 4)} onClick={doRefund} data-testid="pos-refund-confirm">
                Refund {m(est)}
              </Button>
            </DialogActions>
          </Sheet>
        )}
        <Typography level="body-xs">
          {sale.payments.map((p) => `${methodLabel(p.method)} ${m(p.amount)}`).join(" · ")}
          {sale.created_by && ` · by ${sale.created_by}`}
        </Typography>
        <DialogActions sx={{ flexWrap: "wrap" }}>
          {sale.status !== "held" && (
            <>
              <Button variant="outlined" onClick={() => window.print()} data-testid="pos-sale-print">
                Print
              </Button>
              <Button variant="outlined" onClick={() => setEmailing(true)} data-testid="pos-sale-email">
                Email
              </Button>
            </>
          )}
          {canRefund && !refunding && (
            <Button variant="outlined" color="danger" onClick={() => setRefunding(true)} data-testid="pos-refund-btn">
              Refund
            </Button>
          )}
          {canVoid && !refunding && (
            <Button variant="outlined" color="danger" onClick={() => setVoiding(true)} data-testid="pos-void-btn">
              Void
            </Button>
          )}
          {voiding && (
            <VoidDialog
              saleId={sale.id}
              number={String(sale.number)}
              registerId={regId}
              needsPin={ext?.void_needs_pin ?? true}
              onClose={() => setVoiding(false)}
              onDone={() => {
                setVoiding(false);
                getSale(sale.id).then(setSale);
                onChanged();
              }}
            />
          )}
          {emailing && <EmailReceiptDialog saleId={sale.id} defaultTo="" serverEmail={serverEmail} fallbackHref={emailReceiptHref(sale, settings)} onClose={() => setEmailing(false)} />}
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}
