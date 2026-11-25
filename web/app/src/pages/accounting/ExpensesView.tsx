import { useCallback, useEffect, useRef, useState } from "react";
import {
  Box,
  Button,
  Checkbox,
  Chip,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Sheet,
  Table,
  Textarea,
  Typography,
} from "@mui/joy";
import DeleteOutline from "@mui/icons-material/DeleteOutline";
import AttachFile from "@mui/icons-material/AttachFile";
import {
  deleteExpense,
  deleteReport,
  listContacts,
  listExpenses,
  listReports,
  payBill,
  receiptURL,
  reportAction,
  saveExpense,
  saveReport,
  uploadReceipt,
} from "./api";
import { STATUS_COLOR, gstOf, money, todayISO } from "./logic";
import type { Account, ContactLite, Expense, ExpenseReport } from "./types";
import { AccountSelect, ContactInput, Empty, ErrorAlert, isBank, rightCell, useErr } from "./ui";

const GST_RATES = [0, 5, 12, 18, 28];

export function ExpensesView({ accounts, version, onChanged }: { accounts: Account[]; version: number; onChanged: () => void }) {
  const [tab, setTab] = useState<"expenses" | "reports">("expenses");
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", mb: 1.5 }}>
        <Typography level="h3" sx={{ mr: "auto" }}>
          Expenses
        </Typography>
        <Button size="sm" variant={tab === "expenses" ? "solid" : "plain"} onClick={() => setTab("expenses")} data-testid="acct-tab-expenses">
          Expenses and bills
        </Button>
        <Button size="sm" variant={tab === "reports" ? "solid" : "plain"} onClick={() => setTab("reports")} data-testid="acct-tab-reports">
          Expense reports
        </Button>
      </Box>
      {tab === "expenses" ? <ExpenseList accounts={accounts} version={version} onChanged={onChanged} /> : <ReportsList accounts={accounts} version={version} onChanged={onChanged} />}
    </Box>
  );
}

