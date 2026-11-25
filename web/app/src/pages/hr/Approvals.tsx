import { LoadOlder } from "../../components/LoadOlder";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Input, Table, Typography } from "@mui/joy";
import { hr } from "./api";
import type { LeaveRequest, Regularisation } from "./api";
import { Card, ErrorAlert, Row, StatusChip, useErr } from "./ui";

export function Approvals({ onChanged }: { onChanged: () => void }) {
  const { err, run } = useErr();
  const [leave, setLeave] = useState<LeaveRequest[]>([]);
  const [nextLeave, setNextLeave] = useState("");
  const [regs, setRegs] = useState<Regularisation[]>([]);
  const [recent, setRecent] = useState<LeaveRequest[]>([]);
  const [note, setNote] = useState("");
  const load = useCallback(() => void run(async () => {
    const pg = await hr.leaveRequestsPage("team", "pending");
    setLeave(pg.items);
    setNextLeave(pg.next);
    setRegs(await hr.regularisations("team", "pending"));
    setRecent((await hr.leaveRequests("team")).filter((r) => r.status !== "pending").slice(0, 10));
  }), [run]);
  useEffect(load, [load]);
  const decideLeave = (id: string, a: "approve" | "reject") => void run(async () => { await hr.decideLeave(id, a, note); setNote(""); load(); onChanged(); });
  const decideReg = (id: string, a: "approve" | "reject") => void run(async () => { await hr.decideRegularisation(id, a, note); setNote(""); load(); onChanged(); });
  return (
    <Box sx={{ display: "grid", gap: 2 }}>
      <ErrorAlert err={err} />
      <Row><Input placeholder="Optional note sent with your decision" value={note} onChange={(e) => setNote(e.target.value)} sx={{ minWidth: 320 }} slotProps={{ input: { "aria-label": "Decision note" } }} /></Row>
      <Card title="Leave requests awaiting approval">
        <Table size="sm" aria-label="Pending leave" data-testid="pending-leave">
          <thead><tr><th>Employee</th><th>Type</th><th>Dates</th><th>Days</th><th>Reason</th><th /></tr></thead>
          <tbody>
            {leave.map((r) => (
              <tr key={r.id} data-testid={`pending-${r.emp_code}`}>
                <td>{r.employee_name}</td><td>{r.type_name}</td><td>{r.from_date}{r.to_date !== r.from_date ? ` → ${r.to_date}` : ""}{r.half_day ? ` (${r.half_day} half)` : ""}</td><td>{r.days}</td><td>{r.reason || "—"}</td>
                <td>{r.can_decide ? <Row><Button size="sm" color="success" onClick={() => decideLeave(r.id, "approve")} data-testid={`approve-${r.emp_code}`}>Approve</Button><Button size="sm" color="danger" variant="outlined" onClick={() => decideLeave(r.id, "reject")}>Reject</Button></Row> : <Typography level="body-xs">Waiting for someone else</Typography>}</td>
              </tr>
            ))}
            {leave.length === 0 && <tr><td colSpan={6}>Nothing waiting.</td></tr>}
          </tbody>
        </Table>
        <LoadOlder next={nextLeave} shown={leave.length} label="Load more requests" testId="leave-more" loadMore={async (c) => {
          const pg = await hr.leaveRequestsPage("team", "pending", Number(c));
          setLeave((l) => [...l, ...pg.items]);
          setNextLeave(pg.next);
        }} />
      </Card>
      <Card title="Attendance regularisations awaiting approval">
        <Table size="sm" aria-label="Pending regularisations">
          <thead><tr><th>Employee</th><th>Date</th><th>Times</th><th>Reason</th><th /></tr></thead>
          <tbody>
            {regs.map((r) => (
              <tr key={r.id}>
                <td>{r.employee_name}</td><td>{r.day}</td><td>{r.in_time} – {r.out_time}</td><td>{r.reason}</td>
                <td>{r.can_decide ? <Row><Button size="sm" color="success" onClick={() => decideReg(r.id, "approve")}>Approve</Button><Button size="sm" color="danger" variant="outlined" onClick={() => decideReg(r.id, "reject")}>Reject</Button></Row> : "—"}</td>
              </tr>
            ))}
            {regs.length === 0 && <tr><td colSpan={5}>Nothing waiting.</td></tr>}
          </tbody>
        </Table>
      </Card>
      <Card title="Recently decided">
        {recent.map((r) => <Row key={r.id} sx={{ alignItems: "center" }}><Typography level="body-sm">{r.employee_name} · {r.type_name} · {r.from_date}</Typography><StatusChip status={r.status} /></Row>)}
        {recent.length === 0 && <Typography level="body-sm">No decisions yet.</Typography>}
      </Card>
    </Box>
  );
}
