// One confirm dialog for every security action: reason (required), ticket,
// the action's own fields, a dry-run preview, step-up when the server asks for
// it, and the result (a one-time secret is shown here and never again).
import { useEffect, useState } from "react";
import { Alert, Box, Button, DialogActions, DialogContent, DialogTitle, FormControl, FormLabel, Input, Modal, ModalDialog, Typography, Chip } from "@mui/joy";
import { A, IamError, iam, type ActionResult } from "./api";
import { paramsFor, validateAction, type ActionDef } from "./logic";

export interface Target {
  id: string;
  email: string;
}

export function StepUpDialog({ open, onClose, onDone }: { open: boolean; onClose: () => void; onDone: () => void }) {
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (open) {
      setPassword("");
      setCode("");
      setErr("");
    }
  }, [open]);
  const go = async () => {
    setBusy(true);
    setErr("");
    try {
      await iam(`${A}/step-up`, { method: "POST", json: { password, code } });
      onDone();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: 420 }} data-testid="stepup-dialog">
        <DialogTitle>Confirm it is you</DialogTitle>
        <DialogContent>
          <Typography level="body-sm" sx={{ mb: 1.5 }}>
            This change needs a fresh sign-in check. It stays valid for 10 minutes.
          </Typography>
          {err && (
            <Alert color="danger" variant="soft" sx={{ mb: 1 }}>
              {err}
            </Alert>
          )}
          <FormControl sx={{ mb: 1 }}>
            <FormLabel>Your password</FormLabel>
            <Input type="password" autoFocus value={password} onChange={(e) => setPassword(e.target.value)} slotProps={{ input: { "data-testid": "stepup-password" } }} onKeyDown={(e) => e.key === "Enter" && password && void go()} />
          </FormControl>
          <FormControl>
            <FormLabel>One-time code (only if you use an authenticator app)</FormLabel>
            <Input value={code} onChange={(e) => setCode(e.target.value)} inputMode="numeric" />
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button loading={busy} disabled={!password} onClick={() => void go()} data-testid="stepup-go">
            Confirm
          </Button>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}

