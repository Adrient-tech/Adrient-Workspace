// Shared helpers for the identity admin sections: a JSON fetch that surfaces the
// server's error text, a page header and a small confirm dialog.
import { useState, type ReactNode } from "react";
import { Alert, Box, Button, DialogActions, DialogContent, DialogTitle, Modal, ModalDialog, Typography } from "@mui/joy";

export async function api<T>(path: string, init?: RequestInit & { json?: unknown }): Promise<T> {
  const { json, ...rest } = init ?? {};
  const r = await fetch(path, {
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(json !== undefined ? { "Content-Type": "application/json" } : {}) },
    ...(json !== undefined ? { body: JSON.stringify(json) } : {}),
    ...rest,
  });
  if (!r.ok) {
    let msg = `HTTP ${r.status}`;
    try {
      const b = (await r.json()) as { error?: string; message?: string; detail?: string };
      msg = b.error && b.error !== "forbidden" ? b.error : (b.message ?? b.detail ?? msg);
    } catch {
      /* keep the status */
    }
    throw new Error(msg);
  }
  const text = await r.text();
  return (text ? JSON.parse(text) : {}) as T;
}

export function Page({ title, intro, children, testId }: { title: string; intro?: string; children: ReactNode; testId: string }) {
  return (
    <Box data-testid={testId}>
      <Typography level="h4">{title}</Typography>
      {intro && (
        <Typography level="body-sm" sx={{ opacity: 0.7, mb: 2, maxWidth: 760 }}>
          {intro}
        </Typography>
      )}
      {children}
    </Box>
  );
}

export function Notice({ error, notice }: { error?: string | null; notice?: string | null }) {
  return (
    <>
      {notice && (
        <Alert color="success" variant="soft" sx={{ mb: 1.5 }} data-testid="notice">
          {notice}
        </Alert>
      )}
      {error && (
        <Alert color="danger" variant="soft" sx={{ mb: 1.5 }} data-testid="error">
          {error}
        </Alert>
      )}
    </>
  );
}

export function Confirm({
  open,
  title,
  body,
  confirmLabel,
  danger,
  onCancel,
  onConfirm,
}: {
  open: boolean;
  title: string;
  body: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  onCancel: () => void;
  onConfirm: () => void | Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  return (
    <Modal open={open} onClose={onCancel}>
      <ModalDialog sx={{ maxWidth: 460 }}>
        <DialogTitle>{title}</DialogTitle>
        <DialogContent>{body}</DialogContent>
        <DialogActions>
          <Button
            color={danger ? "danger" : "primary"}
            loading={busy}
            data-testid="confirm-yes"
            onClick={async () => {
              setBusy(true);
              try {
                await onConfirm();
              } finally {
                setBusy(false);
              }
            }}
          >
            {confirmLabel}
          </Button>
          <Button variant="plain" color="neutral" onClick={onCancel}>
            Cancel
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}

export function when(s?: string | null): string {
  if (!s) return "";
  const d = new Date(s);
  return Number.isNaN(d.getTime()) ? s : d.toLocaleString();
}
