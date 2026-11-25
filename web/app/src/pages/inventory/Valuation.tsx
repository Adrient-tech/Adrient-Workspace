import { Fragment, useState } from "react";
import { Alert, Box, Button, Chip, Input, Option, Select, Sheet, Table, Tab, TabList, Tabs, Typography } from "@mui/joy";
import { api } from "./stockApi";
import { money, qtyText } from "./logic";
import { Empty, ErrorBar, Loading, Title, dateTime, right, useAction, useLoad } from "./stockUi";

export function ValuationView({ version }: { version: number }) {
  const [tab, setTab] = useState("value");
  return (
    <Box>
      <Tabs value={tab} onChange={(_, v) => setTab(String(v))} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ mb: 2 }}>
          <Tab value="value" data-testid="val-tab-value">Stock valuation</Tab>
          <Tab value="cogs" data-testid="val-tab-cogs">COGS and adjustments</Tab>
        </TabList>
      </Tabs>
      {tab === "value" ? <Report version={version} /> : <Ledger version={version} />}
    </Box>
  );
}

const today = () => new Date().toISOString().slice(0, 10);

function Report({ version }: { version: number }) {
  const [asOf, setAsOf] = useState(today());
  const rep = useLoad(() => api.valuation(asOf), [asOf, version]);
  const [open, setOpen] = useState("");
  const layers = useLoad(() => (open ? api.productLayers(open) : Promise.resolve([])), [open]);
  const csv = () => {
    if (!rep.data) return;
    const lines = [["Product", "SKU", "Category", "Method", "Qty", "Avg cost", "Value"], ...rep.data.rows.map((r) => [r.name, r.sku, r.category, r.method, r.qty, r.avg_cost, r.value])];
    const text = lines.map((l) => l.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(",")).join("\r\n");
    const a = document.createElement("a");
    a.href = URL.createObjectURL(new Blob([text], { type: "text/csv" }));
    a.download = `stock-valuation-${asOf}.csv`;
    a.click();
  };
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1, alignItems: "center" }}>
            <Typography level="body-sm">As of</Typography>
            <Input size="sm" type="date" value={asOf} onChange={(e) => setAsOf(e.target.value || today())} slotProps={{ input: { max: today(), "data-testid": "val-asof" } }} />
            <Button size="sm" variant="outlined" onClick={csv} disabled={!rep.data?.rows.length}>Export CSV</Button>
          </Box>
        }
      >
        Stock value
      </Title>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Value is built from receipts and movements: each product uses FIFO or weighted average (set per product), receipts carry landed costs, and returns
        come back at their cost. Pick a past date to see the value on that day.
      </Typography>
      <ErrorBar error={rep.error} />
      <Loading show={rep.loading && !rep.data} />
      {rep.data && (
        <>
          <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap", mb: 2 }}>
            <Sheet variant="soft" sx={{ p: 2, borderRadius: "md", minWidth: 180 }}>
              <Typography level="body-xs">Total value</Typography>
              <Typography level="h3" data-testid="val-total">{money(rep.data.total_value)}</Typography>
            </Sheet>
            <Sheet variant="soft" sx={{ p: 2, borderRadius: "md", minWidth: 140 }}>
              <Typography level="body-xs">Units</Typography>
              <Typography level="h3">{qtyText(rep.data.total_qty)}</Typography>
            </Sheet>
            {rep.data.in_transit_qty > 0 && (
              <Sheet variant="soft" sx={{ p: 2, borderRadius: "md", minWidth: 140 }}>
                <Typography level="body-xs">Of which in transit</Typography>
                <Typography level="h3">{qtyText(rep.data.in_transit_qty)}</Typography>
              </Sheet>
            )}
            {rep.data.by_category.map((c) => (
              <Sheet key={c.category} variant="outlined" sx={{ p: 2, borderRadius: "md" }}>
                <Typography level="body-xs">{c.category}</Typography>
                <Typography level="title-md">{money(c.value)}</Typography>
              </Sheet>
            ))}
          </Box>
          {rep.data.rows.length === 0 ? (
            <Empty>No stock value on this date.</Empty>
          ) : (
            <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
              <Table size="sm" stickyHeader data-testid="val-table">
                <thead>
                  <tr>
                    <th>Product</th>
                    <th>Method</th>
                    <th style={right}>Qty</th>
                    <th style={right}>Avg cost</th>
                    <th style={right}>Value</th>
                  </tr>
                </thead>
                <tbody>
                  {rep.data.rows.map((r) => (
                    <Fragment key={r.product_id}>
                      <tr onClick={() => setOpen(open === r.product_id ? "" : r.product_id)} style={{ cursor: "pointer" }} data-testid={`val-row-${r.sku || r.product_id}`}>
                        <td>{r.name} <Typography level="body-xs" component="span">{r.sku}</Typography></td>
                        <td>{r.method === "fifo" ? "FIFO" : "Average"}</td>
                        <td style={right}>{qtyText(r.qty)}</td>
                        <td style={right}>{money(r.avg_cost)}</td>
                        <td style={{ ...right, fontWeight: 600 }}>{money(r.value)}</td>
                      </tr>
                      {open === r.product_id && (
                        <tr>
                          <td colSpan={5} style={{ background: "var(--joy-palette-background-level1)" }}>
                            <Typography level="body-xs" sx={{ mb: 0.5 }}>Open cost layers (today)</Typography>
                            {(layers.data ?? []).length === 0 ? (
                              <Typography level="body-sm">No open layers.</Typography>
                            ) : (
                              <Table size="sm" borderAxis="none">
                                <tbody>
                                  {(layers.data ?? []).map((l) => (
                                    <tr key={l.id}>
                                      <td>{dateTime(l.received_at)}</td>
                                      <td>{l.source}{l.ref ? ` ${l.ref.slice(0, 8)}` : ""}</td>
                                      <td style={right}>{qtyText(l.qty_remaining)} of {qtyText(l.qty_in)}</td>
                                      <td style={right}>@ {money(l.unit_cost)}</td>
                                    </tr>
                                  ))}
                                </tbody>
                              </Table>
                            )}
                          </td>
                        </tr>
                      )}
                    </Fragment>
                  ))}
                </tbody>
              </Table>
            </Sheet>
          )}
        </>
      )}
    </Box>
  );
}