export function ActionDialog({
  def,
  target,
  extra,
  alertId,
  onClose,
  onDone,
}: {
  def: ActionDef | null;
  target: Target;
  extra?: Record<string, unknown>;
  alertId?: string;
  onClose: () => void;
  onDone: () => void;
}) {
  const [reason, setReason] = useState("");
  const [ticket, setTicket] = useState("");
  const [fields, setFields] = useState<Record<string, string>>({});
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ActionResult | null>(null);
  const [preview, setPreview] = useState<ActionResult | null>(null);
  const [needStep, setNeedStep] = useState(false);
  const [pending, setPending] = useState<boolean | null>(null);

  useEffect(() => {
    setReason("");
    setTicket("");
    setFields({});
    setErr("");
    setResult(null);
    setPreview(null);
    setNeedStep(false);
  }, [def?.action, target.id]);

  if (!def) return null;

  const url = def.action === "contain" && alertId ? `${A}/alerts/${alertId}/contain` : `${A}/users/${target.id}/actions`;
  const send = async (dry: boolean) => {
    if (!dry) {
      const v = validateAction(def, reason, fields);
      if (v) {
        setErr(v);
        return;
      }
    }
    setBusy(true);
    setErr("");
    try {
      const res = await iam<ActionResult>(url, {
        method: "POST",
        json: { action: def.action, reason, ticket, dry_run: dry, params: { ...paramsFor(def, fields), ...(extra ?? {}) } },
      });
      if (dry) setPreview(res);
      else {
        setResult(res);
        onDone();
      }
    } catch (e) {
      if (e instanceof IamError && e.stepUp) {
        setPending(dry);
        setNeedStep(true);
      } else setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Modal open onClose={onClose}>
        <ModalDialog sx={{ width: 520, maxHeight: "90vh", overflow: "auto" }} data-testid="action-dialog">
          <DialogTitle>
            {def.label}
            <Chip size="sm" variant="soft" sx={{ ml: 1 }}>
              {target.email}
            </Chip>
          </DialogTitle>
          <DialogContent>
            {!result && (
              <>
                <Typography level="body-sm" sx={{ mb: 1.5 }}>
                  {def.help}
                </Typography>
                {err && (
                  <Alert color="danger" variant="soft" sx={{ mb: 1 }} data-testid="action-error">
                    {err}
                  </Alert>
                )}
                {(def.fields ?? []).map((f) => (
                  <FormControl key={f.key} sx={{ mb: 1 }}>
                    <FormLabel>{f.label}</FormLabel>
                    <Input type={f.type === "number" ? "number" : "text"} placeholder={f.placeholder} value={fields[f.key] ?? ""} onChange={(e) => setFields({ ...fields, [f.key]: e.target.value })} slotProps={{ input: { "data-testid": `field-${f.key}` } }} />
                  </FormControl>
                ))}
                <FormControl sx={{ mb: 1 }} required>
                  <FormLabel>Reason (kept in the audit log)</FormLabel>
                  <Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="For example: lost phone, confirmed on a video call" slotProps={{ input: { "data-testid": "action-reason" } }} />
                </FormControl>
                <FormControl>
                  <FormLabel>Ticket id (optional)</FormLabel>
                  <Input value={ticket} onChange={(e) => setTicket(e.target.value)} placeholder="IT-1234" slotProps={{ input: { "data-testid": "action-ticket" } }} />
                </FormControl>
                <Typography level="body-xs" sx={{ mt: 1.5, opacity: 0.7 }}>
                  The person is notified of this change.
                </Typography>
                {preview && (
                  <Alert color="neutral" variant="soft" sx={{ mt: 1.5, display: "block" }} data-testid="action-preview">
                    <Typography level="title-sm">Preview: nothing changed yet</Typography>
                    <Typography level="body-sm">{preview.message}</Typography>
                    {(preview.steps ?? []).map((s) => (
                      <Typography key={s.name} level="body-sm">
                        • {s.name}: {s.note}
                      </Typography>
                    ))}
                  </Alert>
                )}
              </>
            )}
            {result && (
              <Box data-testid="action-result">
                <Alert color={result.status === "pending_approval" ? "warning" : "success"} variant="soft" sx={{ mb: 1.5, display: "block" }}>
                  {result.message}
                </Alert>
                {(result.steps ?? []).map((s) => (
                  <Box key={s.name} sx={{ display: "flex", gap: 1, mb: 0.5, alignItems: "baseline" }}>
                    <Chip size="sm" color={s.status === "failed" ? "danger" : s.status === "done" ? "success" : "neutral"} variant="soft">
                      {s.status}
                    </Chip>
                    <Typography level="body-sm">
                      <b>{s.name}</b>: {s.note}
                    </Typography>
                  </Box>
                ))}
                {result.secret && (
                  <Box sx={{ mt: 1.5, p: 1.5, bgcolor: "warning.softBg", borderRadius: "sm" }}>
                    <Typography level="body-xs">{result.secret_label}. Shown once. It is not stored.</Typography>
                    <Typography level="title-lg" sx={{ fontFamily: "monospace", userSelect: "all" }} data-testid="action-secret">
                      {result.secret}
                    </Typography>
                    <Button size="sm" variant="outlined" sx={{ mt: 1 }} onClick={() => void navigator.clipboard?.writeText(result.secret ?? "")}>
                      Copy
                    </Button>
                  </Box>
                )}
              </Box>
            )}
          </DialogContent>
          <DialogActions>
            {!result ? (
              <>
                <Button color={def.danger ? "danger" : "primary"} loading={busy} onClick={() => void send(false)} data-testid="action-confirm">
                  {def.label}
                </Button>
                <Button variant="outlined" color="neutral" disabled={busy} onClick={() => void send(true)} data-testid="action-preview-btn">
                  Preview
                </Button>
                <Button variant="plain" color="neutral" onClick={onClose}>
                  Cancel
                </Button>
              </>
            ) : (
              <Button onClick={onClose} data-testid="action-close">
                Close
              </Button>
            )}
          </DialogActions>
        </ModalDialog>
      </Modal>
      <StepUpDialog
        open={needStep}
        onClose={() => setNeedStep(false)}
        onDone={() => {
          setNeedStep(false);
          void send(pending ?? false);
        }}
      />
    </>
  );
}
