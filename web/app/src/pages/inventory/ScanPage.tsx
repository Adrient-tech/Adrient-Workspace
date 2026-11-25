import { useEffect, useRef, useState } from "react";
import { Alert, Box, Button, Chip, FormControl, FormLabel, IconButton, Input, Option, Select, Sheet, Tab, TabList, Tabs, Typography } from "@mui/joy";
import { api } from "./stockApi";
import type { LookupResult, Po, Transfer } from "./stockApi";
import { qtyText } from "./logic";
import { cleanScan } from "./stockLogic";
import { ErrorBar, Loading, useAction, useLoad } from "./stockUi";
import { ScanDialog } from "./ScanDialog";

type Mode = "receive" | "pick" | "putaway" | "lookup";

/** Phone-friendly receiving, picking and lookup by barcode: USB/Bluetooth scanner (types + Enter) or the camera. */
export function ScanPage({ version, onChanged }: { version: number; onChanged: () => void }) {
  const [mode, setMode] = useState<Mode>("receive");
  return (
    <Box sx={{ maxWidth: 560, mx: "auto" }}>
      <Tabs value={mode} onChange={(_, v) => setMode(v as Mode)} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ mb: 2 }} tabFlex={1}>
          <Tab value="receive" data-testid="scan-tab-receive">Receive</Tab>
          <Tab value="pick" data-testid="scan-tab-pick">Pick</Tab>
          <Tab value="putaway" data-testid="scan-tab-putaway">Arrivals</Tab>
          <Tab value="lookup" data-testid="scan-tab-lookup">Look up</Tab>
        </TabList>
      </Tabs>
      {mode === "receive" && <ReceiveScan version={version} onChanged={onChanged} />}
      {mode === "pick" && <TransferScan kind="ship" version={version} onChanged={onChanged} />}
      {mode === "putaway" && <TransferScan kind="receive" version={version} onChanged={onChanged} />}
      {mode === "lookup" && <Lookup />}
    </Box>
  );
}

/** Input that accepts scanner keystrokes (Enter ends a scan) or camera scans. */
function ScanBox({ onCode, disabled }: { onCode: (code: string) => void; disabled?: boolean }) {
  const [v, setV] = useState("");
  const [cam, setCam] = useState(false);
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => { ref.current?.focus(); }, []);
  const submit = (raw: string) => {
    const c = cleanScan(raw);
    setV("");
    if (c) onCode(c);
    ref.current?.focus();
  };
  return (
    <Box sx={{ display: "flex", gap: 1, my: 1.5 }}>
      <Input
        size="lg"
        sx={{ flex: 1 }}
        placeholder="Scan or type a barcode, then Enter"
        value={v}
        disabled={disabled}
        onChange={(e) => setV(e.target.value)}
        onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); submit(v); } }}
        slotProps={{ input: { ref, inputMode: "text", autoComplete: "off", autoCapitalize: "off", "data-testid": "scan-input" } }}
      />
      <Button size="lg" variant="outlined" onClick={() => setCam(true)} data-testid="scan-camera">Camera</Button>
      <ScanDialog open={cam} onClose={() => setCam(false)} onCode={(c) => { setCam(false); submit(c); }} />
    </Box>
  );
}

function Flash({ text, tone }: { text: string; tone: "success" | "warning" | "danger" }) {
  if (!text) return null;
  return <Alert color={tone} variant="soft" role="status" sx={{ mb: 1 }} data-testid="scan-flash">{text}</Alert>;
}

// ----------------------------------------------------------- receive a PO

interface RLine { line_id: string; product_id: string; name: string; sku: string; tracking: string; ordered: number; received: number; scanned: number; lot: string; expiry: string; serials: string[] }

