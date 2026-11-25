import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Option, Select, Sheet, Tab, TabList, Table, Tabs, Typography } from "@mui/joy";
import { deleteTaxRate, importGstr2b, listGstParties, listTaxRates, recordTds, saveGstParty, saveHsn, saveTaxRate, settleTds } from "./api2";
import type { GstParty, TaxRate } from "./api2";
import { fyStart, todayISO } from "./logic";
import type { Account, Settings } from "./types";
import { AccountSelect, ErrorAlert, isBank, rightCell, useErr } from "./ui";
import { ExportButtons, GenericReport } from "./ReportTable";

export function TaxView({ accounts, settings, version, onChanged, onOpenJournal }: { accounts: Account[]; settings: Settings; version: number; onChanged: () => void; onOpenJournal?: (id: string) => void }) {
  const [tab, setTab] = useState("gstr1");
  const [from, setFrom] = useState(fyStart(todayISO(), settings.fy_start_month));
  const [to, setTo] = useState(todayISO());
  const monthStart = () => {
    const t = todayISO();
    setFrom(t.slice(0, 8) + "01");
    setTo(t);
  };
  const q = { from, to };
  return (
    <Box>
      <Typography level="h3" sx={{ mb: 1 }}>
        Taxes
      </Typography>
      {!settings.gstin && (
        <Sheet variant="soft" color="warning" sx={{ p: 1, borderRadius: "md", mb: 1 }} data-testid="acct-tax-gstin-warning">
          Your GSTIN is not set. Add it under Settings so place of supply and CGST/SGST/IGST are worked out correctly.
        </Sheet>
      )}
      <Tabs value={tab} onChange={(_, v) => setTab(String(v))} sx={{ bgcolor: "transparent" }}>
        <TabList sx={{ flexWrap: "wrap" }}>
          <Tab value="gstr1" data-testid="acct-tax-gstr1">GSTR-1</Tab>
          <Tab value="gstr3b" data-testid="acct-tax-gstr3b">GSTR-3B</Tab>
          <Tab value="hsn-summary" data-testid="acct-tax-hsn">HSN summary</Tab>
          <Tab value="tds" data-testid="acct-tax-tds">TDS</Tab>
          <Tab value="itc" data-testid="acct-tax-itc">ITC reconciliation</Tab>
          <Tab value="setup" data-testid="acct-tax-setup">Rates and parties</Tab>
        </TabList>
      </Tabs>
      <Box sx={{ mt: 1.5 }}>
        {["gstr1", "gstr3b", "hsn-summary"].includes(tab) && (
          <>
            <Box className="no-print" sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 1 }}>
              <Input size="sm" type="date" value={from} onChange={(e) => setFrom(e.target.value)} aria-label="From date" />
              <Input size="sm" type="date" value={to} onChange={(e) => setTo(e.target.value)} aria-label="To date" />
              <Button size="sm" variant="plain" onClick={monthStart}>
                This month
              </Button>
              <ExportButtons name={tab} q={q} />
              <Button size="sm" variant="soft" component="a" href={`/api/v1/accounting/reports/${tab}?from=${from}&to=${to}`} target="_blank">
                JSON
              </Button>
            </Box>
            <GenericReport name={tab} q={q} version={version} drillFrom={from} drillTo={to} onOpenJournal={onOpenJournal} />
          </>
        )}
        {tab === "tds" && <TdsPanel accounts={accounts} from={from} to={to} setFrom={setFrom} setTo={setTo} version={version} onChanged={onChanged} />}
        {tab === "itc" && <ItcPanel version={version} />}
        {tab === "setup" && <Setup version={version} onChanged={onChanged} />}
      </Box>
    </Box>
  );
}