function ExpenseList({ accounts, version, onChanged }: { accounts: Account[]; version: number; onChanged: () => void }) {
  const [rows, setRows] = useState<Expense[] | null>(null);
  const [q, setQ] = useState("");
  const [edit, setEdit] = useState<Partial<Expense> | null>(null);
  const [pay, setPay] = useState<Expense | null>(null);
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => listExpenses({ q }));
    if (r) setRows(r);
  }, [q, run]);
  useEffect(() => {
    load();
  }, [load, version]);
  return (
    <>
      <Box sx={{ display: "flex", gap: 1, mb: 1.5 }}>
        <Input size="sm" placeholder="Search vendor, notes, category" value={q} onChange={(e) => setQ(e.target.value)} sx={{ flex: 1, maxWidth: 360 }} />
        <Button size="sm" sx={{ ml: "auto" }} onClick={() => setEdit({ date: todayISO(), gst_rate: 18, paid: true, receipts: [] })} data-testid="acct-new-expense">
          New expense
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {rows && rows.length === 0 && <Empty>No expenses yet. Add one with a receipt photo or PDF.</Empty>}
      {rows && rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow data-testid="acct-expense-table">
            <thead>
              <tr>
                <th style={{ width: 110 }}>Date</th>
                <th>Vendor</th>
                <th>Category</th>
                <th style={rightCell}>GST</th>
                <th style={rightCell}>Total</th>
                <th>Status</th>
                <th style={{ width: 90 }} />
              </tr>
            </thead>
            <tbody>
              {rows.map((e) => (
                <tr key={e.id} onClick={() => setEdit(e)} style={{ cursor: "pointer" }}>
                  <td>{e.date}</td>
                  <td>
                    {e.vendor_name || "—"} {e.receipts.length > 0 && <AttachFile fontSize="small" sx={{ verticalAlign: "middle" }} titleAccess="Has receipt" />}
                  </td>
                  <td>{e.category_name}</td>
                  <td style={rightCell}>{e.gst_amount ? money(e.gst_amount) : ""}</td>
                  <td style={rightCell}>{money(e.total)}</td>
                  <td>
                    {e.report_id ? (
                      <Chip size="sm" variant="soft" color={e.posted ? "success" : "warning"}>
                        {e.report_title}
                      </Chip>
                    ) : e.paid ? (
                      <Chip size="sm" variant="soft" color="success">
                        Paid
                      </Chip>
                    ) : (
                      <Chip size="sm" variant="soft" color="warning">
                        Bill unpaid
                      </Chip>
                    )}
                  </td>
                  <td onClick={(ev) => ev.stopPropagation()}>
                    {!e.paid && !e.out_of_pocket && (
                      <Button size="sm" variant="soft" onClick={() => setPay(e)} data-testid="acct-pay-bill">
                        Pay
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      {edit && (
        <ExpenseDialog
          expense={edit}
          accounts={accounts}
          onClose={() => setEdit(null)}
          onSaved={() => {
            setEdit(null);
            onChanged();
          }}
        />
      )}
      {pay && (
        <Modal open onClose={() => setPay(null)}>
          <PayDialog
            title={`Pay bill: ${pay.vendor_name || pay.category_name} (${money(pay.total)})`}
            accounts={accounts}
            onClose={() => setPay(null)}
            onPay={async (acct, date) => {
              const ok = await run(() => payBill(pay.id, acct, date));
              if (ok) {
                setPay(null);
                onChanged();
              }
              return !!ok;
            }}
          />
        </Modal>
      )}
    </>
  );
}

function PayDialog({ title, accounts, onClose, onPay }: { title: string; accounts: Account[]; onClose: () => void; onPay: (acct: string, date: string) => Promise<boolean> }) {
  const [acct, setAcct] = useState(accounts.find((a) => a.system_key === "bank")?.id ?? "");
  const [date, setDate] = useState(todayISO());
  return (
    <ModalDialog sx={{ width: "min(420px, 94vw)" }}>
      <ModalClose />
      <Typography level="h4">{title}</Typography>
      <FormControl>
        <FormLabel>Pay from</FormLabel>
        <AccountSelect accounts={accounts} value={acct} onChange={setAcct} filter={isBank} testid="acct-pay-from" />
      </FormControl>
      <FormControl>
        <FormLabel>Date</FormLabel>
        <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
      </FormControl>
      <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
        <Button variant="plain" color="neutral" onClick={onClose}>
          Cancel
        </Button>
        <Button disabled={!acct} onClick={() => onPay(acct, date)} data-testid="acct-pay-confirm">
          Pay
        </Button>
      </Box>
    </ModalDialog>
  );
}

function ExpenseDialog({
  expense,
  accounts,
  onClose,
  onSaved,
  reportId,
}: {
  expense: Partial<Expense>;
  accounts: Account[];
  onClose: () => void;
  onSaved: () => void;
  reportId?: string;
}) {
  const [e, setE] = useState<Partial<Expense>>({
    receipts: [],
    ...expense,
    report_id: reportId ?? expense.report_id ?? "",
    out_of_pocket: reportId ? true : expense.out_of_pocket,
    paid_from_id: expense.paid_from_id || (reportId || expense.id ? "" : (accounts.find((a) => a.system_key === "bank")?.id ?? "")),
  });
  const [contacts, setContacts] = useState<ContactLite[]>([]);
  const [busy, setBusy] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const { err, run } = useErr();
  useEffect(() => {
    listContacts().then(setContacts);
  }, []);
  const { gst, total } = gstOf(e.amount ?? 0, e.gst_rate ?? 0);
  const locked = !!e.report_id && e.posted === true && !!expense.id;
  const addReceipt = async (f: File) => {
    setBusy(true);
    const r = await run(() => uploadReceipt(f));
    setBusy(false);
    if (r) setE((x) => ({ ...x, receipts: [...(x.receipts ?? []), r] }));
  };
  const save = async () => {
    const ok = await run(() => saveExpense(e));
    if (ok) onSaved();
  };
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: "min(640px, 96vw)", maxHeight: "92vh", overflow: "auto" }} data-testid="acct-expense-dialog">
        <ModalClose />
        <Typography level="h4">{e.id ? "Edit expense" : "New expense"}</Typography>
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 1 }}>
          <FormControl>
            <FormLabel>Date</FormLabel>
            <Input size="sm" type="date" value={e.date ?? ""} onChange={(ev) => setE({ ...e, date: ev.target.value })} data-testid="acct-expense-date" />
          </FormControl>
          <FormControl>
            <FormLabel>Vendor</FormLabel>
            <ContactInput contacts={contacts} id={e.vendor_id ?? ""} name={e.vendor_name ?? ""} onChange={(i, n) => setE((x) => ({ ...x, vendor_id: i, vendor_name: n }))} placeholder="Vendor" />
          </FormControl>
          <FormControl>
            <FormLabel>Category</FormLabel>
            <AccountSelect accounts={accounts} value={e.category_id ?? ""} onChange={(id) => setE({ ...e, category_id: id })} filter={(a) => a.type === "expense"} testid="acct-expense-category" />
          </FormControl>
          <FormControl>
            <FormLabel>Amount before GST</FormLabel>
            <Input size="sm" type="number" slotProps={{ input: { step: "0.01", min: 0, "data-testid": "acct-expense-amount" } }} value={e.amount ?? ""} onChange={(ev) => setE({ ...e, amount: Number(ev.target.value) || 0 })} />
          </FormControl>
          <FormControl>
            <FormLabel>GST rate</FormLabel>
            <Select size="sm" value={e.gst_rate ?? 0} onChange={(_, v) => setE({ ...e, gst_rate: Number(v) || 0 })} slotProps={{ button: { "data-testid": "acct-expense-gst" } as never }}>
              {GST_RATES.map((r) => (
                <Option key={r} value={r}>
                  {r}%
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Total</FormLabel>
            <Typography level="title-md" data-testid="acct-expense-total">
              {money(total)} <Typography level="body-xs">(GST {money(gst)})</Typography>
            </Typography>
          </FormControl>
        </Box>
        {e.out_of_pocket ? (
          <Typography level="body-sm">Paid by the employee. It is posted when the report is approved and settled when reimbursed.</Typography>
        ) : (
          <Box sx={{ display: "flex", gap: 1, alignItems: "flex-end", flexWrap: "wrap" }}>
            <Checkbox label="Paid" checked={e.paid ?? true} onChange={(ev) => setE({ ...e, paid: ev.target.checked })} data-testid="acct-expense-paid" />
            {(e.paid ?? true) ? (
              <FormControl>
                <FormLabel>Paid from</FormLabel>
                <AccountSelect accounts={accounts} value={e.paid_from_id ?? ""} onChange={(id) => setE({ ...e, paid_from_id: id })} filter={isBank} testid="acct-expense-paid-from" />
              </FormControl>
            ) : (
              <Typography level="body-sm">Saved as a bill in Accounts Payable.</Typography>
            )}
          </Box>
        )}
        <FormControl>
          <FormLabel>Notes</FormLabel>
          <Textarea minRows={2} value={e.notes ?? ""} onChange={(ev) => setE({ ...e, notes: ev.target.value })} />
        </FormControl>
        <FormControl>
          <FormLabel>Receipts (photo or PDF, stored in Drive)</FormLabel>
          <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "center" }}>
            {(e.receipts ?? []).map((r) => (
              <Chip
                key={r.id}
                size="md"
                variant="outlined"
                slotProps={{ action: { component: "a", href: receiptURL(r.id), target: "_blank", rel: "noreferrer" } as never }}
                endDecorator={
                  <IconButton size="sm" aria-label={`Remove ${r.name}`} onClick={() => setE({ ...e, receipts: (e.receipts ?? []).filter((x) => x.id !== r.id) })}>
                    <DeleteOutline fontSize="small" />
                  </IconButton>
                }
              >
                {r.name}
              </Chip>
            ))}
            <Button size="sm" variant="soft" loading={busy} onClick={() => fileRef.current?.click()} data-testid="acct-receipt-button">
              Add receipt
            </Button>
            <input ref={fileRef} type="file" accept="image/*,application/pdf" capture="environment" hidden data-testid="acct-receipt-input" onChange={(ev) => ev.target.files?.[0] && addReceipt(ev.target.files[0])} />
          </Box>
        </FormControl>
        <ErrorAlert err={err} />
        <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
          {e.id && !locked && (
            <Button
              color="danger"
              variant="plain"
              sx={{ mr: "auto" }}
              onClick={async () => {
                if (window.confirm("Delete this expense?") && (await run(() => deleteExpense(e.id!))) !== undefined) onSaved();
              }}
            >
              Delete
            </Button>
          )}
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={save} data-testid="acct-expense-save">
            Save
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

function ReportsList({ accounts, version, onChanged }: { accounts: Account[]; version: number; onChanged: () => void }) {
  const [rows, setRows] = useState<ExpenseReport[] | null>(null);
  const [open, setOpen] = useState<ExpenseReport | null>(null);
  const [title, setTitle] = useState("");
  const { err, run } = useErr();
  const load = useCallback(async () => {
    const r = await run(() => listReports());
    if (r) {
      setRows(r);
      setOpen((o) => (o ? (r.find((x) => x.id === o.id) ?? null) : o));
    }
  }, [run]);
  useEffect(() => {
    load();
  }, [load, version]);
  if (open) return <ReportDetail report={open} accounts={accounts} onBack={() => setOpen(null)} onChanged={() => (load(), onChanged())} />;
  return (
    <>
      <Box sx={{ display: "flex", gap: 1, mb: 1.5 }}>
        <Input size="sm" placeholder="New report title, e.g. Delhi trip Nov" value={title} onChange={(e) => setTitle(e.target.value)} sx={{ maxWidth: 360, flex: 1 }} data-testid="acct-report-title" />
        <Button
          size="sm"
          disabled={!title.trim()}
          onClick={async () => {
            const r = await run(() => saveReport({ title }));
            if (r) {
              setTitle("");
              setOpen(r);
              onChanged();
            }
          }}
          data-testid="acct-report-create"
        >
          Create report
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {rows && rows.length === 0 && <Empty>No expense reports. Create one to collect out-of-pocket claims for approval.</Empty>}
      {rows && rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow>
            <thead>
              <tr>
                <th>Title</th>
                <th>Submitted by</th>
                <th style={rightCell}>Expenses</th>
                <th style={rightCell}>Total</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} style={{ cursor: "pointer" }} onClick={() => setOpen(r)}>
                  <td>{r.title}</td>
                  <td>{r.submitter_name}</td>
                  <td style={rightCell}>{r.count}</td>
                  <td style={rightCell}>{money(r.total)}</td>
                  <td>
                    <Chip size="sm" variant="soft" color={STATUS_COLOR[r.status]}>
                      {r.status}
                    </Chip>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </>
  );
}

function ReportDetail({ report, accounts, onBack, onChanged }: { report: ExpenseReport; accounts: Account[]; onBack: () => void; onChanged: () => void }) {
  const [exps, setExps] = useState<Expense[]>([]);
  const [edit, setEdit] = useState<Partial<Expense> | null>(null);
  const [note, setNote] = useState("");
  const [reimburse, setReimburse] = useState(false);
  const { err, run } = useErr();
  const editable = report.status === "draft" || report.status === "rejected";
  const load = useCallback(async () => {
    const r = await run(() => listExpenses({ report_id: report.id }));
    if (r) setExps(r);
  }, [report.id, run]);
  useEffect(() => {
    load();
  }, [load, report.total, report.status]);
  const act = async (action: "submit" | "approve" | "reject") => {
    const r = await run(() => reportAction(report.id, action, { note }));
    if (r) onChanged();
  };
  return (
    <Box>
      <Button size="sm" variant="plain" onClick={onBack}>
        ← Expense reports
      </Button>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap", mb: 1 }}>
        <Typography level="h4">{report.title}</Typography>
        <Chip size="sm" variant="soft" color={STATUS_COLOR[report.status]} data-testid="acct-report-status">
          {report.status}
        </Chip>
        <Typography level="title-md" sx={{ ml: "auto" }}>
          {money(report.total)}
        </Typography>
      </Box>
      {report.decision_note && <Typography level="body-sm">Note: {report.decision_note}</Typography>}
      <ErrorAlert err={err} />
      {exps.length === 0 ? (
        <Empty>No expenses on this report yet.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow>
            <thead>
              <tr>
                <th>Date</th>
                <th>Vendor</th>
                <th>Category</th>
                <th style={rightCell}>Total</th>
              </tr>
            </thead>
            <tbody>
              {exps.map((e) => (
                <tr key={e.id} style={{ cursor: editable ? "pointer" : "default" }} onClick={() => editable && setEdit(e)}>
                  <td>{e.date}</td>
                  <td>
                    {e.vendor_name} {e.receipts.length > 0 && <AttachFile fontSize="small" sx={{ verticalAlign: "middle" }} />}
                  </td>
                  <td>{e.category_name}</td>
                  <td style={rightCell}>{money(e.total)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
      <Box sx={{ display: "flex", gap: 1, mt: 1.5, flexWrap: "wrap", alignItems: "center" }}>
        {editable && (
          <>
            <Button size="sm" variant="soft" onClick={() => setEdit({ date: todayISO(), gst_rate: 0, out_of_pocket: true, receipts: [] })} data-testid="acct-report-add-expense">
              Add expense
            </Button>
            <Button size="sm" onClick={() => act("submit")} disabled={exps.length === 0} data-testid="acct-report-submit">
              Submit for approval
            </Button>
            <Button
              size="sm"
              color="danger"
              variant="plain"
              onClick={async () => {
                if (window.confirm("Delete this report and its expenses?") && (await run(() => deleteReport(report.id))) !== undefined) {
                  onChanged();
                  onBack();
                }
              }}
            >
              Delete report
            </Button>
          </>
        )}
        {report.status === "submitted" && (
          <>
            <Input size="sm" placeholder="Note (optional)" value={note} onChange={(e) => setNote(e.target.value)} />
            <Button size="sm" color="success" onClick={() => act("approve")} data-testid="acct-report-approve">
              Approve
            </Button>
            <Button size="sm" color="danger" variant="soft" onClick={() => act("reject")} data-testid="acct-report-reject">
              Reject
            </Button>
          </>
        )}
        {report.status === "approved" && (
          <Button size="sm" onClick={() => setReimburse(true)} data-testid="acct-report-reimburse">
            Reimburse
          </Button>
        )}
        {report.status === "reimbursed" && <Typography level="body-sm">Reimbursed on {report.reimbursed_at}</Typography>}
      </Box>
      {edit && (
        <ExpenseDialog
          expense={edit}
          accounts={accounts}
          reportId={report.id}
          onClose={() => setEdit(null)}
          onSaved={() => {
            setEdit(null);
            onChanged();
            load();
          }}
        />
      )}
      {reimburse && (
        <Modal open onClose={() => setReimburse(false)}>
          <PayDialog
            title={`Reimburse ${report.submitter_name || "employee"} ${money(report.total)}`}
            accounts={accounts}
            onClose={() => setReimburse(false)}
            onPay={async (acct, date) => {
              const r = await run(() => reportAction(report.id, "reimburse", { account_id: acct, date }));
              if (r) {
                setReimburse(false);
                onChanged();
              }
              return !!r;
            }}
          />
        </Modal>
      )}
    </Box>
  );
}
