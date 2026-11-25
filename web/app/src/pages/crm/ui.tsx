import { useCallback, useState } from "react";
import { auditExport } from "../../lib/auditExport";
import type { ReactNode } from "react";
import { Alert, Autocomplete, Button, DialogTitle, Modal, ModalClose, ModalDialog, Sheet, Typography } from "@mui/joy";
import type { Activity, Company, ContactLite, Deal, FieldDef, Followup, HistoryRow, Me, Person, Settings, UserLite } from "./types";

export interface Data {
  settings: Settings;
  companies: Company[];
  people: Person[];
  deals: Deal[];
  history: HistoryRow[];
  followups: Followup[];
  activities: Activity[];
  contacts: ContactLite[];
  fields: FieldDef[];
  me: Me;
  users: UserLite[];
  reload: () => void;
}

export const contactName = (d: Data, id: string | null | undefined) => (id ? (d.contacts.find((c) => c.id === id)?.display_name ?? "Unknown contact") : "");
export const companyName = (d: Data, id: string | null | undefined) => (id ? (d.companies.find((c) => c.id === id)?.name ?? "") : "");

export function useErr() {
  const [err, setErr] = useState("");
  const run = useCallback(async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
    try {
      setErr("");
      return await fn();
    } catch (e) {
      setErr((e as Error).message);
      return undefined;
    }
  }, []);
  return { err, setErr, run };
}

export function ErrorAlert({ err }: { err: string }) {
  return err ? (
    <Alert color="danger" role="alert" sx={{ my: 1 }}>
      {err}
    </Alert>
  ) : null;
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <Sheet variant="soft" sx={{ p: 4, textAlign: "center", borderRadius: "md" }}>
      <Typography level="title-md">{children}</Typography>
    </Sheet>
  );
}

export function Stat({ label, value, sub, color, testid }: { label: string; value: string; sub?: string; color?: string; testid?: string }) {
  return (
    <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", minWidth: 150, flex: "1 1 150px" }}>
      <Typography level="body-xs">{label}</Typography>
      <Typography level="h4" data-testid={testid} sx={{ color }}>
        {value}
      </Typography>
      {sub && <Typography level="body-xs">{sub}</Typography>}
    </Sheet>
  );
}

export function Dialog({ title, onClose, children, width = 560 }: { title: string; onClose: () => void; children: ReactNode; width?: number }) {
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: { xs: "95vw", sm: width }, maxHeight: "92vh", overflow: "auto" }} role="dialog" aria-label={title}>
        <ModalClose />
        <DialogTitle>{title}</DialogTitle>
        {children}
      </ModalDialog>
    </Modal>
  );
}

export function ConfirmButton({ label, onConfirm, testid }: { label: string; onConfirm: () => void; testid?: string }) {
  const [armed, setArmed] = useState(false);
  return (
    <Button
      size="sm"
      color="danger"
      variant={armed ? "solid" : "outlined"}
      data-testid={testid}
      onClick={() => (armed ? onConfirm() : setArmed(true))}
      onBlur={() => setArmed(false)}
    >
      {armed ? "Confirm?" : label}
    </Button>
  );
}

/** ContactPicker chooses an existing Contact (people ARE Contacts). */
export function ContactPicker({
  contacts,
  value,
  onChange,
  label,
  testid,
}: {
  contacts: ContactLite[];
  value: string;
  onChange: (id: string) => void;
  label?: string;
  testid?: string;
}) {
  const sel = contacts.find((c) => c.id === value) ?? null;
  return (
    <Autocomplete
      placeholder={label ?? "Choose a contact"}
      options={contacts}
      value={sel}
      data-testid={testid}
      getOptionLabel={(c) => c.display_name || c.emails[0] || "Unnamed"}
      isOptionEqualToValue={(a, b) => a.id === b.id}
      onChange={(_, c) => onChange(c?.id ?? "")}
    />
  );
}

export const fmtDate = (s: string | null | undefined) => (s ? new Date(s.length === 10 ? `${s}T12:00:00` : s).toLocaleDateString() : "");
export const ago = (ms: number) => {
  if (!ms) return "never";
  const d = Math.floor((Date.now() - ms) / 86400000);
  return d <= 0 ? "today" : d === 1 ? "yesterday" : `${d} days ago`;
};

export function downloadCSV(name: string, csv: string) {
  auditExport("crm", name, Math.max(0, csv.split("\n").length - 1));
  const url = URL.createObjectURL(new Blob([csv], { type: "text/csv" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