function TdsPanel({ accounts, from, to, setFrom, setTo, version, onChanged }: { accounts: Account[]; from: string; to: string; setFrom: (s: string) => void; setTo: (s: string) => void; version: number; onChanged: () => void }) {
  const [rates, setRates] = useState<TaxRate[]>([]);
  const [f, setF] = useState({ direction: "payable", section: "194J", party_name: "", pan: "", date: todayISO(), base_amount: 0, rate: 10 });
  const [bump, setBump] = useState(0);
  const [settle, setSettle] = useState<{ id: string; challan: string; account: string } | null>(null);
  const [rows, setRows] = useState<{ id: string; status: string; direction: string; section: string; party: string; tds: number }[]>([]);
  const { err, run } = useErr();
  useEffect(() => {
    listTaxRates().then((r) => setRates(r.filter((x) => x.kind === "tds" && x.active))).catch(() => undefined);
  }, [version]);
  const loadRows = useCallback(async () => {
    // the generic table has no ids; read the raw report JSON for pending entries
    const r = await run(() => fetch(`/api/v1/accounting/reports/tds?from=${from}&to=${to}`, { credentials: "same-origin" }).then((x) => x.json()));
    if (r) setRows((r.rows ?? []).map((e: { id: string; status: string; direction: string; section: string; party_name: string; tds_amount: number }) => ({ id: e.id, status: e.status, direction: e.direction, section: e.section, party: e.party_name, tds: e.tds_amount })));
  }, [from, to, run]);
  useEffect(() => {
    loadRows();
  }, [loadRows, version, bump]);
  return (
    <Box>
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", mb: 1.5, display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end" }}>
        <FormControl>
          <FormLabel>Direction</FormLabel>
          <Select size="sm" value={f.direction} onChange={(_, v) => setF({ ...f, direction: String(v) })} slotProps={{ button: { "data-testid": "acct-tds-direction" } as never }}>
            <Option value="payable">We deducted (payable)</Option>
            <Option value="receivable">Customer deducted (receivable)</Option>
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Section</FormLabel>
          <Select
            size="sm"
            value={f.section}
            onChange={(_, v) => {
              const r = rates.find((x) => x.code === v);
              setF({ ...f, section: String(v), rate: r ? r.rate : f.rate });
            }}
            sx={{ minWidth: 110 }}
          >
            {rates.map((r) => (
              <Option key={r.id} value={r.code}>
                {r.code} ({r.rate}%)
              </Option>
            ))}
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Party</FormLabel>
          <Input size="sm" value={f.party_name} onChange={(e) => setF({ ...f, party_name: e.target.value })} slotProps={{ input: { "data-testid": "acct-tds-party" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>PAN</FormLabel>
          <Input size="sm" value={f.pan} onChange={(e) => setF({ ...f, pan: e.target.value.toUpperCase() })} sx={{ width: 120 }} />
        </FormControl>
        <FormControl>
          <FormLabel>Date</FormLabel>
          <Input size="sm" type="date" value={f.date} onChange={(e) => setF({ ...f, date: e.target.value })} />
        </FormControl>
        <FormControl>
          <FormLabel>Base amount</FormLabel>
          <Input size="sm" type="number" value={f.base_amount || ""} onChange={(e) => setF({ ...f, base_amount: Number(e.target.value) })} sx={{ width: 130 }} slotProps={{ input: { "data-testid": "acct-tds-base" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>Rate %</FormLabel>
          <Input size="sm" type="number" value={f.rate} onChange={(e) => setF({ ...f, rate: Number(e.target.value) })} sx={{ width: 80 }} />
        </FormControl>
        <Button
          size="sm"
          data-testid="acct-tds-save"
          onClick={async () => {
            if (await run(() => recordTds(f))) {
              setBump((b) => b + 1);
              onChanged();
            }
          }}
        >
          Record TDS
        </Button>
      </Sheet>
      <ErrorAlert err={err} />
      <Box className="no-print" sx={{ display: "flex", gap: 1, mb: 1, alignItems: "center" }}>
        <Input size="sm" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
        <Input size="sm" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        <ExportButtons name="tds" q={{ from, to }} />
      </Box>
      <GenericReport name="tds" q={{ from, to }} version={version + bump} />
      {rows.filter((r) => r.status === "pending").length > 0 && (
        <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", mt: 1.5 }}>
          <Typography level="title-sm" sx={{ mb: 1 }}>
            Pending entries
          </Typography>
          {rows
            .filter((r) => r.status === "pending")
            .map((r) => (
              <Box key={r.id} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 0.5 }}>
                <Chip size="sm" variant="soft">{r.direction}</Chip>
                <span>
                  {r.section} {r.party} {r.tds.toFixed(2)}
                </span>
                {settle?.id === r.id ? (
                  <>
                    <Input size="sm" placeholder="Challan / reference" value={settle.challan} onChange={(e) => setSettle({ ...settle, challan: e.target.value })} />
                    {r.direction === "payable" && <AccountSelect accounts={accounts} value={settle.account} onChange={(a) => setSettle({ ...settle, account: a })} filter={isBank} placeholder="Paid from" />}
                    <Button
                      size="sm"
                      onClick={async () => {
                        if (await run(() => settleTds(r.id, { challan: settle.challan, account_id: settle.account }))) {
                          setSettle(null);
                          setBump((b) => b + 1);
                          onChanged();
                        }
                      }}
                    >
                      {r.direction === "payable" ? "Mark deposited" : "Mark claimed"}
                    </Button>
                  </>
                ) : (
                  <Button size="sm" variant="soft" onClick={() => setSettle({ id: r.id, challan: "", account: "" })}>
                    {r.direction === "payable" ? "Deposit…" : "Claim…"}
                  </Button>
                )}
              </Box>
            ))}
        </Sheet>
      )}
    </Box>
  );
}

function ItcPanel({ version }: { version: number }) {
  const [period, setPeriod] = useState(todayISO().slice(0, 7));
  const [csv, setCsv] = useState("");
  const [msg, setMsg] = useState("");
  const [bump, setBump] = useState(0);
  const { err, run } = useErr();
  return (
    <Box>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        Paste or upload your GSTR-2B supplier data as CSV with the columns gstin, supplier, invoice_no, invoice_date (YYYY-MM-DD), taxable, igst, cgst, sgst. Bills in Adrient are matched on supplier GSTIN and invoice number; set a supplier’s GSTIN under Rates and parties.
      </Typography>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 1 }}>
        <Input size="sm" type="month" value={period} onChange={(e) => setPeriod(e.target.value)} slotProps={{ input: { "data-testid": "acct-itc-period" } }} />
        <Button size="sm" variant="soft" component="label">
          Choose CSV file
          <input
            type="file"
            accept=".csv,text/csv"
            hidden
            data-testid="acct-itc-file"
            onChange={async (e) => {
              const f = e.target.files?.[0];
              if (f) setCsv(await f.text());
            }}
          />
        </Button>
        <Button
          size="sm"
          disabled={!csv}
          data-testid="acct-itc-import"
          onClick={async () => {
            const r = await run(() => importGstr2b(period, csv));
            if (r) {
              setMsg(`Imported ${r.imported} GSTR-2B rows for ${period}.`);
              setBump((b) => b + 1);
            }
          }}
        >
          Import GSTR-2B
        </Button>
        <ExportButtons name="itc-reconciliation" q={{ period }} />
      </Box>
      <Input placeholder="or paste CSV here" value={csv} onChange={(e) => setCsv(e.target.value)} sx={{ mb: 1 }} />
      <ErrorAlert err={err} />
      {msg && <Typography level="body-sm" color="success" sx={{ mb: 1 }}>{msg}</Typography>}
      <GenericReport name="itc-reconciliation" q={{ period }} version={version + bump} />
    </Box>
  );
}

