import { useState } from "react";
import { Box, Button, Chip, FormControl, FormLabel, IconButton, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { api } from "./stockApi";
import type { Loc, Transfer } from "./stockApi";
import { qtyText } from "./logic";
import { Empty, ErrorBar, Loading, ProductPicker, Title, dateTime, right, useAction, useLoad } from "./stockUi";

const TONE = { draft: "neutral", in_transit: "warning", received: "success", cancelled: "danger" } as const;
const LABEL = { draft: "Draft", in_transit: "In transit", received: "Received", cancelled: "Cancelled" } as const;

export function TransfersView({ version, onChanged }: { version: number; onChanged: () => void }) {
  const list = useLoad(() => api.transfers(), [version]);
  const locs = useLoad(() => api.locations(), [version]);
  const [creating, setCreating] = useState(false);
  const [receiving, setReceiving] = useState<Transfer | null>(null);
  const act = useAction();
  const done = () => { onChanged(); void list.reload(); };
  return (
    <Box>
      <Title right={<Button size="sm" onClick={() => setCreating(true)} data-testid="tr-new">New transfer</Button>}>Stock transfers</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Shipping takes the stock out of the source and holds it in transit. The destination receives it later, all at once or in parts. Lots and serial
        numbers travel with the goods.
      </Typography>
      <ErrorBar error={list.error || act.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>No transfers yet.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="tr-table">
            <thead>
              <tr>
                <th>Number</th>
                <th>From → To</th>
                <th>Items</th>
                <th>Status</th>
                <th>Created</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {list.data.map((t) => (
                <tr key={t.id}>
                  <td>{t.number}</td>
                  <td>{t.from_name} → {t.to_name}</td>
                  <td>
                    {t.lines.map((l) => (
                      <div key={l.id}>
                        {l.product_name}: {qtyText(l.qty)}
                        {t.state === "in_transit" && <Typography level="body-xs" component="span"> (received {qtyText(l.qty_received)} of {qtyText(l.qty_shipped)})</Typography>}
                      </div>
                    ))}
                  </td>
                  <td><Chip size="sm" color={TONE[t.state]} variant="soft" data-testid={`tr-state-${t.number}`}>{LABEL[t.state]}</Chip></td>
                  <td>{dateTime(t.created_at)}</td>
                  <td style={{ ...right, whiteSpace: "nowrap" }}>
                    {t.state === "draft" && (
                      <Button size="sm" variant="plain" onClick={async () => { if (await act.run(() => api.shipTransfer(t.id))) done(); }} data-testid={`tr-ship-${t.number}`}>Ship</Button>
                    )}
                    {t.state === "in_transit" && (
                      <>
                        <Button size="sm" variant="plain" onClick={async () => { if (await act.run(() => api.receiveTransfer(t.id))) done(); }} data-testid={`tr-receive-all-${t.number}`}>Receive all</Button>
                        <Button size="sm" variant="plain" onClick={() => setReceiving(t)} data-testid={`tr-receive-${t.number}`}>Receive part</Button>
                      </>
                    )}
                    {(t.state === "draft" || t.state === "in_transit") && (
                      <Button
                        size="sm"
                        variant="plain"
                        color="danger"
                        onClick={async () => {
                          if (t.state === "in_transit" && !window.confirm("Cancel and return the goods in transit to the source?")) return;
                          if (await act.run(() => api.cancelTransfer(t.id))) done();
                        }}
                      >
                        Cancel
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {creating && <NewTransfer locations={locs.data ?? []} onClose={() => setCreating(false)} onDone={() => { setCreating(false); done(); }} />}
      {receiving && <ReceivePart t={receiving} onClose={() => setReceiving(null)} onDone={() => { setReceiving(null); done(); }} />}
    </Box>
  );
}

function NewTransfer({ locations, onClose, onDone }: { locations: Loc[]; onClose: () => void; onDone: () => void }) {
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [note, setNote] = useState("");
  const [lines, setLines] = useState<{ product_id: string; qty: string }[]>([{ product_id: "", qty: "1" }]);
  const act = useAction();
  const save = async (ship: boolean) => {
    const body = { from_loc_id: from, to_loc_id: to, note, ship, lines: lines.filter((l) => l.product_id).map((l) => ({ product_id: l.product_id, qty: Number(l.qty) })) };
    if (await act.run(() => api.createTransfer(body))) onDone();
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, sm: 560 }, maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-md">New stock transfer</Typography>
        <ErrorBar error={act.error} />
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1 }}>
          <FormControl>
            <FormLabel>From</FormLabel>
            <Select value={from} onChange={(_, v) => setFrom(v ?? "")} data-testid="tr-from">
              {locations.map((l) => <Option key={l.id} value={l.id}>{l.path}</Option>)}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>To</FormLabel>
            <Select value={to} onChange={(_, v) => setTo(v ?? "")} data-testid="tr-to">
              {locations.filter((l) => l.id !== from).map((l) => <Option key={l.id} value={l.id}>{l.path}</Option>)}
            </Select>
          </FormControl>
        </Box>
        {lines.map((l, i) => (
          <Box key={i} sx={{ display: "flex", gap: 1, alignItems: "center" }}>
            <Box sx={{ flex: 1 }}>
              <ProductPicker value={l.product_id} testId={`tr-line-product-${i}`} onChange={(id) => setLines((ls) => ls.map((x, j) => (j === i ? { ...x, product_id: id } : x)))} />
            </Box>
            <Input size="sm" type="number" sx={{ width: 90 }} value={l.qty} onChange={(e) => setLines((ls) => ls.map((x, j) => (j === i ? { ...x, qty: e.target.value } : x)))} slotProps={{ input: { min: 0, step: "any", "data-testid": `tr-line-qty-${i}` } }} />
            <IconButton size="sm" variant="plain" aria-label="Remove line" onClick={() => setLines((ls) => ls.filter((_, j) => j !== i))} disabled={lines.length === 1}>✕</IconButton>
          </Box>
        ))}
        <Button size="sm" variant="soft" onClick={() => setLines((ls) => [...ls, { product_id: "", qty: "1" }])}>Add product</Button>
        <FormControl>
          <FormLabel>Note</FormLabel>
          <Input value={note} onChange={(e) => setNote(e.target.value)} />
        </FormControl>
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button variant="outlined" loading={act.busy} onClick={() => save(false)} data-testid="tr-save-draft">Save draft</Button>
          <Button loading={act.busy} onClick={() => save(true)} data-testid="tr-save-ship">Save and ship</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

function ReceivePart({ t, onClose, onDone }: { t: Transfer; onClose: () => void; onDone: () => void }) {
  const [qty, setQty] = useState<Record<string, string>>({});
  const act = useAction();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340 }}>
        <ModalClose />
        <Typography level="title-md">Receive on {t.number}</Typography>
        <ErrorBar error={act.error} />
        {t.lines.map((l) => {
          const open = Math.round((l.qty_shipped - l.qty_received) * 1000) / 1000;
          return (
            <FormControl key={l.id}>
              <FormLabel>{l.product_name} (in transit {qtyText(open)})</FormLabel>
              <Input type="number" value={qty[l.id] ?? ""} placeholder="0" onChange={(e) => setQty((q) => ({ ...q, [l.id]: e.target.value }))} slotProps={{ input: { min: 0, max: open, step: "any", "data-testid": `tr-recv-qty-${l.sku || l.product_id}` } }} />
            </FormControl>
          );
        })}
        <Button
          loading={act.busy}
          onClick={async () => {
            const lines = t.lines.map((l) => ({ line_id: l.id, qty: Number(qty[l.id] ?? 0) })).filter((l) => l.qty > 0);
            if (lines.length === 0) { act.setError("Enter a quantity for at least one line"); return; }
            if (await act.run(() => api.receiveTransfer(t.id, lines))) onDone();
          }}
          data-testid="tr-recv-save"
        >
          Receive
        </Button>
      </ModalDialog>
    </Modal>
  );
}
