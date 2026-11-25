import { usePermissions } from "../../lib/permissions";
import { useCallback, useEffect, useState } from "react";
import {
  Box,
  Button,
  Chip,
  DialogActions,
  FormControl,
  FormLabel,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Sheet,
  Table,
  Typography,
} from "@mui/joy";
import { cashMove, closeShift, getShift, listShifts, openShift } from "./api";
import { money, round2 } from "./logic";
import type { PosRegister, PosSettings, PosShift } from "./types";
import { Empty, ErrorAlert, rightCell, useErr } from "./ui";
import { ZReportDialog } from "./PosExtDialogs";

export function ShiftsView({
  registers,
  settings,
  version,
  onChanged,
}: {
  registers: PosRegister[];
  settings: PosSettings;
  version: number;
  onChanged: () => void;
}) {
  const [shifts, setShifts] = useState<PosShift[]>([]);
  const [live, setLive] = useState<Record<string, PosShift>>({});
  const [dlg, setDlg] = useState<{ kind: "open" | "close" | "move"; reg: PosRegister } | null>(null);
  const [zShift, setZShift] = useState("");
  const { can } = usePermissions();
  const m = (n: number) => money(n, settings.currency);

  const load = useCallback(async () => {
    setShifts(await listShifts().catch(() => []));
    const entries = await Promise.all(
      registers.filter((r) => r.open_shift_id).map(async (r) => [r.id, await getShift(r.open_shift_id).catch(() => null)] as const),
    );
    const o: Record<string, PosShift> = {};
    for (const [id, s] of entries) if (s) o[id] = s;
    setLive(o);
  }, [registers]);
  useEffect(() => {
    load();
  }, [load, version]);

  const done = () => {
    setDlg(null);
    onChanged();
    load();
  };

  return (
    <Box>
      {registers.length === 0 && <Empty>Add a register in Settings first.</Empty>}
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))", gap: 2, mb: 3 }}>
        {registers.map((r) => {
          const s = live[r.id];
          return (
            <Sheet key={r.id} variant="outlined" sx={{ p: 2, borderRadius: "md" }} data-testid="pos-drawer-card">
              <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <Typography level="title-md">{r.name}</Typography>
                <Chip size="sm" color={s ? "success" : "neutral"} variant="soft">
                  {s ? "Open" : "Closed"}
                </Chip>
              </Box>
              {s ? (
                <>
                  <Typography level="body-sm" sx={{ mt: 1 }}>
                    Opened {new Date(s.opened_at).toLocaleString()} by {s.opened_by}
                  </Typography>
                  <Box sx={{ display: "grid", gridTemplateColumns: "1fr auto", gap: 0.25, mt: 1, fontSize: "sm" }}>
                    <span>Opening cash</span>
                    <span>{m(s.opening_cash)}</span>
                    <span>Cash sales</span>
                    <span>{m(s.cash_sales)}</span>
                    <span>Cash refunds</span>
                    <span>-{m(s.cash_refunds)}</span>
                    <span>Paid in / out</span>
                    <span>
                      {m(s.cash_in)} / {m(s.cash_out)}
                    </span>
                    <strong>Expected in drawer</strong>
                    <strong data-testid="pos-expected">{m(s.expected_cash)}</strong>
                    <span>All sales ({s.sales_count})</span>
                    <span>{m(s.sales_total)}</span>
                  </Box>
                  <Box sx={{ display: "flex", gap: 1, mt: 1.5 }}>
                    <Button size="sm" variant="outlined" disabled={!can("pos:cash:move")} title={can("pos:cash:move") ? undefined : "Needs Manager or above"} onClick={() => setDlg({ kind: "move", reg: r })} data-testid="pos-cash-move">
                      Pay in / out
                    </Button>
                    <Button size="sm" color="danger" variant="soft" disabled={!can("pos:shift:close")} title={can("pos:shift:close") ? undefined : "Needs Manager or above"} onClick={() => setDlg({ kind: "close", reg: r })} data-testid="pos-close-shift">
                      Close shift
                    </Button>
                    <Button size="sm" variant="outlined" onClick={() => setZShift(r.open_shift_id)} data-testid="pos-z-open">
                      X report
                    </Button>
                  </Box>
                </>
              ) : (
                <Button size="sm" sx={{ mt: 1.5 }} onClick={() => setDlg({ kind: "open", reg: r })} data-testid="pos-shift-open-btn">
                  Open shift
                </Button>
              )}
            </Sheet>
          );
        })}
      </Box>
      <Typography level="title-md" sx={{ mb: 1 }}>
        Shift history
      </Typography>
      {shifts.length === 0 ? (
        <Empty>No shifts yet.</Empty>
      ) : (
        <Sheet variant="outlined" sx={{ borderRadius: "md", overflow: "auto" }}>
          <Table aria-label="Shift history">
            <thead>
              <tr>
                <th>Register</th>
                <th>Opened</th>
                <th>Closed</th>
                <th style={rightCell}>Sales</th>
                <th style={rightCell}>Expected</th>
                <th style={rightCell}>Counted</th>
                <th style={rightCell}>Variance</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {shifts.map((s) => (
                <tr key={s.id} data-testid="pos-shift-row">
                  <td>{s.register_name}</td>
                  <td>{new Date(s.opened_at).toLocaleString()}</td>
                  <td>{s.closed ? new Date(s.closed_at).toLocaleString() : "Open"}</td>
                  <td style={rightCell}>{m(s.sales_total)}</td>
                  <td style={rightCell}>{m(s.expected_cash)}</td>
                  <td style={rightCell}>{s.closed ? m(s.counted_cash) : ""}</td>
                  <td style={rightCell}>
                    {s.closed ? (
                      <Chip size="sm" variant="soft" color={Math.abs(s.variance) < 0.005 ? "success" : "warning"}>
                        {m(s.variance)}
                      </Chip>
                    ) : (
                      ""
                    )}
                  </td>
                  <td style={rightCell}>
                    <Button size="sm" variant="plain" onClick={() => setZShift(s.id)} data-testid="pos-z-btn">
                      Z report
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
          {zShift && <ZReportDialog shiftId={zShift} currency={settings.currency} onClose={() => setZShift("")} />}
        </Sheet>
      )}
      {dlg?.kind === "open" && <OpenDialog reg={dlg.reg} onClose={() => setDlg(null)} onDone={done} />}
      {dlg?.kind === "close" && live[dlg.reg.id] && <CloseDialog shift={live[dlg.reg.id]} settings={settings} onClose={() => setDlg(null)} onDone={done} />}
      {dlg?.kind === "move" && live[dlg.reg.id] && <MoveDialog shift={live[dlg.reg.id]} onClose={() => setDlg(null)} onDone={done} />}
    </Box>
  );
}

function amountInput(v: string, set: (s: string) => void, testid: string) {
  return <Input type="number" value={v} onChange={(e) => set(e.target.value)} slotProps={{ input: { min: 0, step: "any", "data-testid": testid } }} />;
}

function OpenDialog({ reg, onClose, onDone }: { reg: PosRegister; onClose: () => void; onDone: () => void }) {
  const [cash, setCash] = useState("0");
  const { err, run } = useErr();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 380, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">Open shift on {reg.name}</Typography>
        <FormControl>
          <FormLabel>Opening cash in drawer</FormLabel>
          {amountInput(cash, setCash, "pos-opening-cash")}
        </FormControl>
        <ErrorAlert err={err} />
        <DialogActions>
          <Button onClick={async () => (await run(() => openShift(reg.id, round2(parseFloat(cash) || 0)))) && onDone()} data-testid="pos-open-shift-confirm">
            Open shift
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}

function CloseDialog({ shift, settings, onClose, onDone }: { shift: PosShift; settings: PosSettings; onClose: () => void; onDone: () => void }) {
  const [counted, setCounted] = useState("");
  const [note, setNote] = useState("");
  const { err, run } = useErr();
  const m = (n: number) => money(n, settings.currency);
  const c = parseFloat(counted);
  const variance = Number.isFinite(c) ? round2(c - shift.expected_cash) : null;
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 400, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">Close shift on {shift.register_name}</Typography>
        <Typography level="body-sm">Expected in drawer: {m(shift.expected_cash)}</Typography>
        <FormControl required>
          <FormLabel>Counted cash</FormLabel>
          {amountInput(counted, setCounted, "pos-counted")}
        </FormControl>
        {variance !== null && (
          <Typography level="title-sm" color={Math.abs(variance) < 0.005 ? "success" : "warning"} data-testid="pos-variance">
            Variance {m(variance)}
          </Typography>
        )}
        <FormControl>
          <FormLabel>Note</FormLabel>
          <Input value={note} onChange={(e) => setNote(e.target.value)} />
        </FormControl>
        <ErrorAlert err={err} />
        <DialogActions>
          <Button
            color="danger"
            disabled={variance === null}
            onClick={async () => (await run(() => closeShift(shift.id, round2(c), note))) && onDone()}
            data-testid="pos-close-confirm"
          >
            Close shift
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}

function MoveDialog({ shift, onClose, onDone }: { shift: PosShift; onClose: () => void; onDone: () => void }) {
  const [amount, setAmount] = useState("");
  const [note, setNote] = useState("");
  const { err, run } = useErr();
  const a = parseFloat(amount);
  const go = async (sign: 1 | -1) => (await run(() => cashMove(shift.id, sign * round2(a), note))) && onDone();
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 380, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">Pay in / out</Typography>
        <FormControl>
          <FormLabel>Amount</FormLabel>
          {amountInput(amount, setAmount, "pos-move-amount")}
        </FormControl>
        <FormControl>
          <FormLabel>Reason</FormLabel>
          <Input value={note} onChange={(e) => setNote(e.target.value)} slotProps={{ input: { "data-testid": "pos-move-note" } }} />
        </FormControl>
        <ErrorAlert err={err} />
        <DialogActions>
          <Button variant="outlined" disabled={!(a > 0)} onClick={() => go(-1)} data-testid="pos-move-out">
            Pay out
          </Button>
          <Button disabled={!(a > 0)} onClick={() => go(1)} data-testid="pos-move-in">
            Pay in
          </Button>
        </DialogActions>
      </ModalDialog>
    </Modal>
  );
}