function ReceiveScan({ version, onChanged }: { version: number; onChanged: () => void }) {
  const pos = useLoad(() => api.orders(), [version]);
  const open = (pos.data ?? []).filter((o) => ["approved", "sent", "acknowledged", "partial"].includes(o.state));
  const [po, setPo] = useState<Po | null>(null);
  const [lines, setLines] = useState<RLine[]>([]);
  const [flash, setFlash] = useState<{ text: string; tone: "success" | "warning" | "danger" }>({ text: "", tone: "success" });
  const act = useAction();
  const choose = (id: string) => {
    const o = open.find((x) => x.id === id) ?? null;
    setPo(o);
    setFlash({ text: "", tone: "success" });
    setLines((o?.lines ?? []).map((l) => ({ line_id: l.id, product_id: l.product_id, name: l.product_name, sku: l.sku, tracking: l.tracking, ordered: l.qty, received: l.qty_received, scanned: 0, lot: "", expiry: "", serials: [] })));
  };
  const onCode = async (code: string) => {
    let r: LookupResult;
    try { r = await api.lookup(code); } catch { setFlash({ text: `"${code}" is not a known barcode, SKU, lot or serial.`, tone: "danger" }); return; }
    const i = lines.findIndex((l) => l.product_id === r.product_id);
    if (i < 0) { setFlash({ text: `${r.name} is not on this order.`, tone: "danger" }); return; }
    const l = lines[i];
    if (l.tracking === "serial") {
      if (r.kind === "product") { setFlash({ text: `${l.name} needs serial numbers: scan each unit's serial label.`, tone: "warning" }); return; }
      if (l.serials.includes(code)) { setFlash({ text: `Serial ${code} was already scanned.`, tone: "warning" }); return; }
      setLines((ls) => ls.map((x, j) => (j === i ? { ...x, serials: [...x.serials, code], scanned: x.scanned + 1 } : x)));
      setFlash({ text: `${l.name}: serial ${code} added`, tone: "success" });
      return;
    }
    setLines((ls) => ls.map((x, j) => (j === i ? { ...x, scanned: Math.round((x.scanned + 1) * 1000) / 1000 } : x)));
    const total = l.received + l.scanned + 1;
    setFlash(total > l.ordered ? { text: `${l.name}: ${qtyText(total)} is more than the ${qtyText(l.ordered)} ordered`, tone: "warning" } : { text: `${l.name}: ${qtyText(l.scanned + 1)} scanned`, tone: "success" });
  };
  const book = async () => {
    if (!po) return;
    const got = lines.filter((l) => l.scanned > 0);
    if (got.length === 0) { setFlash({ text: "Scan at least one item first.", tone: "warning" }); return; }
    const r = await act.run(() => api.receive({ po_id: po.id, lines: got.map((l) => ({ po_line_id: l.line_id, qty: l.scanned, lot_no: l.lot || undefined, expiry: l.expiry || undefined, serials: l.tracking === "serial" ? l.serials : undefined })) }));
    if (r) { setFlash({ text: `Booked ${r.number}`, tone: "success" }); onChanged(); setPo(null); setLines([]); void pos.reload(); }
  };
  return (
    <Box>
      <ErrorBar error={pos.error || act.error} />
      <Loading show={pos.loading && !pos.data} />
      <FormControl>
        <FormLabel>Purchase order</FormLabel>
        <Select size="lg" value={po?.id ?? ""} placeholder={open.length === 0 ? "No open orders to receive" : "Choose an order"} onChange={(_, v) => choose(v ?? "")} data-testid="scan-po">
          {open.map((o) => <Option key={o.id} value={o.id}>{o.number} · {o.supplier_name}</Option>)}
        </Select>
      </FormControl>
      {po && (
        <>
          <ScanBox onCode={onCode} />
          <Flash {...flash} />
          {lines.map((l) => (
            <Sheet key={l.line_id} variant="outlined" sx={{ p: 1.25, borderRadius: "md", mb: 1 }} data-testid={`scan-line-${l.sku || l.product_id}`}>
              <Box sx={{ display: "flex", justifyContent: "space-between", gap: 1 }}>
                <Typography level="title-sm">{l.name}</Typography>
                <Chip size="md" variant="soft" color={l.scanned === 0 ? "neutral" : l.received + l.scanned >= l.ordered ? "success" : "warning"} data-testid={`scan-count-${l.sku || l.product_id}`}>
                  {qtyText(l.scanned)} / {qtyText(Math.max(0, l.ordered - l.received))}
                </Chip>
              </Box>
              {l.tracking === "lot" && (
                <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
                  <Input size="sm" placeholder="Lot number" value={l.lot} onChange={(e) => setLines((ls) => ls.map((x) => (x.line_id === l.line_id ? { ...x, lot: e.target.value } : x)))} slotProps={{ input: { "data-testid": `scan-lot-${l.sku || l.product_id}` } }} />
                  <Input size="sm" type="date" value={l.expiry} onChange={(e) => setLines((ls) => ls.map((x) => (x.line_id === l.line_id ? { ...x, expiry: e.target.value } : x)))} slotProps={{ input: { "aria-label": "Expiry" } }} />
                </Box>
              )}
              <Box sx={{ display: "flex", gap: 1, mt: 1 }}>
                <IconButton size="sm" variant="outlined" aria-label="One less" disabled={l.scanned <= 0 || l.tracking === "serial"} onClick={() => setLines((ls) => ls.map((x) => (x.line_id === l.line_id ? { ...x, scanned: Math.max(0, x.scanned - 1) } : x)))}>−</IconButton>
                <IconButton size="sm" variant="outlined" aria-label="One more" disabled={l.tracking === "serial"} onClick={() => setLines((ls) => ls.map((x) => (x.line_id === l.line_id ? { ...x, scanned: x.scanned + 1 } : x)))}>+</IconButton>
              </Box>
            </Sheet>
          ))}
          <Button size="lg" fullWidth loading={act.busy} onClick={book} data-testid="scan-book">Book receipt</Button>
        </>
      )}
    </Box>
  );
}

// ------------------------------------------- pick (ship) / receive a transfer

