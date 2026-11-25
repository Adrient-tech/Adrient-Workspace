import { useState } from "react";
import { Alert, Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Table, Tab, TabList, Tabs, Typography } from "@mui/joy";
import { api } from "./stockApi";
import type { Count } from "./stockApi";
import { money, qtyText } from "./logic";
import { countVariance, daysCoverText } from "./stockLogic";
import { Margins, Shrinkage } from "./Shrinkage";
import { Empty, ErrorBar, Loading, Title, dateOnly, dateTime, right, useAction, useLoad } from "./stockUi";

export function CountsView({ version, onChanged }: { version: number; onChanged: () => void }) {
  const list = useLoad(() => api.counts(), [version]);
  const locs = useLoad(() => api.locations(), [version]);
  const [creating, setCreating] = useState(false);
  const [open, setOpen] = useState("");
  const reload = () => { onChanged(); void list.reload(); };
  return (
    <Box>
      <Title right={<Button size="sm" onClick={() => setCreating(true)} data-testid="count-new">Start a count</Button>}>Stock counts</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        A cycle count checks part of the stock (a category, an ABC class) and a full count checks everything at a location. Quantities counted are reviewed
        with their value before they change stock, and approval books the difference as an adjustment with your reason.
      </Typography>
      <ErrorBar error={list.error} />
      <Loading show={list.loading && !list.data} />
      {list.data && list.data.length === 0 && <Empty>No counts yet.</Empty>}
      {list.data && list.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow data-testid="count-table">
            <thead><tr><th>Number</th><th>Type</th><th>Location</th><th>Progress</th><th>Status</th><th style={right}>Net variance</th><th>Started</th></tr></thead>
            <tbody>
              {list.data.map((c) => (
                <tr key={c.id} onClick={() => setOpen(c.id)} style={{ cursor: "pointer" }} data-testid={`count-row-${c.number}`}>
                  <td>{c.number}</td>
                  <td>{c.kind === "full" ? "Full" : "Cycle"}{c.category ? ` (${c.category})` : ""}</td>
                  <td>{c.location}</td>
                  <td>{c.counted} of {c.total}</td>
                  <td><Chip size="sm" variant="soft" color={c.state === "approved" ? "success" : c.state === "review" ? "warning" : c.state === "cancelled" ? "danger" : "primary"}>{c.state}</Chip></td>
                  <td style={right}>{money(c.net_variance_value)}</td>
                  <td>{dateOnly(c.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {creating && <StartDialog locations={locs.data ?? []} onClose={() => setCreating(false)} onDone={(id) => { setCreating(false); reload(); setOpen(id); }} />}
      {open && <CountDialog id={open} onClose={() => setOpen("")} onChanged={reload} />}
    </Box>
  );
}

function StartDialog({ locations, onClose, onDone }: { locations: import("./stockApi").Loc[]; onClose: () => void; onDone: (id: string) => void }) {
  const [kind, setKind] = useState("cycle");
  const [loc, setLoc] = useState("");
  const [abc, setAbc] = useState("");
  const [category, setCategory] = useState("");
  const act = useAction();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: 340 }}>
        <ModalClose />
        <Typography level="title-md">Start a stock count</Typography>
        <ErrorBar error={act.error} />
        <FormControl><FormLabel>Type</FormLabel>
          <Select value={kind} onChange={(_, v) => setKind(v ?? "cycle")} data-testid="count-kind">
            <Option value="cycle">Cycle count (part of the stock)</Option>
            <Option value="full">Full count (everything at the location)</Option>
          </Select>
        </FormControl>
        <FormControl><FormLabel>Location (includes everything inside it)</FormLabel>
          <Select value={loc} onChange={(_, v) => setLoc(v ?? "")} data-testid="count-location">
            {locations.map((l) => <Option key={l.id} value={l.id}>{l.path}</Option>)}
          </Select>
        </FormControl>
        {kind === "cycle" && (
          <>
            <FormControl><FormLabel>Only ABC class (by last year&apos;s sales value)</FormLabel>
              <Select value={abc} onChange={(_, v) => setAbc(v ?? "")} data-testid="count-abc">
                <Option value="">All</Option>
                <Option value="A">A: top sellers</Option>
                <Option value="B">B</Option>
                <Option value="C">C: slow and unsold</Option>
              </Select>
            </FormControl>
            <FormControl><FormLabel>Only category</FormLabel><Input value={category} onChange={(e) => setCategory(e.target.value)} placeholder="Leave empty for all" /></FormControl>
          </>
        )}
        <Typography level="body-xs">Quantities expected are frozen when you start. Serial-numbered items are not counted here.</Typography>
        <Button loading={act.busy} onClick={async () => { const r = await act.run(() => api.startCount({ kind, location_id: loc, abc: abc || undefined, category: category || undefined })); if (r) onDone(r.id); }} data-testid="count-start">Start counting</Button>
      </ModalDialog>
    </Modal>
  );
}

function CountDialog({ id, onClose, onChanged }: { id: string; onClose: () => void; onChanged: () => void }) {
  const c = useLoad<Count>(() => api.count(id), [id]);
  const [vals, setVals] = useState<Record<string, string>>({});
  const [reason, setReason] = useState("");
  const [onlyDiff, setOnlyDiff] = useState(false);
  const act = useAction();
  const count = c.data;
  const editable = count && (count.state === "counting" || count.state === "review");
  const apply = async (fn: () => Promise<Count>) => {
    const r = await act.run(fn);
    if (r) { c.setData(r); setVals({}); onChanged(); }
    return r;
  };
  const saveEntries = async () => {
    const entries = Object.entries(vals).filter(([, v]) => v !== "").map(([line_id, v]) => ({ line_id, counted_qty: Number(v) }));
    if (entries.length === 0) return count;
    return apply(() => api.recordCount(id, entries));
  };
  const rows = (count?.lines ?? []).filter((l) => {
    if (!onlyDiff) return true;
    const v = vals[l.id];
    const counted = v !== undefined && v !== "" ? Number(v) : l.counted_qty;
    return counted !== null && counted !== l.expected_qty;
  });
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ minWidth: { xs: 320, md: 860 }, maxWidth: "97vw", maxHeight: "93vh", overflow: "auto" }}>
        <ModalClose />
        <Loading show={c.loading && !count} />
        <ErrorBar error={c.error || act.error} />
        {count && (
          <>
            <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }}>
              <Typography level="title-lg" data-testid="count-number">{count.number}</Typography>
              <Chip variant="soft" color={count.state === "approved" ? "success" : "primary"} data-testid="count-state">{count.state}</Chip>
              <Typography level="body-sm">{count.location} · {count.counted} of {count.total} counted</Typography>
              <Box sx={{ flex: 1 }} />
              <Checkbox size="sm" label="Show differences only" checked={onlyDiff} onChange={(e) => setOnlyDiff(e.target.checked)} />
            </Box>
            <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
              <Table size="sm" stickyHeader data-testid="count-lines">
                <thead>
                  <tr>
                    <th>Product</th><th>Shelf</th><th style={right}>Expected</th><th style={{ ...right, width: 110 }}>Counted</th>
                    <th style={right}>Difference</th><th style={right}>Value</th>{count.state === "review" && <th>Accept</th>}
                  </tr>
                </thead>
                <tbody>
                  {rows.map((l) => {
                    const typed = vals[l.id];
                    const counted = typed !== undefined && typed !== "" ? Number(typed) : l.counted_qty;
                    const v = countVariance(l.expected_qty, counted, l.unit_cost);
                    return (
                      <tr key={l.id}>
                        <td>{l.product_name} <Typography level="body-xs" component="span">{l.sku}</Typography></td>
                        <td>{l.shelf_code}</td>
                        <td style={right}>{qtyText(l.expected_qty)}</td>
                        <td style={right}>
                          {editable ? (
                            <Input size="sm" type="number" value={typed ?? (l.counted_qty === null ? "" : String(l.counted_qty))} onChange={(e) => setVals((x) => ({ ...x, [l.id]: e.target.value }))} slotProps={{ input: { min: 0, step: "any", "data-testid": `count-qty-${l.sku || l.product_id}` } }} />
                          ) : (
                            l.counted_qty === null ? "—" : qtyText(l.counted_qty)
                          )}
                        </td>
                        <td style={{ ...right, color: v.qty < 0 ? "var(--joy-palette-danger-600)" : v.qty > 0 ? "var(--joy-palette-success-600)" : undefined }}>{counted === null ? "" : qtyText(v.qty)}</td>
                        <td style={right}>{counted === null ? "" : money(v.value)}</td>
                        {count.state === "review" && (
                          <td><Checkbox size="sm" checked={l.accepted} disabled={l.counted_qty === l.expected_qty} onChange={(e) => void apply(() => api.recordCount(id, [{ line_id: l.id, accepted: e.target.checked }]))} aria-label={`Accept ${l.product_name}`} /></td>
                        )}
                      </tr>
                    );
                  })}
                </tbody>
              </Table>
            </Sheet>
            <Typography level="body-sm" sx={{ textAlign: "right" }} data-testid="count-net">Net difference in value (accepted lines): <b>{money(count.net_variance_value)}</b></Typography>
            {count.state === "counting" && (
              <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end", flexWrap: "wrap" }}>
                <Button variant="plain" color="danger" onClick={() => void apply(() => api.cancelCount(id))}>Cancel count</Button>
                <Button variant="outlined" loading={act.busy} onClick={saveEntries} data-testid="count-save">Save counts</Button>
                <Button loading={act.busy} onClick={async () => { if (Object.keys(vals).length > 0) await saveEntries(); await apply(() => api.submitCount(id)); }} data-testid="count-submit">Send for review</Button>
              </Box>
            )}
            {count.state === "review" && (
              <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end" }}>
                <FormControl sx={{ flex: 1, minWidth: 220 }}>
                  <FormLabel>Reason for the adjustments</FormLabel>
                  <Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="Annual count, damaged goods…" data-testid="count-reason" />
                </FormControl>
                <Button variant="plain" color="danger" onClick={() => void apply(() => api.cancelCount(id))}>Cancel count</Button>
                <Button loading={act.busy} onClick={() => void apply(() => api.approveCount(id, reason))} data-testid="count-approve">Approve and post adjustments</Button>
              </Box>
            )}
            {count.state === "approved" && <Alert color="success" variant="soft">Approved: {count.reason}. {dateTime(count.approved_at)}</Alert>}
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}

// -------------------------------------------------------------- planning

export function PlanningView({ version, onChanged }: { version: number; onChanged?: () => void }) {
  const [tab, setTab] = useState("cover");
  return (
    <Box>
      <Tabs value={tab} onChange={(_, v) => setTab(String(v))} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ mb: 2, flexWrap: "wrap" }}>
          <Tab value="cover" data-testid="plan-tab-cover">Days of cover and forecast</Tab>
          <Tab value="abc" data-testid="plan-tab-abc">ABC analysis</Tab>
          <Tab value="dead" data-testid="plan-tab-dead">Dead stock</Tab>
          <Tab value="shrink" data-testid="plan-tab-shrink">Shrinkage</Tab>
          <Tab value="margin" data-testid="plan-tab-margin">Margins</Tab>
        </TabList>
      </Tabs>
      {tab === "cover" && <Cover version={version} />}
      {tab === "abc" && <Abc version={version} />}
      {tab === "dead" && <Dead version={version} />}
      {tab === "shrink" && <Shrinkage version={version} onChanged={onChanged ?? (() => undefined)} />}
      {tab === "margin" && <Margins version={version} />}
    </Box>
  );
}