function Ledger({ version }: { version: number }) {
  const [kind, setKind] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const rows = useLoad(() => api.ledger({ kind, from, to }), [kind, from, to, version]);
  const status = useLoad(() => api.accountingStatus(), [version]);
  const act = useAction();
  const sync = async () => {
    const r = await act.run(() => api.accountingSync());
    if (r) { void status.reload(); void rows.reload(); }
  };
  return (
    <Box>
      <Title
        right={
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
            <Select size="sm" value={kind} onChange={(_, v) => setKind(v ?? "")} sx={{ minWidth: 170 }} placeholder="All entries">
              <Option value="">All entries</Option>
              <Option value="stock_cogs">Cost of goods sold</Option>
              <Option value="stock_adjustment">Stock adjustments</Option>
            </Select>
            <Input size="sm" type="date" value={from} onChange={(e) => setFrom(e.target.value)} slotProps={{ input: { "aria-label": "From date" } }} />
            <Input size="sm" type="date" value={to} onChange={(e) => setTo(e.target.value)} slotProps={{ input: { "aria-label": "To date" } }} />
          </Box>
        }
      >
        COGS and stock adjustments
      </Title>
      {status.data && (
        <Alert color={status.data.failed > 0 ? "warning" : "neutral"} variant="soft" sx={{ mb: 1.5 }} endDecorator={<Button size="sm" variant="outlined" loading={act.busy} onClick={sync} data-testid="acct-sync">Send to Accounting now</Button>}>
          {status.data.connected
            ? `Accounting: ${status.data.pending} waiting to be posted, ${status.data.failed} failed. Entries post automatically; a failure (for example a closed period) is retried.`
            : "Accounting is not connected on this server, so these entries stay here."}
        </Alert>
      )}
      <ErrorBar error={rows.error || act.error} />
      <Loading show={rows.loading && !rows.data} />
      {rows.data && rows.data.length === 0 && <Empty>No valued movements in this range.</Empty>}
      {rows.data && rows.data.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" stickyHeader data-testid="cogs-table">
            <thead>
              <tr>
                <th>When</th>
                <th>Product</th>
                <th>Reason</th>
                <th>Ref</th>
                <th style={right}>Qty</th>
                <th style={right}>Unit cost</th>
                <th style={right}>Value</th>
                <th>Accounting</th>
              </tr>
            </thead>
            <tbody>
              {rows.data.map((r) => (
                <tr key={r.id}>
                  <td>{dateTime(r.ts)}</td>
                  <td>{r.product_name}</td>
                  <td>{r.reason}</td>
                  <td>{r.ref}</td>
                  <td style={right}>{qtyText(r.qty)}</td>
                  <td style={right}>{money(r.unit_cost)}</td>
                  <td style={right}>{money(r.value)}</td>
                  <td>
                    {r.journal_kind === "" ? <Typography level="body-xs">not accounted here</Typography> : (
                      <Chip size="sm" variant="soft" color={r.journal_state === "posted" ? "success" : r.journal_state === "failed" ? "danger" : "warning"} title={r.journal_error}>
                        {r.journal_kind === "stock_cogs" ? "COGS" : "Adjustment"} · {r.journal_state}
                      </Chip>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}
