import { useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Table, Typography } from "@mui/joy";
import { createAccount, deleteAccount, getRegister, updateAccount } from "./api";
import { money } from "./logic";
import type { Account, AccountType, RegisterRow } from "./types";
import { Empty, ErrorAlert, rightCell, useErr } from "./ui";

const TYPES: AccountType[] = ["asset", "liability", "equity", "income", "expense"];
const TYPE_LABEL: Record<AccountType, string> = { asset: "Assets", liability: "Liabilities", equity: "Equity", income: "Income", expense: "Expenses" };
const SUBTYPES = ["", "bank", "cash", "receivable", "payable", "fixed_asset", "loan", "opex", "sales", "other"];

export function AccountsView({ accounts, onChanged, onOpenJournal }: { accounts: Account[]; onChanged: () => void; onOpenJournal: (id: string) => void }) {
  const [edit, setEdit] = useState<Partial<Account> | null>(null);
  const [reg, setReg] = useState<Account | null>(null);
  const { err, run } = useErr();
  const save = async () => {
    if (!edit) return;
    const ok = await run(() => (edit.id ? updateAccount(edit.id, edit) : createAccount(edit)));
    if (ok) {
      setEdit(null);
      onChanged();
    }
  };
  if (reg) return <Register account={reg} onBack={() => setReg(null)} onOpenJournal={onOpenJournal} />;
  return (
    <Box>
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", mb: 1.5 }}>
        <Typography level="h3" sx={{ mr: "auto" }}>
          Chart of accounts
        </Typography>
        <Button size="sm" onClick={() => setEdit({ type: "expense", subtype: "opex", active: true })} data-testid="acct-new-account">
          New account
        </Button>
      </Box>
      <ErrorAlert err={err} />
      {TYPES.map((t) => {
        const list = accounts.filter((a) => a.type === t);
        if (!list.length) return null;
        return (
          <Sheet key={t} variant="outlined" sx={{ borderRadius: "md", mb: 2, overflow: "auto" }}>
            <Table size="sm" hoverRow>
              <thead>
                <tr>
                  <th colSpan={4}>{TYPE_LABEL[t]}</th>
                </tr>
              </thead>
              <tbody>
                {list.map((a) => (
                  <tr key={a.id} style={{ opacity: a.active ? 1 : 0.5 }}>
                    <td style={{ width: 70 }}>{a.code}</td>
                    <td>
                      <Typography
                        component="button"
                        onClick={() => setReg(a)}
                        sx={{ background: "none", border: 0, p: 0, cursor: "pointer", color: "primary.plainColor", font: "inherit", textAlign: "left" }}
                      >
                        {a.name}
                      </Typography>{" "}
                      {a.system_key && (
                        <Chip size="sm" variant="soft">
                          system
                        </Chip>
                      )}
                    </td>
                    <td style={{ ...rightCell, width: 150 }}>{money(a.balance)}</td>
                    <td style={{ width: 70, textAlign: "right" }}>
                      <Button size="sm" variant="plain" onClick={() => setEdit(a)}>
                        Edit
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </Table>
          </Sheet>
        );
      })}
      {edit && (
        <Modal open onClose={() => setEdit(null)}>
          <ModalDialog sx={{ width: "min(480px, 94vw)" }}>
            <ModalClose />
            <Typography level="h4">{edit.id ? "Edit account" : "New account"}</Typography>
            <FormControl>
              <FormLabel>Name</FormLabel>
              <Input value={edit.name ?? ""} onChange={(e) => setEdit({ ...edit, name: e.target.value })} data-testid="acct-account-name" autoFocus />
            </FormControl>
            <Box sx={{ display: "flex", gap: 1 }}>
              <FormControl sx={{ width: 110 }}>
                <FormLabel>Code</FormLabel>
                <Input value={edit.code ?? ""} onChange={(e) => setEdit({ ...edit, code: e.target.value })} data-testid="acct-account-code" />
              </FormControl>
              <FormControl sx={{ flex: 1 }}>
                <FormLabel>Type</FormLabel>
                <Select value={edit.type ?? "expense"} onChange={(_, v) => setEdit({ ...edit, type: (v as AccountType) ?? "expense" })} slotProps={{ button: { "data-testid": "acct-account-type" } as never }}>
                  {TYPES.map((t) => (
                    <Option key={t} value={t}>
                      {TYPE_LABEL[t]}
                    </Option>
                  ))}
                </Select>
              </FormControl>
            </Box>
            <FormControl>
              <FormLabel>Detail type (bank and cash accounts appear in Banking)</FormLabel>
              <Select value={edit.subtype ?? ""} onChange={(_, v) => setEdit({ ...edit, subtype: (v as string) ?? "" })}>
                {SUBTYPES.map((s) => (
                  <Option key={s} value={s}>
                    {s || "General"}
                  </Option>
                ))}
              </Select>
            </FormControl>
            {edit.id && <Checkbox label="Active" checked={edit.active !== false} onChange={(e) => setEdit({ ...edit, active: e.target.checked })} />}
            <ErrorAlert err={err} />
            <Box sx={{ display: "flex", gap: 1, justifyContent: "flex-end" }}>
              {edit.id && !edit.system_key && (
                <Button
                  color="danger"
                  variant="plain"
                  sx={{ mr: "auto" }}
                  onClick={async () => {
                    if (window.confirm("Delete this account?") && (await run(() => deleteAccount(edit.id!))) !== undefined) {
                      setEdit(null);
                      onChanged();
                    }
                  }}
                >
                  Delete
                </Button>
              )}
              <Button variant="plain" color="neutral" onClick={() => setEdit(null)}>
                Cancel
              </Button>
              <Button onClick={save} data-testid="acct-account-save">
                Save
              </Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}
    </Box>
  );
}

function Register({ account, onBack, onOpenJournal }: { account: Account; onBack: () => void; onOpenJournal: (id: string) => void }) {
  const [rows, setRows] = useState<RegisterRow[] | null>(null);
  const [opening, setOpening] = useState(0);
  const { err, run } = useErr();
  useEffect(() => {
    run(() => getRegister(account.id)).then((r) => {
      if (r) {
        setRows(r.rows ?? []);
        setOpening(r.opening);
      }
    });
  }, [account.id, run]);
  return (
    <Box>
      <Button size="sm" variant="plain" onClick={onBack}>
        ← Chart of accounts
      </Button>
      <Typography level="h3" sx={{ mb: 1 }}>
        {account.code} {account.name} <Typography level="body-md">balance {money(account.balance)}</Typography>
      </Typography>
      <ErrorAlert err={err} />
      {rows && rows.length === 0 && <Empty>No postings in this account yet.</Empty>}
      {rows && rows.length > 0 && (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table size="sm" hoverRow data-testid="acct-register">
            <thead>
              <tr>
                <th style={{ width: 110 }}>Date</th>
                <th>Description</th>
                <th>Contact</th>
                <th style={rightCell}>Debit</th>
                <th style={rightCell}>Credit</th>
                <th style={rightCell}>Balance</th>
              </tr>
            </thead>
            <tbody>
              {opening !== 0 && (
                <tr>
                  <td colSpan={5}>Opening balance</td>
                  <td style={rightCell}>{money(opening)}</td>
                </tr>
              )}
              {rows.map((r, i) => (
                <tr key={i} style={{ cursor: "pointer" }} onClick={() => onOpenJournal(r.journal_id)}>
                  <td>{r.date}</td>
                  <td>{r.memo}</td>
                  <td>{r.contact}</td>
                  <td style={rightCell}>{r.debit ? money(r.debit) : ""}</td>
                  <td style={rightCell}>{r.credit ? money(r.credit) : ""}</td>
                  <td style={rightCell}>{money(r.balance)}</td>
                </tr>
              ))}
            </tbody>
          </Table>
        </Sheet>
      )}
    </Box>
  );
}
