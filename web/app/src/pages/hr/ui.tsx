import { useCallback, useState } from "react";
import type { ReactNode } from "react";
import { Alert, Box, Button, Chip, DialogTitle, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Sheet, Typography } from "@mui/joy";
import { STATUS_COLOR, statusLabel } from "./logic";

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
  if (!err) return null;
  return (
    <Alert color="danger" variant="soft" sx={{ my: 1 }} role="alert" data-testid="hr-error">
      {err}
    </Alert>
  );
}

export function StatusChip({ status }: { status: string }) {
  return (
    <Chip size="sm" variant="soft" color={STATUS_COLOR[status] ?? "neutral"} data-testid={`status-${status}`}>
      {statusLabel(status)}
    </Chip>
  );
}

export function Card({ title, action, children, sx }: { title?: string; action?: ReactNode; children: ReactNode; sx?: object }) {
  return (
    <Sheet variant="outlined" sx={{ p: 2, borderRadius: "md", ...sx }}>
      {(title || action) && (
        <Box sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", mb: 1, gap: 1 }}>
          <Typography level="title-md">{title}</Typography>
          {action}
        </Box>
      )}
      {children}
    </Sheet>
  );
}

export function Field({ label, children, sx }: { label: string; children: ReactNode; sx?: object }) {
  return (
    <FormControl sx={{ minWidth: 160, flex: 1, ...sx }}>
      <FormLabel>{label}</FormLabel>
      {children}
    </FormControl>
  );
}

export function Dialog({ open, onClose, title, children, wide }: { open: boolean; onClose: () => void; title: string; children: ReactNode; wide?: boolean }) {
  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: wide ? "min(900px, 96vw)" : "min(560px, 96vw)", maxHeight: "92vh", overflow: "auto" }}>
        <ModalClose />
        <DialogTitle>{title}</DialogTitle>
        {children}
      </ModalDialog>
    </Modal>
  );
}

export function Stat({ label, value, hint, testId }: { label: string; value: ReactNode; hint?: string; testId?: string }) {
  return (
    <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md", minWidth: 130, flex: 1 }} data-testid={testId}>
      <Typography level="body-xs" sx={{ textTransform: "uppercase", letterSpacing: 0.5 }}>
        {label}
      </Typography>
      <Typography level="h3">{value}</Typography>
      {hint && <Typography level="body-xs">{hint}</Typography>}
    </Sheet>
  );
}

/** A button that opens a native file picker and hands back the chosen file. */
export function FileButton({ label, accept, onFile, disabled }: { label: string; accept?: string; onFile: (f: File) => void; disabled?: boolean }) {
  return (
    <Button component="label" variant="outlined" size="sm" disabled={disabled}>
      {label}
      <input
        hidden
        type="file"
        accept={accept}
        onChange={(e) => {
          const f = e.target.files?.[0];
          if (f) onFile(f);
          e.target.value = "";
        }}
      />
    </Button>
  );
}

export function DateInput({ value, onChange, label, min }: { value: string; onChange: (v: string) => void; label: string; min?: string }) {
  return <Input type="date" value={value} onChange={(e) => onChange(e.target.value)} slotProps={{ input: { "aria-label": label, min } }} />;
}

/** Plain download link styled as a button (same-origin GET with cookies). */
export function DownloadLink({ href, children, testId }: { href: string; children: ReactNode; testId?: string }) {
  return (
    <Button component="a" href={href} download variant="outlined" size="sm" data-testid={testId}>
      {children}
    </Button>
  );
}

export const Row = ({ children, sx }: { children: ReactNode; sx?: object }) => (
  <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap", alignItems: "flex-end", ...sx }}>{children}</Box>
);