function TransferScan({ kind, version, onChanged }: { kind: "ship" | "receive"; version: number; onChanged: () => void }) {
  const want = kind === "ship" ? "draft" : "in_transit";
  const trs = useLoad(() => api.transfers(want), [version, want]);
  const [tr, setTr] = useState<Transfer | null>(null);
  const [done, setDone] = useState<Record<string, number>>({});
  const [flash, setFlash] = useState<{ text: string; tone: "success" | "warning" | "danger" }>({ text: "", tone: "success" });
  const act = useAction();
  const target = (l: Transfer["lines"][number]) => (kind === "ship" ? l.qty : Math.round((l.qty_shipped - l.qty_received) * 1000) / 1000);
  const choose = (id: string) => { setTr((trs.data ?? []).find((t) => t.id === id) ?? null); setDone({}); setFlash({ text: "", tone: "success" }); };
  const onCode = async (code: string) => {
    if (!tr) return;
    let r: LookupResult;
    try { r = await api.lookup(code); } catch { setFlash({ text: `"${code}" is not a known barcode.`, tone: "danger" }); return; }
    const l = tr.lines.find((x) => x.product_id === r.product_id);
    if (!l) { setFlash({ text: `${r.name} is not on this transfer.`, tone: "danger" }); return; }
    const next = (done[l.id] ?? 0) + 1;
    if (next > target(l)) { setFlash({ text: `${l.product_name}: already ${qtyText(target(l))} of ${qtyText(target(l))}`, tone: "warning" }); return; }
    setDone((d) => ({ ...d, [l.id]: next }));
    setFlash({ text: `${l.product_name}: ${qtyText(next)} of ${qtyText(target(l))}`, tone: "success" });
  };
  const complete = tr ? tr.lines.every((l) => (done[l.id] ?? 0) >= target(l)) : false;
  const finish = async () => {
    if (!tr) return;
    const r = await act.run(() => (kind === "ship" ? api.shipTransfer(tr.id) : api.receiveTransfer(tr.id, tr.lines.filter((l) => (done[l.id] ?? 0) > 0).map((l) => ({ line_id: l.id, qty: done[l.id] }))))
    );
    if (r) { setFlash({ text: kind === "ship" ? `${tr.number} shipped` : `${tr.number} received`, tone: "success" }); onChanged(); setTr(null); setDone({}); void trs.reload(); }
  };
  return (
    <Box>
      <ErrorBar error={trs.error || act.error} />
      <Loading show={trs.loading && !trs.data} />
      <FormControl>
        <FormLabel>{kind === "ship" ? "Transfer to pick" : "Transfer arriving"}</FormLabel>
        <Select size="lg" value={tr?.id ?? ""} placeholder={(trs.data ?? []).length === 0 ? "Nothing waiting" : "Choose a transfer"} onChange={(_, v) => choose(v ?? "")} data-testid="scan-transfer">
          {(trs.data ?? []).map((t) => <Option key={t.id} value={t.id}>{t.number} · {t.from_name} → {t.to_name}</Option>)}
        </Select>
      </FormControl>
      {tr && (
        <>
          <ScanBox onCode={onCode} />
          <Flash {...flash} />
          {tr.lines.map((l) => (
            <Sheet key={l.id} variant="outlined" sx={{ p: 1.25, borderRadius: "md", mb: 1, display: "flex", justifyContent: "space-between", gap: 1 }}>
              <Typography level="title-sm">{l.product_name}</Typography>
              <Chip size="md" variant="soft" color={(done[l.id] ?? 0) >= target(l) ? "success" : "neutral"} data-testid={`scan-tr-count-${l.sku || l.product_id}`}>{qtyText(done[l.id] ?? 0)} / {qtyText(target(l))}</Chip>
            </Sheet>
          ))}
          <Button size="lg" fullWidth disabled={kind === "ship" ? !complete : Object.keys(done).length === 0} loading={act.busy} onClick={finish} data-testid="scan-finish">
            {kind === "ship" ? "Ship picked items" : "Receive scanned items"}
          </Button>
          {kind === "ship" && !complete && <Typography level="body-xs" sx={{ mt: 0.5 }}>Scan every line to the full quantity to enable shipping.</Typography>}
        </>
      )}
    </Box>
  );
}

// ------------------------------------------------------------------ lookup

function Lookup() {
  const [r, setR] = useState<LookupResult | null>(null);
  const [err, setErr] = useState("");
  return (
    <Box>
      <ScanBox onCode={async (c) => { setErr(""); try { setR(await api.lookup(c)); } catch { setR(null); setErr(`Nothing found for "${c}".`); } }} />
      {err && <Alert color="danger" variant="soft">{err}</Alert>}
      {r && (
        <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md" }} data-testid="scan-result">
          <Typography level="title-md">{r.name}</Typography>
          <Typography level="body-sm">{r.kind === "product" ? "Product" : r.kind === "lot" ? "Lot of" : "Serial number of"} {r.sku}</Typography>
          {r.kind === "product" && <Typography level="body-md">On hand: <b>{qtyText(r.stock ?? 0)}</b> {r.unit}</Typography>}
          {r.kind === "serial" && <Typography level="body-md">State: <b>{r.state === "in_stock" ? "in stock" : "gone out"}</b></Typography>}
        </Sheet>
      )}
    </Box>
  );
}