function Setup({ version, onChanged }: { version: number; onChanged: () => void }) {
  const [rates, setRates] = useState<TaxRate[]>([]);
  const [parties, setParties] = useState<GstParty[]>([]);
  const [nr, setNr] = useState<Partial<TaxRate>>({ kind: "gst", code: "", name: "", rate: 0, active: true });
  const [np, setNp] = useState<Partial<GstParty>>({ name: "", gstin: "", pan: "" });
  const [hsn, setHsn] = useState({ match: "", hsn: "" });
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const [r, p] = (await run(() => Promise.all([listTaxRates(), listGstParties()]))) ?? [[], []];
    setRates(r);
    setParties(p);
  }, [run]);
  useEffect(() => {
    load();
  }, [load, version]);
  return (
    <Box sx={{ display: "grid", gap: 2 }}>
      <ErrorAlert err={err} />
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md" }}>
        <Typography level="title-md" sx={{ mb: 1 }}>
          Tax rates and codes
        </Typography>
        <Table size="sm" data-testid="acct-taxrates">
          <thead>
            <tr>
              <th>Kind</th>
              <th>Code</th>
              <th>Name</th>
              <th style={rightCell}>Rate %</th>
              <th>Active</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {rates.map((r) => (
              <tr key={r.id}>
                <td>{r.kind.toUpperCase()}</td>
                <td>{r.code}</td>
                <td>{r.name}</td>
                <td style={rightCell}>{r.rate}</td>
                <td>
                  <Checkbox size="sm" checked={r.active} onChange={async (e) => { await run(() => saveTaxRate({ ...r, active: e.target.checked })); load(); }} slotProps={{ input: { "aria-label": `Active ${r.code}` } }} />
                </td>
                <td>
                  <Button size="sm" variant="plain" color="danger" onClick={async () => { await run(() => deleteTaxRate(r.id)); load(); }}>
                    Delete
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </Table>
        <Box sx={{ display: "flex", gap: 1, mt: 1, flexWrap: "wrap" }}>
          <Select size="sm" value={nr.kind} onChange={(_, v) => setNr({ ...nr, kind: v as "gst" | "tds" })}>
            <Option value="gst">GST</Option>
            <Option value="tds">TDS</Option>
          </Select>
          <Input size="sm" placeholder="Code" value={nr.code} onChange={(e) => setNr({ ...nr, code: e.target.value })} sx={{ width: 100 }} slotProps={{ input: { "data-testid": "acct-taxrate-code" } }} />
          <Input size="sm" placeholder="Name" value={nr.name} onChange={(e) => setNr({ ...nr, name: e.target.value })} />
          <Input size="sm" type="number" placeholder="Rate %" value={nr.rate ?? 0} onChange={(e) => setNr({ ...nr, rate: Number(e.target.value) })} sx={{ width: 90 }} />
          <Button
            size="sm"
            onClick={async () => {
              if (await run(() => saveTaxRate({ ...nr, section: nr.kind === "tds" ? nr.code : "" }))) {
                setNr({ ...nr, code: "", name: "", rate: 0 });
                load();
              }
            }}
          >
            Add rate
          </Button>
        </Box>
      </Sheet>
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md" }}>
        <Typography level="title-md" sx={{ mb: 0.5 }}>
          Customer and vendor GST details
        </Typography>
        <Typography level="body-sm" sx={{ mb: 1 }}>
          The GSTIN decides B2B or B2C in GSTR-1 and, with the first two digits as the state, whether tax is CGST+SGST or IGST.
        </Typography>
        {parties.length > 0 && (
          <Table size="sm" data-testid="acct-parties">
            <tbody>
              {parties.map((p) => (
                <tr key={p.contact_id}>
                  <td>{p.name}</td>
                  <td>{p.gstin}</td>
                  <td>{p.state_code}</td>
                  <td>{p.pan}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
        <Box sx={{ display: "flex", gap: 1, mt: 1, flexWrap: "wrap" }}>
          <Input size="sm" placeholder="Name (as on invoices and bills)" value={np.name} onChange={(e) => setNp({ ...np, name: e.target.value })} slotProps={{ input: { "data-testid": "acct-party-name" } }} />
          <Input size="sm" placeholder="GSTIN" value={np.gstin} onChange={(e) => setNp({ ...np, gstin: e.target.value.toUpperCase() })} slotProps={{ input: { "data-testid": "acct-party-gstin" } }} />
          <Input size="sm" placeholder="PAN" value={np.pan} onChange={(e) => setNp({ ...np, pan: e.target.value.toUpperCase() })} sx={{ width: 120 }} />
          <Button
            size="sm"
            data-testid="acct-party-save"
            onClick={async () => {
              if (await run(() => saveGstParty(np))) {
                setNp({ name: "", gstin: "", pan: "" });
                load();
                onChanged();
              }
            }}
          >
            Save
          </Button>
        </Box>
      </Sheet>
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md" }}>
        <Typography level="title-md" sx={{ mb: 0.5 }}>
          HSN / SAC codes by item
        </Typography>
        <Typography level="body-sm" sx={{ mb: 1 }}>
          Invoice lines can carry an HSN code. For lines that do not, map the item description to a code here.
        </Typography>
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap" }}>
          <Input size="sm" placeholder="Item description" value={hsn.match} onChange={(e) => setHsn({ ...hsn, match: e.target.value })} />
          <Input size="sm" placeholder="HSN / SAC" value={hsn.hsn} onChange={(e) => setHsn({ ...hsn, hsn: e.target.value })} sx={{ width: 130 }} />
          <Button
            size="sm"
            onClick={async () => {
              if (await run(() => saveHsn(hsn.match, hsn.hsn))) {
                setHsn({ match: "", hsn: "" });
                onChanged();
              }
            }}
          >
            Save mapping
          </Button>
        </Box>
      </Sheet>
    </Box>
  );
}