const STATUS_TONE = { out: "danger", low: "warning", ok: "success", overstock: "primary", no_demand: "neutral" } as const;
const STATUS_TEXT = { out: "Out", low: "Reorder soon", ok: "OK", overstock: "Overstocked", no_demand: "No sales" } as const;

function Cover({ version }: { version: number }) {
  const rows = useLoad(() => api.cover(8), [version]);
  const [only, setOnly] = useState(false);
  const list = (rows.data ?? []).filter((r) => !only || r.status === "low" || r.status === "out");
  return (
    <Box>
      <Title right={<Checkbox size="sm" label="Only items that need attention" checked={only} onChange={(e) => setOnly(e.target.checked)} />}>Days of cover</Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Demand is the average of the last 4 weeks of sales (a moving average). Days of cover is available stock divided by daily demand; it is flagged when it is shorter than the supplier lead time (or 7 days).
      </Typography>
      <ErrorBar error={rows.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && list.length === 0 && <Empty>Nothing to show.</Empty>}
      {list.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" stickyHeader data-testid="cover-table">
            <thead><tr><th>Product</th><th style={right}>Available</th><th style={right}>Incoming</th><th style={right}>Per day</th><th style={right}>Cover</th><th style={right}>Next 4 weeks</th><th>Last 8 weeks</th><th>Status</th></tr></thead>
            <tbody>
              {list.map((r) => (
                <tr key={r.product_id}>
                  <td>{r.name}</td>
                  <td style={right}>{qtyText(r.available)}</td>
                  <td style={right}>{qtyText(r.incoming)}</td>
                  <td style={right}>{qtyText(r.daily_demand)}</td>
                  <td style={right}>{daysCoverText(r.days_cover)}</td>
                  <td style={right}>{qtyText(r.forecast_4w)}</td>
                  <td><Spark values={r.weekly_sales} /></td>
                  <td><Chip size="sm" variant="soft" color={STATUS_TONE[r.status]}>{STATUS_TEXT[r.status]}</Chip></td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}

function Spark({ values }: { values: number[] }) {
  const max = Math.max(1, ...values);
  return (
    <svg width={values.length * 8} height={22} role="img" aria-label={`Weekly sales ${values.join(", ")}`}>
      {values.map((v, i) => (
        <rect key={i} x={i * 8} y={22 - (v / max) * 20 - 1} width={6} height={Math.max(1, (v / max) * 20)} fill="#126C5A" opacity={v > 0 ? 1 : 0.25} />
      ))}
    </svg>
  );
}

function Abc({ version }: { version: number }) {
  const [days, setDays] = useState("365");
  const rows = useLoad(() => api.abc(Number(days)), [days, version]);
  const counts = { A: 0, B: 0, C: 0 };
  (rows.data ?? []).forEach((r) => { counts[r.class]++; });
  return (
    <Box>
      <Title
        right={
          <Select size="sm" value={days} onChange={(_, v) => setDays(v ?? "365")} sx={{ minWidth: 150 }}>
            <Option value="90">Last 90 days</Option>
            <Option value="180">Last 6 months</Option>
            <Option value="365">Last 12 months</Option>
          </Select>
        }
      >
        ABC analysis
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Products ranked by the cost of goods sold. A covers the first 80% of value, B the next 15%, C the rest and everything that did not sell. Count A items most often.
      </Typography>
      <Box sx={{ display: "flex", gap: 1, mb: 1.5 }}>
        <Chip color="success" variant="soft">A: {counts.A} items</Chip><Chip color="warning" variant="soft">B: {counts.B}</Chip><Chip variant="soft">C: {counts.C}</Chip>
      </Box>
      <ErrorBar error={rows.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && rows.data.length === 0 && <Empty>No products.</Empty>}
      {rows.data && rows.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" stickyHeader data-testid="abc-table">
            <thead><tr><th>Class</th><th>Product</th><th style={right}>Units sold</th><th style={right}>Cost of sales</th><th style={right}>Share</th><th style={right}>Cumulative</th></tr></thead>
            <tbody>
              {rows.data.map((r) => (
                <tr key={r.product_id}>
                  <td><Chip size="sm" variant="soft" color={r.class === "A" ? "success" : r.class === "B" ? "warning" : "neutral"}>{r.class}</Chip></td>
                  <td>{r.name}</td><td style={right}>{qtyText(r.units)}</td><td style={right}>{money(r.value)}</td><td style={right}>{r.share_pct}%</td><td style={right}>{r.cum_pct}%</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}

function Dead({ version }: { version: number }) {
  const [days, setDays] = useState("90");
  const rows = useLoad(() => api.dead(Number(days)), [days, version]);
  const total = (rows.data ?? []).reduce((s, r) => s + r.value, 0);
  return (
    <Box>
      <Title
        right={
          <Select size="sm" value={days} onChange={(_, v) => setDays(v ?? "90")} sx={{ minWidth: 170 }}>
            <Option value="30">No sales in 30 days</Option>
            <Option value="90">No sales in 90 days</Option>
            <Option value="180">No sales in 6 months</Option>
            <Option value="365">No sales in a year</Option>
          </Select>
        }
      >
        Dead stock
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>Stock on the shelf that has not moved out for that long. The value is what it is carried at.</Typography>
      <ErrorBar error={rows.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && rows.data.length === 0 && <Empty>No dead stock for this period.</Empty>}
      {rows.data && rows.data.length > 0 && (
        <>
          <Alert color="warning" variant="soft" sx={{ mb: 1.5 }} data-testid="dead-total">{rows.data.length} items worth {money(total)} have not moved.</Alert>
          <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
            <Table size="sm" stickyHeader data-testid="dead-table">
              <thead><tr><th>Product</th><th style={right}>On hand</th><th style={right}>Value</th><th>Last out</th><th style={right}>Days idle</th></tr></thead>
              <tbody>
                {rows.data.map((r) => (
                  <tr key={r.product_id}><td>{r.name}</td><td style={right}>{qtyText(r.on_hand)}</td><td style={right}>{money(r.value)}</td><td>{r.last_out ? dateOnly(r.last_out) : "Never"}</td><td style={right}>{r.days_idle}</td></tr>
                ))}
              </tbody>
            </Table>
          </Sheet>
        </>
      )}
    </Box>
  );
}
