import { useState } from "react";
import { Box, Button, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Switch, Table, Tab, TabList, Tabs, Typography } from "@mui/joy";
import { api } from "./stockApi";
import type { Lot, Recall, Serial, TraceRow } from "./stockApi";
import { qtyText } from "./logic";
import { expiryText, expiryTone } from "./stockLogic";
import { Empty, ErrorBar, Loading, Title, dateOnly, dateTime, right, useAction, useLoad } from "./stockUi";

export function LotsView({ version, onChanged }: { version: number; onChanged: () => void }) {
  const [tab, setTab] = useState("lots");
  return (
    <Box>
      <Tabs value={tab} onChange={(_, v) => setTab(String(v))} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ mb: 2 }}>
          <Tab value="lots" data-testid="lots-tab-lots">Lots and expiry</Tab>
          <Tab value="serials" data-testid="lots-tab-serials">Serial numbers</Tab>
        </TabList>
      </Tabs>
      {tab === "lots" ? <LotList version={version} onChanged={onChanged} /> : <SerialList version={version} />}
    </Box>
  );
}

function LotList({ version, onChanged }: { version: number; onChanged: () => void }) {
  const [q, setQ] = useState("");
  const [window_, setWindow] = useState("0");
  const [onlyStock, setOnlyStock] = useState(true);
  const lots = useLoad(() => api.lots({ q, only_stock: onlyStock, expiring_days: Number(window_) }), [q, window_, onlyStock, version]);
  const [recall, setRecall] = useState<Lot | null>(null);
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
            <Input size="sm" placeholder="Search lot or product" value={q} onChange={(e) => setQ(e.target.value)} data-testid="lot-search" />
            <Select size="sm" value={window_} onChange={(_, v) => setWindow(v ?? "0")} sx={{ minWidth: 190 }} data-testid="lot-window">
              <Option value="0">All lots</Option>
              <Option value="30">Expiring within 30 days</Option>
              <Option value="90">Expiring within 90 days</Option>
            </Select>
            <FormControl orientation="horizontal" sx={{ gap: 0.75 }}>
              <Switch size="sm" checked={onlyStock} onChange={(e) => setOnlyStock(e.target.checked)} />
              <FormLabel>In stock only</FormLabel>
            </FormControl>
          </Box>
        }
      >
        Lots and batches
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Turn on lot tracking in a product&apos;s stock settings. Sales and transfers pick the earliest expiry first (FEFO) and never pick an expired or recalled lot.
      </Typography>
      <ErrorBar error={lots.error} />
      <Loading show={lots.loading && !lots.data} />
      {lots.data && lots.data.length === 0 && <Empty>No lots match.</Empty>}
      {lots.data && lots.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" stickyHeader data-testid="lot-table">
            <thead>
              <tr>
                <th>Product</th>
                <th>Lot</th>
                <th>Expiry</th>
                <th>Status</th>
                <th style={right}>On hand</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {lots.data.map((l) => (
                <tr key={l.id}>
                  <td>{l.product_name}</td>
                  <td>{l.lot_no}</td>
                  <td>{l.expiry ? dateOnly(l.expiry) : "—"}</td>
                  <td>
                    <Chip size="sm" variant="soft" color={expiryTone(l.days_left, l.recalled)}>
                      {l.recalled ? "Recalled" : expiryText(l.days_left)}
                    </Chip>
                  </td>
                  <td style={right}>{qtyText(l.on_hand)}</td>
                  <td style={right}>
                    <Button size="sm" variant="plain" onClick={() => setRecall(l)} data-testid={`lot-trace-${l.lot_no}`}>Where did it go</Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {recall && <RecallDialog lot={recall} onClose={() => setRecall(null)} onChanged={() => { onChanged(); void lots.reload(); }} />}
    </Box>
  );
}

function MovesTable({ rows }: { rows: TraceRow[] }) {
  if (rows.length === 0) return <Typography level="body-sm">None.</Typography>;
  return (
    <Table size="sm">
      <thead>
        <tr>
          <th>When</th>
          <th>What</th>
          <th>Document</th>
          <th>Party</th>
          <th>Where</th>
          <th style={right}>Qty</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((m, i) => (
          <tr key={i}>
            <td>{dateTime(m.at)}</td>
            <td>{m.reason}</td>
            <td>{m.ref}</td>
            <td>{m.party}</td>
            <td>{m.location}</td>
            <td style={right}>{m.qty > 0 ? "+" : ""}{qtyText(m.qty)}</td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
}

function RecallDialog({ lot, onClose, onChanged }: { lot: Lot; onClose: () => void; onChanged: () => void }) {
  const rc = useLoad<Recall>(() => api.recall(lot.id), [lot.id]);
  const act = useAction();
  const cur = rc.data?.lot ?? lot;
  const flag = async (recalled: boolean) => {
    const r = await act.run(() => api.updateLot(lot.id, { expiry: cur.expiry, mfg_date: cur.mfg_date, note: cur.note, recalled }));
    if (r) { onChanged(); void rc.reload(); }
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, md: 760 }, maxWidth: "95vw", maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">Lot {lot.lot_no} · {lot.product_name}</Typography>
        <ErrorBar error={rc.error || act.error} />
        <Loading show={rc.loading && !rc.data} />
        {rc.data && (
          <>
            <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
              <Chip variant="soft">Received {qtyText(rc.data.received)}</Chip>
              <Chip variant="soft">Gone out {qtyText(rc.data.shipped)}</Chip>
              <Chip variant="soft" color="success" data-testid="recall-onhand">On hand {qtyText(cur.on_hand)}</Chip>
              {cur.recalled && <Chip color="danger">Recalled: blocked from sales</Chip>}
            </Box>
            <Typography level="title-sm">Where the rest is</Typography>
            {cur.locations.length === 0 ? <Typography level="body-sm">Nothing left.</Typography> : cur.locations.map((l) => <Typography key={l.location_id} level="body-sm">{l.path}: {qtyText(l.qty)}</Typography>)}
            <Typography level="title-sm">Came in on</Typography>
            <MovesTable rows={rc.data.origins} />
            <Typography level="title-sm">Went out to</Typography>
            <Box data-testid="recall-outbound"><MovesTable rows={rc.data.outbound} /></Box>
            <Typography level="title-sm">Full history</Typography>
            <MovesTable rows={rc.data.moves} />
            <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
              {cur.recalled ? (
                <Button variant="outlined" onClick={() => flag(false)} loading={act.busy}>Clear recall</Button>
              ) : (
                <Button color="danger" variant="outlined" onClick={() => { if (window.confirm("Mark this lot as recalled? It can no longer be sold.")) void flag(true); }} loading={act.busy} data-testid="recall-flag">Mark as recalled</Button>
              )}
            </Box>
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}

function SerialList({ version }: { version: number }) {
  const [q, setQ] = useState("");
  const [state, setState] = useState("");
  const rows = useLoad(() => api.serials({ q, state }), [q, state, version]);
  const [hist, setHist] = useState<Serial | null>(null);
  const h = useLoad(() => (hist ? api.serialHistory(hist.id) : Promise.resolve([] as TraceRow[])), [hist?.id]);
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1 }}>
            <Input size="sm" placeholder="Search serial number" value={q} onChange={(e) => setQ(e.target.value)} data-testid="serial-search" />
            <Select size="sm" value={state} onChange={(_, v) => setState(v ?? "")} sx={{ minWidth: 130 }}>
              <Option value="">All</Option>
              <Option value="in_stock">In stock</Option>
              <Option value="out">Gone out</Option>
            </Select>
          </Box>
        }
      >
        Serial numbers
      </Title>
      <ErrorBar error={rows.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && rows.data.length === 0 && <Empty>No serial numbers.</Empty>}
      {rows.data && rows.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" stickyHeader data-testid="serial-table">
            <thead>
              <tr>
                <th>Serial</th>
                <th>Product</th>
                <th>State</th>
                <th>Where</th>
                <th>Out on</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.data.map((s) => (
                <tr key={s.id}>
                  <td>{s.serial_no}</td>
                  <td>{s.product_name}</td>
                  <td><Chip size="sm" variant="soft" color={s.state === "in_stock" ? "success" : "neutral"}>{s.state === "in_stock" ? "In stock" : "Gone out"}</Chip></td>
                  <td>{s.location}</td>
                  <td>{s.out_ref}</td>
                  <td style={right}><Button size="sm" variant="plain" onClick={() => setHist(s)}>History</Button></td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {hist && (
        <Modal open onClose={() => setHist(null)}>
          <ModalDialog sx={{ minWidth: { xs: 320, md: 640 } }}>
            <ModalClose />
            <Typography level="title-md">Serial {hist.serial_no}</Typography>
            <Loading show={h.loading} />
            <MovesTable rows={h.data ?? []} />
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}
