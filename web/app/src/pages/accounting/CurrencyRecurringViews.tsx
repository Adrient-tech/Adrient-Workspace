import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Modal, ModalDialog, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { assetSchedule, createAsset, deleteRate, deleteRecurring, disposeAsset, fetchRates, listAssets, listRates, listRecurring, revalue, runDepreciation, runRecurring, saveRate, saveRecurring } from "./api2";
import type { Asset, DepRow, FxRate, Recurring, RevalEntry } from "./api2";
import { money, todayISO } from "./logic";
import type { Account, Settings } from "./types";
import { AccountSelect, Empty, ErrorAlert, isBank, rightCell, useErr } from "./ui";

export function CurrenciesView({ settings, version, onChanged }: { settings: Settings; version: number; onChanged: () => void }) {
  const [rates, setRates] = useState<FxRate[]>([]);
  const [f, setF] = useState({ currency: "USD", date: todayISO(), rate: 0 });
  const [revalDate, setRevalDate] = useState(todayISO());
  const [reval, setReval] = useState<RevalEntry[] | null>(null);
  const [note, setNote] = useState("");
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => listRates());
    if (r) setRates(r);
  }, [run]);
  useEffect(() => {
    load();
  }, [load, version]);
  return (
    <Box>
      <Typography level="h3" sx={{ mb: 0.5 }}>
        Currencies
      </Typography>
      <Typography level="body-sm" sx={{ mb: 1.5 }}>
        Base currency is {settings.currency}. A rate says how many {settings.currency} one unit of the foreign currency is worth on a date. The latest rate on or before a document date is used. Payments in a foreign currency book the realised gain or loss against the rate the invoice or bill was booked at.
      </Typography>
      <ErrorAlert err={err} />
      {note && (
        <Sheet variant="soft" color="success" sx={{ p: 1, borderRadius: "md", mb: 1 }}>
          {note}
        </Sheet>
      )}
      <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", mb: 2, display: "flex", gap: 1, flexWrap: "wrap", alignItems: "flex-end" }}>
        <FormControl>
          <FormLabel>Currency</FormLabel>
          <Input size="sm" value={f.currency} onChange={(e) => setF({ ...f, currency: e.target.value.toUpperCase() })} sx={{ width: 90 }} slotProps={{ input: { "data-testid": "acct-rate-currency" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>Date</FormLabel>
          <Input size="sm" type="date" value={f.date} onChange={(e) => setF({ ...f, date: e.target.value })} slotProps={{ input: { "data-testid": "acct-rate-date" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>Rate ({settings.currency} per 1)</FormLabel>
          <Input size="sm" type="number" value={f.rate || ""} onChange={(e) => setF({ ...f, rate: Number(e.target.value) })} sx={{ width: 140 }} slotProps={{ input: { "data-testid": "acct-rate-value" } }} />
        </FormControl>
        <Button
          size="sm"
          data-testid="acct-rate-save"
          onClick={async () => {
            if (await run(() => saveRate(f))) {
              load();
              onChanged();
            }
          }}
        >
          Save rate
        </Button>
        <Button
          size="sm"
          variant="soft"
          onClick={async () => {
            const r = await run(() => fetchRates());
            if (r) {
              setNote(`Fetched ${r.stored} new rates for today (currencies already in use).`);
              load();
            }
          }}
        >
          Fetch today’s rates online
        </Button>
      </Sheet>
      {rates.length === 0 ? (
        <Empty>No exchange rates yet. Add one before posting foreign-currency documents.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto", maxHeight: 300, mb: 2 }}>
          <Table size="sm" data-testid="acct-rates">
            <thead>
              <tr>
                <th>Date</th>
                <th>Currency</th>
                <th style={rightCell}>Rate</th>
                <th>Source</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rates.map((r) => (
                <tr key={r.id}>
                  <td>{r.date}</td>
                  <td>{r.currency}</td>
                  <td style={rightCell}>{r.rate}</td>
                  <td>{r.source}</td>
                  <td>
                    <Button size="sm" variant="plain" color="danger" onClick={async () => { await run(() => deleteRate(r.id)); load(); }}>
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      <Typography level="title-md" sx={{ mb: 0.5 }}>
        Revalue open foreign balances
      </Typography>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        Posts an unrealised gain or loss on open foreign receivables and payables at the rate on this date, and reverses it the next day so the realised result stays correct. Running it again for the same date replaces the entry.
      </Typography>
      <Box sx={{ display: "flex", gap: 1, mb: 1 }}>
        <Input size="sm" type="date" value={revalDate} onChange={(e) => setRevalDate(e.target.value)} />
        <Button
          size="sm"
          data-testid="acct-reval"
          onClick={async () => {
            const r = await run(() => revalue(revalDate));
            if (r) {
              setReval(r.entries);
              onChanged();
            }
          }}
        >
          Revalue
        </Button>
      </Box>
      {reval && (reval.length === 0 ? <Typography level="body-sm">No open foreign-currency balances.</Typography> : (
        <Table size="sm" data-testid="acct-reval-table">
          <thead>
            <tr>
              <th>Currency</th>
              <th>Account</th>
              <th style={rightCell}>Foreign balance</th>
              <th style={rightCell}>Carrying ({settings.currency})</th>
              <th style={rightCell}>Revalued</th>
              <th style={rightCell}>Adjustment</th>
            </tr>
          </thead>
          <tbody>
            {reval.map((e, i) => (
              <tr key={i}>
                <td>
                  {e.currency} @ {e.rate}
                </td>
                <td>{e.account === "ar" ? "Receivable" : "Payable"}</td>
                <td style={rightCell}>{e.foreign_balance.toFixed(2)}</td>
                <td style={rightCell}>{money(e.carrying)}</td>
                <td style={rightCell}>{money(e.revalued)}</td>
                <td style={rightCell}>{money(e.adjustment)}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      ))}
    </Box>
  );
}

const FREQ = ["daily", "weekly", "monthly", "quarterly", "yearly"];

export function RecurringView({ accounts, version, onChanged }: { accounts: Account[]; version: number; onChanged: () => void }) {
  const [rows, setRows] = useState<Recurring[] | null>(null);
  const [show, setShow] = useState(false);
  const [msg, setMsg] = useState("");
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => listRecurring());
    if (r) setRows(r);
  }, [run]);
  useEffect(() => {
    load();
  }, [load, version]);
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", mb: 1 }}>
        <Typography level="h3" sx={{ flex: 1 }}>
          Recurring transactions
        </Typography>
        <Button
          size="sm"
          variant="soft"
          data-testid="acct-recurring-run"
          onClick={async () => {
            const r = await run(() => runRecurring());
            if (r) {
              setMsg(`${r.created} created, ${r.failed} failed${r.errors.length ? ": " + r.errors.join("; ") : ""}`);
              load();
              onChanged();
            }
          }}
        >
          Run due now
        </Button>
        <Button size="sm" onClick={() => setShow(true)} data-testid="acct-new-recurring">
          New schedule
        </Button>
      </Box>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        Schedules create journals, bills or expenses automatically when the server starts and every few hours after. Each date is created once, even if the runner runs twice or the server restarts; a failed one is retried.
      </Typography>
      <ErrorAlert err={err} />
      {msg && (
        <Sheet variant="soft" sx={{ p: 1, borderRadius: "md", mb: 1 }} data-testid="acct-recurring-msg">
          {msg}
        </Sheet>
      )}
      {rows && rows.length === 0 && <Empty>No recurring transactions yet.</Empty>}
      {rows && rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="acct-recurring-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Type</th>
                <th>Repeats</th>
                <th>Created</th>
                <th>Next</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id}>
                  <td>
                    {r.name} {!r.active && <Chip size="sm">paused</Chip>} {r.last_error && <Chip size="sm" color="danger" title={r.last_error}>failed run</Chip>}
                  </td>
                  <td>{r.kind}</td>
                  <td>
                    every {r.every > 1 ? r.every + " " : ""}
                    {r.frequency}
                  </td>
                  <td>{r.runs}</td>
                  <td>{r.next_run || "—"}</td>
                  <td>
                    <Button size="sm" variant="plain" onClick={async () => { await run(() => saveRecurring({ ...r, active: !r.active })); load(); }}>
                      {r.active ? "Pause" : "Resume"}
                    </Button>
                    <Button size="sm" variant="plain" color="danger" onClick={async () => { await run(() => deleteRecurring(r.id)); load(); }}>
                      Delete
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {show && <RecurringEditor accounts={accounts} onClose={() => setShow(false)} onSaved={() => { setShow(false); load(); }} />}
    </Box>
  );
}

function RecurringEditor({ accounts, onClose, onSaved }: { accounts: Account[]; onClose: () => void; onSaved: () => void }) {
  const [kind, setKind] = useState("journal");
  const [name, setName] = useState("");
  const [freq, setFreq] = useState("monthly");
  const [every, setEvery] = useState(1);
  const [start, setStart] = useState(todayISO());
  const [end, setEnd] = useState("");
  const [maxRuns, setMaxRuns] = useState(0);
  const [debit, setDebit] = useState("");
  const [credit, setCredit] = useState("");
  const [amount, setAmount] = useState(0);
  const [vendor, setVendor] = useState("");
  const [gst, setGst] = useState(18);
  const [autoPost, setAutoPost] = useState(true);
  const [paidFrom, setPaidFrom] = useState("");
  const { err, run } = useErr();
  const template = () => {
    if (kind === "journal")
      return { memo: name, lines: [{ account_id: debit, debit: amount, credit: 0, memo: name }, { account_id: credit, debit: 0, credit: amount, memo: name }] };
    if (kind === "bill") return { kind: "bill", vendor_name: vendor, lines: [{ account_id: debit, description: name, amount, tax_rate: gst }] };
    return { vendor_name: vendor, category_id: debit, amount, gst_rate: gst, gst_amount: Math.round(amount * gst) / 100, total: amount + (amount * gst) / 100, paid: !!paidFrom, paid_from_id: paidFrom, notes: name };
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 600, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }}>
        <Typography level="title-lg">New recurring transaction</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
          <FormControl>
            <FormLabel>Creates a</FormLabel>
            <Select value={kind} onChange={(_, v) => setKind(String(v))} slotProps={{ button: { "data-testid": "acct-rec-kind" } as never }}>
              <Option value="journal">Journal</Option>
              <Option value="bill">Bill</Option>
              <Option value="expense">Expense</Option>
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Name</FormLabel>
            <Input value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "data-testid": "acct-rec-name" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Repeats</FormLabel>
            <Box sx={{ display: "flex", gap: 1 }}>
              <Input type="number" value={every} onChange={(e) => setEvery(Number(e.target.value))} sx={{ width: 70 }} />
              <Select value={freq} onChange={(_, v) => setFreq(String(v))} sx={{ flex: 1 }}>
                {FREQ.map((f) => (
                  <Option key={f} value={f}>
                    {f}
                  </Option>
                ))}
              </Select>
            </Box>
          </FormControl>
          <FormControl>
            <FormLabel>First date</FormLabel>
            <Input type="date" value={start} onChange={(e) => setStart(e.target.value)} slotProps={{ input: { "data-testid": "acct-rec-start" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Ends on (optional)</FormLabel>
            <Input type="date" value={end} onChange={(e) => setEnd(e.target.value)} />
          </FormControl>
          <FormControl>
            <FormLabel>Max occurrences (0 = no limit)</FormLabel>
            <Input type="number" value={maxRuns} onChange={(e) => setMaxRuns(Number(e.target.value))} />
          </FormControl>
          <FormControl>
            <FormLabel>{kind === "journal" ? "Debit account" : kind === "bill" ? "Expense account" : "Category"}</FormLabel>
            <AccountSelect accounts={accounts} value={debit} onChange={setDebit} filter={kind === "journal" ? undefined : (a) => a.type === "expense"} testid="acct-rec-debit" />
          </FormControl>
          {kind === "journal" && (
            <FormControl>
              <FormLabel>Credit account</FormLabel>
              <AccountSelect accounts={accounts} value={credit} onChange={setCredit} testid="acct-rec-credit" />
            </FormControl>
          )}
          {kind !== "journal" && (
            <FormControl>
              <FormLabel>Vendor</FormLabel>
              <Input value={vendor} onChange={(e) => setVendor(e.target.value)} />
            </FormControl>
          )}
          <FormControl>
            <FormLabel>Amount {kind !== "journal" && "(before GST)"}</FormLabel>
            <Input type="number" value={amount || ""} onChange={(e) => setAmount(Number(e.target.value))} slotProps={{ input: { "data-testid": "acct-rec-amount" } }} />
          </FormControl>
          {kind !== "journal" && (
            <FormControl>
              <FormLabel>GST %</FormLabel>
              <Input type="number" value={gst} onChange={(e) => setGst(Number(e.target.value))} />
            </FormControl>
          )}
          {kind === "expense" && (
            <FormControl>
              <FormLabel>Paid from (blank = payable)</FormLabel>
              <AccountSelect accounts={accounts} value={paidFrom} onChange={setPaidFrom} filter={isBank} />
            </FormControl>
          )}
        </Box>
        {kind === "bill" && <Checkbox label="Post each bill automatically (otherwise create drafts)" checked={autoPost} onChange={(e) => setAutoPost(e.target.checked)} />}
        <ErrorAlert err={err} />
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button
            data-testid="acct-rec-save"
            onClick={async () => {
              if (await run(() => saveRecurring({ kind, name, frequency: freq, every, start_date: start, end_date: end, max_runs: maxRuns, active: true, auto_post: kind === "bill" ? autoPost : true, template: template() }))) onSaved();
            }}
          >
            Save schedule
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

export function AssetsView({ accounts, version, onChanged }: { accounts: Account[]; version: number; onChanged: () => void }) {
  const [rows, setRows] = useState<Asset[] | null>(null);
  const [show, setShow] = useState(false);
  const [sched, setSched] = useState<{ asset: Asset; rows: DepRow[] } | null>(null);
  const [msg, setMsg] = useState("");
  const [disp, setDisp] = useState<Asset | null>(null);
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => listAssets());
    if (r) setRows(r);
  }, [run]);
  useEffect(() => {
    load();
  }, [load, version]);
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", mb: 1 }}>
        <Typography level="h3" sx={{ flex: 1 }}>
          Fixed assets
        </Typography>
        <Button
          size="sm"
          variant="soft"
          data-testid="acct-dep-run"
          onClick={async () => {
            const r = await run(() => runDepreciation(todayISO().slice(0, 7)));
            if (r) {
              setMsg(`${r.posted} depreciation entries posted${r.locked ? `, ${r.locked} skipped in the closed period` : ""}.`);
              load();
              onChanged();
            }
          }}
        >
          Post depreciation to this month
        </Button>
        <Button size="sm" onClick={() => setShow(true)} data-testid="acct-new-asset">
          New asset
        </Button>
      </Box>
      <Typography level="body-sm" sx={{ mb: 1 }}>
        Depreciation posts monthly (Dr Depreciation Expense, Cr Accumulated Depreciation) automatically for every month that has fully passed, and can be posted up to the current month here. Each month is posted once.
      </Typography>
      <ErrorAlert err={err} />
      {msg && (
        <Sheet variant="soft" sx={{ p: 1, borderRadius: "md", mb: 1 }} data-testid="acct-dep-msg">
          {msg}
        </Sheet>
      )}
      {rows && rows.length === 0 && <Empty>No fixed assets yet.</Empty>}
      {rows && rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" data-testid="acct-assets-table">
            <thead>
              <tr>
                <th>Asset</th>
                <th>Bought</th>
                <th>Method</th>
                <th style={rightCell}>Cost</th>
                <th style={rightCell}>Accumulated</th>
                <th style={rightCell}>Book value</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.map((a) => (
                <tr key={a.id}>
                  <td>
                    {a.name} {a.tag && <Chip size="sm">{a.tag}</Chip>} {a.status === "disposed" && <Chip size="sm" color="neutral">disposed</Chip>}
                  </td>
                  <td>{a.purchase_date}</td>
                  <td>{a.method === "wdv" ? `WDV ${a.wdv_rate}%` : `SLM ${a.life_months} mo`}</td>
                  <td style={rightCell}>{money(a.cost)}</td>
                  <td style={rightCell}>{money(a.accumulated)}</td>
                  <td style={rightCell}>{money(a.book_value)}</td>
                  <td>
                    <Button size="sm" variant="plain" onClick={async () => { const r = await run(() => assetSchedule(a.id)); if (r) setSched({ asset: a, rows: r }); }}>
                      Schedule
                    </Button>
                    {a.status === "active" && (
                      <Button size="sm" variant="plain" color="danger" onClick={() => setDisp(a)}>
                        Dispose
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {show && <AssetEditor accounts={accounts} onClose={() => setShow(false)} onSaved={() => { setShow(false); load(); onChanged(); }} />}
      {disp && <DisposeDialog asset={disp} accounts={accounts} onClose={() => setDisp(null)} onDone={() => { setDisp(null); load(); onChanged(); }} />}
      {sched && (
        <Modal open onClose={() => setSched(null)}>
          <ModalDialog sx={{ width: 720, maxWidth: "96vw", maxHeight: "90vh", overflow: "auto" }}>
            <Typography level="title-lg">Depreciation schedule: {sched.asset.name}</Typography>
            <Table size="sm" data-testid="acct-dep-schedule">
              <thead>
                <tr>
                  <th>Month</th>
                  <th style={rightCell}>Opening</th>
                  <th style={rightCell}>Depreciation</th>
                  <th style={rightCell}>Accumulated</th>
                  <th style={rightCell}>Closing</th>
                  <th>Posted</th>
                </tr>
              </thead>
              <tbody>
                {sched.rows.map((r) => (
                  <tr key={r.period}>
                    <td>{r.period}</td>
                    <td style={rightCell}>{money(r.opening)}</td>
                    <td style={rightCell}>{money(r.depreciation)}</td>
                    <td style={rightCell}>{money(r.accumulated)}</td>
                    <td style={rightCell}>{money(r.closing)}</td>
                    <td>{r.posted ? "✓" : ""}</td>
                  </tr>
                ))}
              </tbody>
            </Table>
            <Button variant="plain" color="neutral" onClick={() => setSched(null)}>
              Close
            </Button>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

function AssetEditor({ accounts, onClose, onSaved }: { accounts: Account[]; onClose: () => void; onSaved: () => void }) {
  const [a, setA] = useState<Partial<Asset>>({ name: "", tag: "", purchase_date: todayISO(), cost: 0, salvage: 0, life_months: 60, method: "slm", wdv_rate: 15, class: "" });
  const [paidFrom, setPaidFrom] = useState("");
  const { err, run } = useErr();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 560, maxWidth: "96vw" }}>
        <Typography level="title-lg">New fixed asset</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
          <FormControl>
            <FormLabel>Name</FormLabel>
            <Input value={a.name} onChange={(e) => setA({ ...a, name: e.target.value })} slotProps={{ input: { "data-testid": "acct-asset-name" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Asset tag</FormLabel>
            <Input value={a.tag} onChange={(e) => setA({ ...a, tag: e.target.value })} />
          </FormControl>
          <FormControl>
            <FormLabel>Purchase date</FormLabel>
            <Input type="date" value={a.purchase_date} onChange={(e) => setA({ ...a, purchase_date: e.target.value })} slotProps={{ input: { "data-testid": "acct-asset-date" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Cost</FormLabel>
            <Input type="number" value={a.cost || ""} onChange={(e) => setA({ ...a, cost: Number(e.target.value) })} slotProps={{ input: { "data-testid": "acct-asset-cost" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Salvage value</FormLabel>
            <Input type="number" value={a.salvage || ""} onChange={(e) => setA({ ...a, salvage: Number(e.target.value) })} />
          </FormControl>
          <FormControl>
            <FormLabel>Useful life (months)</FormLabel>
            <Input type="number" value={a.life_months} onChange={(e) => setA({ ...a, life_months: Number(e.target.value) })} slotProps={{ input: { "data-testid": "acct-asset-life" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Method</FormLabel>
            <Select value={a.method} onChange={(_, v) => setA({ ...a, method: String(v) })}>
              <Option value="slm">Straight line (SLM)</Option>
              <Option value="wdv">Written down value (WDV)</Option>
            </Select>
          </FormControl>
          {a.method === "wdv" && (
            <FormControl>
              <FormLabel>WDV rate % per year</FormLabel>
              <Input type="number" value={a.wdv_rate} onChange={(e) => setA({ ...a, wdv_rate: Number(e.target.value) })} />
            </FormControl>
          )}
          <FormControl>
            <FormLabel>Class / cost center</FormLabel>
            <Input value={a.class} onChange={(e) => setA({ ...a, class: e.target.value })} />
          </FormControl>
          <FormControl>
            <FormLabel>Paid from (posts the purchase)</FormLabel>
            <AccountSelect accounts={accounts} value={paidFrom} onChange={setPaidFrom} filter={(x) => isBank(x) || x.subtype === "payable"} placeholder="Already in the books" />
          </FormControl>
        </Box>
        <ErrorAlert err={err} />
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button
            data-testid="acct-asset-save"
            onClick={async () => {
              if (await run(() => createAsset({ ...a, paid_from_id: paidFrom }))) onSaved();
            }}
          >
            Save asset
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

function DisposeDialog({ asset, accounts, onClose, onDone }: { asset: Asset; accounts: Account[]; onClose: () => void; onDone: () => void }) {
  const [date, setDate] = useState(todayISO());
  const [proceeds, setProceeds] = useState(0);
  const [acct, setAcct] = useState("");
  const { err, run } = useErr();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 460, maxWidth: "96vw" }}>
        <Typography level="title-lg">Dispose of {asset.name}</Typography>
        <Typography level="body-sm">Depreciation is brought up to the disposal month. Book value then is posted against the proceeds as a gain or loss.</Typography>
        <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
        <Input type="number" placeholder="Sale proceeds (0 if scrapped)" value={proceeds || ""} onChange={(e) => setProceeds(Number(e.target.value))} />
        {proceeds > 0 && <AccountSelect accounts={accounts} value={acct} onChange={setAcct} filter={isBank} placeholder="Proceeds received in" />}
        <ErrorAlert err={err} />
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button
            color="danger"
            onClick={async () => {
              if (await run(() => disposeAsset(asset.id, { date, proceeds, account_id: acct }))) onDone();
            }}
          >
            Dispose
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
