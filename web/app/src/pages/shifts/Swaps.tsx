import { useCallback, useEffect, useState } from "react";
import { Alert, Box, Button, Table, Typography } from "@mui/joy";
import { Card, Dialog, ErrorAlert, Row, StatusChip, useErr } from "../hr/ui";
import { fmtDay, fmtRange } from "../ops/dates";
import { shiftsApi } from "./api";
import type { Swap } from "./api";

export function Swaps() {
  const { err, run } = useErr();
  const [swaps, setSwaps] = useState<Swap[]>([]);
  const [review, setReview] = useState<{ swap: Swap; warnings: string[] } | null>(null);
  const load = useCallback(() => void run(async () => setSwaps(await shiftsApi.swaps())), [run]);
  useEffect(load, [load]);
  const act = (s: Swap, a: "approve" | "decline" | "cancel") => void run(async () => { await shiftsApi.resolveSwap(s.id, a); setReview(null); load(); });
  return (
    <Card title="Shift swap requests">
      <ErrorAlert err={err} />
      {!swaps.length && <Typography level="body-sm">No swap requests. Open one of your shifts in the schedule and choose “Request swap”.</Typography>}
      <Box sx={{ overflowX: "auto" }}>
        {swaps.length > 0 && (
          <Table size="sm" aria-label="Swap requests">
            <thead><tr><th>Shift</th><th>From</th><th>To</th><th>Reason</th><th>Status</th><th /></tr></thead>
            <tbody>
              {swaps.map((s) => (
                <tr key={s.id}>
                  <td>{s.snapshot.date ? fmtDay(s.snapshot.date) : "—"}<br /><Typography level="body-xs">{s.snapshot.start && s.snapshot.end ? fmtRange(s.snapshot.start, s.snapshot.end) : ""} {s.snapshot.role}</Typography></td>
                  <td>{s.from_name}</td><td>{s.to_name}</td><td>{s.reason || "—"}</td><td><StatusChip status={s.status} /></td>
                  <td>
                    <Row>
                      {s.can_resolve && <Button size="sm" onClick={() => void run(async () => setReview({ swap: s, warnings: await shiftsApi.swapWarnings(s.id) }))}>Review</Button>}
                      {s.can_cancel && <Button size="sm" variant="plain" color="neutral" onClick={() => act(s, "cancel")}>Cancel</Button>}
                    </Row>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
      </Box>
      {review && (
        <Dialog open onClose={() => setReview(null)} title="Review swap">
          <Typography level="body-sm">{review.swap.from_name} → {review.swap.to_name}, {review.swap.snapshot.date ? fmtDay(review.swap.snapshot.date) : ""} {review.swap.snapshot.start && review.swap.snapshot.end ? fmtRange(review.swap.snapshot.start, review.swap.snapshot.end) : ""}</Typography>
          {review.warnings.length ? review.warnings.map((w, i) => <Alert key={i} color="warning" variant="soft" sx={{ mt: 1 }}>{w}</Alert>) : <Alert color="success" variant="soft" sx={{ mt: 1 }}>No conflicts or limit warnings.</Alert>}
          <Row sx={{ mt: 2, justifyContent: "flex-end" }}>
            <Button variant="outlined" color="danger" onClick={() => act(review.swap, "decline")}>Decline</Button>
            <Button onClick={() => act(review.swap, "approve")}>Approve and reassign</Button>
          </Row>
        </Dialog>
      )}
    </Card>
  );
}
