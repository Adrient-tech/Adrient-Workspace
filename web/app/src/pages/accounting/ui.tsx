import { useCallback, useState } from "react";
import type { ReactNode } from "react";
import { Alert, Autocomplete, Option, Select, Sheet, Typography } from "@mui/joy";
import type { Account, ContactLite } from "./types";

export const rightCell = { textAlign: "right" } as const;

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

export function Stat({ label, value, color, testid }: { label: string; value: string; color?: string; testid?: string }) {
  return (
    <Sheet variant="outlined" sx={{ p: 1.5, borderRadius: "md", minWidth: 150, flex: "1 1 150px" }}>
      <Typography level="body-xs">{label}</Typography>
      <Typography level="h4" data-testid={testid} sx={{ color }}>
        {value}
      </Typography>
    </Sheet>
  );
}

/** Account picker; `filter` limits the choices (for example bank accounts only). */
export function AccountSelect({
  accounts,
  value,
  onChange,
  filter,
  placeholder = "Choose account",
  size = "sm",
  testid,
}: {
  accounts: Account[];
  value: string;
  onChange: (id: string) => void;
  filter?: (a: Account) => boolean;
  placeholder?: string;
  size?: "sm" | "md";
  testid?: string;
}) {
  const list = accounts.filter((a) => (a.active || a.id === value) && (!filter || filter(a)));
  return (
    <Select
      size={size}
      value={value || null}
      placeholder={placeholder}
      onChange={(_, v) => onChange((v as string) ?? "")}
      slotProps={{ button: { "data-testid": testid } as never, listbox: { sx: { maxHeight: 320, zIndex: 2000 } } }}
      sx={{ minWidth: 190 }}
    >
      {list.map((a) => (
        <Option key={a.id} value={a.id}>
          {a.code} {a.name}
        </Option>
      ))}
    </Select>
  );
}

export const isBank = (a: Account) => a.type === "asset" && (a.subtype === "bank" || a.subtype === "cash");

/** Contact picker that also accepts a free-typed name. */
export function ContactInput({
  contacts,
  id,
  name,
  onChange,
  placeholder = "Contact (optional)",
}: {
  contacts: ContactLite[];
  id: string;
  name: string;
  onChange: (id: string, name: string) => void;
  placeholder?: string;
}) {
  const sel = contacts.find((c) => c.id === id) ?? null;
  return (
    <Autocomplete
      size="sm"
      freeSolo
      placeholder={placeholder}
      options={contacts}
      getOptionLabel={(o) => (typeof o === "string" ? o : o.display_name)}
      value={sel ?? (name || null)}
      onChange={(_, v) => {
        if (!v) onChange("", "");
        else if (typeof v === "string") onChange("", v);
        else onChange(v.id, v.display_name);
      }}
      onInputChange={(_, v, reason) => {
        if (reason === "input" && !contacts.some((c) => c.display_name === v)) onChange("", v);
      }}
      sx={{ minWidth: 190 }}
    />
  );
}
