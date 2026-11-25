/* eslint-disable @typescript-eslint/no-explicit-any */
import { PersonSelect, PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, FormControl, FormLabel, Input, LinearProgress, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import { ACCENT, progressOf } from "./logic";
import { Empty, ErrorAlert, Section, personName, useErr } from "./ui";
import type { Ctx } from "./ui";

export function PerformanceView({ ctx }: { ctx: Ctx }) {
  return (
    <>
      <Goals ctx={ctx} />
      <Reviews ctx={ctx} />
      <OneOnOnes ctx={ctx} />
      <Feedback ctx={ctx} />
    </>
  );
}

function Goals({ ctx }: { ctx: Ctx }) {
  const [owner, setOwner] = useState(ctx.me.user_id);
  const [goals, setGoals] = useState<any[]>([]);
  const [f, setF] = useState({ title: "", target: "100", visibility: "team", parent_id: "" });
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => setGoals(await api.listGoals(owner))), [owner, run]);
  useEffect(() => { load(); }, [load]);
  const mine = owner === ctx.me.user_id;
  const objectives = goals.filter((g) => g.kind === "objective");
  return (
    <Section title="Goals and OKRs" actions={
      <PersonSelect value={owner === ctx.me.user_id ? "" : owner} known={ctx.people} onChange={(id) => setOwner(id || ctx.me.user_id)} placeholder="My goals" ariaLabel="Goals of" testId="goals-owner" size="sm" />}>
      <ErrorAlert err={err} />
      {goals.length === 0 && <Empty>{mine ? "No goals yet." : "Nothing visible to you for this person."}</Empty>}
      {objectives.map((o) => (
        <Box key={o.id} sx={{ mb: 1.5 }} data-testid="goal">
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
            <Typography level="title-md">{o.title}</Typography>
            <Chip size="sm" variant="soft">{o.visibility}</Chip>
            {mine && <Button size="sm" variant="plain" color="danger" onClick={() => run(async () => { await api.deleteGoal(o.id); await load(); })}>Delete</Button>}
          </Box>
          <LinearProgress determinate value={o.progress} sx={{ my: 0.5 }} />
          {goals.filter((k) => k.parent_id === o.id).map((k) => (
            <Box key={k.id} sx={{ pl: 2, display: "flex", gap: 1, alignItems: "center" }} data-testid="key-result">
              <Typography level="body-sm" sx={{ minWidth: 220 }}>{k.title}</Typography>
              <Typography level="body-xs">{k.current} / {k.target} {k.unit} ({progressOf(k.start_value, k.target, k.current)}%)</Typography>
              {mine && <Input size="sm" type="number" sx={{ width: 90 }} placeholder="Update" onKeyDown={(e) => { if (e.key === "Enter") { const v = Number((e.target as HTMLInputElement).value); run(async () => { await api.saveGoal(k.id, { ...k, current: v, due_date: k.due_date ? String(k.due_date).slice(0, 10) : "" }); await load(); }); } }} />}
            </Box>
          ))}
          {mine && <KeyResultAdd parent={o.id} onAdded={load} />}
        </Box>
      ))}
      {mine && (
        <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap" }}>
          <FormControl><FormLabel>New objective</FormLabel><Input value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} slotProps={{ input: { "data-testid": "goal-title" } }} /></FormControl>
          <FormControl><FormLabel>Visible to</FormLabel><Select value={f.visibility} onChange={(_, v) => setF({ ...f, visibility: v ?? "team" })}><Option value="private">Me and my managers</Option><Option value="team">My team</Option><Option value="company">Everyone</Option></Select></FormControl>
          <Button sx={{ bgcolor: ACCENT }} onClick={() => run(async () => { await api.saveGoal("", { title: f.title, visibility: f.visibility, kind: "objective", target: 100 }); setF({ ...f, title: "" }); await load(); })} data-testid="goal-add">Add objective</Button>
        </Box>
      )}
    </Section>
  );
}

function KeyResultAdd({ parent, onAdded }: { parent: string; onAdded: () => void }) {
  const [t, setT] = useState("");
  const [target, setTarget] = useState("100");
  const { err, run } = useErr();
  return (
    <Box sx={{ pl: 2, display: "flex", gap: 1, mt: 0.5 }}>
      <Input size="sm" placeholder="Add key result" value={t} onChange={(e) => setT(e.target.value)} />
      <Input size="sm" type="number" sx={{ width: 90 }} value={target} onChange={(e) => setTarget(e.target.value)} />
      <Button size="sm" variant="soft" onClick={() => run(async () => { await api.saveGoal("", { title: t, parent_id: parent, target: Number(target), visibility: "team" }); setT(""); onAdded(); })}>Add</Button>
      <ErrorAlert err={err} />
    </Box>
  );
}

function Reviews({ ctx }: { ctx: Ctx }) {
  const [cycles, setCycles] = useState<any[]>([]);
  const [tasks, setTasks] = useState<any[]>([]);
  const [name, setName] = useState("");
  const [dates, setDates] = useState({ s: "", e: "" });
  const [kinds, setKinds] = useState<string[]>(["self", "manager"]);
  const [result, setResult] = useState<any>(null);
  const [calib, setCalib] = useState<{ id: string; rows: any[] } | null>(null);
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => { setCycles(await api.listCycles()); setTasks(await api.myReviews()); }), [run]);
  useEffect(() => { load(); }, [load]);
  return (
    <Section title="Performance reviews">
      <ErrorAlert err={err} />
      {tasks.filter((t) => !t.submitted).map((t) => <ReviewForm key={t.id} t={t} onDone={load} />)}
      {cycles.map((c) => (
        <Box key={c.id} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 0.5 }} data-testid="cycle-row">
          <Typography level="title-sm">{c.name}</Typography>
          <Chip size="sm" variant="soft" color={c.status === "active" ? "success" : "neutral"}>{c.status}</Chip>
          <Typography level="body-xs">{c.submitted}/{c.reviews} submitted</Typography>
          <Button size="sm" variant="outlined" onClick={() => run(async () => setResult(await api.getResult(c.id, ctx.me.user_id).catch((e) => ({ error: e.message }))))}>My result</Button>
          {ctx.me.is_hr && ["draft", "active", "closed"].includes(c.status) && (
            <Button size="sm" onClick={() => run(async () => { await api.setCycleStatus(c.id, { draft: "active", active: "closed", closed: "published" }[c.status as string] as string); await load(); })} data-testid="cycle-advance">
              {{ draft: "Start cycle", active: "Close cycle", closed: "Publish results" }[c.status as string]}
            </Button>
          )}
          {ctx.me.is_hr && c.status !== "draft" && <Button size="sm" variant="outlined" onClick={() => run(async () => setCalib({ id: c.id, rows: await api.calibration(c.id) }))} data-testid="calibration-open">Calibration</Button>}
        </Box>
      ))}
      {result && (result.error ? <Typography level="body-sm" color="warning">{result.error}</Typography> : (
        <Box sx={{ p: 1, bgcolor: "#f3f6f5", borderRadius: "sm", my: 1 }} data-testid="result-box">
          <Typography level="title-sm">Result for {result.subject_name} {result.published ? "" : "(not published yet: manager and peer feedback stay hidden)"}</Typography>
          <Typography level="body-sm">Self: {result.self?.rating ?? "not submitted"}. Manager: {result.manager?.rating ?? "hidden or pending"}. Peer average: {result.peer_avg ?? "hidden or none"}. Final: {result.final_rating ?? "pending"}</Typography>
        </Box>
      ))}
      {calib && (
        <Table size="sm" data-testid="calibration-table" sx={{ my: 1 }}>
          <thead><tr><th>Person</th><th>Self</th><th>Manager</th><th>Peer avg</th><th>Calibrated</th></tr></thead>
          <tbody>{calib.rows.map((r) => (
            <tr key={r.subject_id}><td>{r.name}</td><td>{r.self_rating ?? "-"}</td><td>{r.manager_rating ?? "-"}</td><td>{r.peer_avg ?? "-"}</td>
              <td><Select size="sm" value={r.calibrated_rating ?? null} placeholder="Set" disabled={r.manager_rating == null}
                onChange={(_, v) => v && run(async () => { await api.calibrate(calib.id, r.subject_id, v); setCalib({ id: calib.id, rows: await api.calibration(calib.id) }); })}>
                {[1, 2, 3, 4, 5].map((n) => <Option key={n} value={n}>{n}</Option>)}</Select></td></tr>
          ))}</tbody>
        </Table>
      )}
      {ctx.me.is_hr && (
        <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap", mt: 1 }}>
          <FormControl><FormLabel>New cycle</FormLabel><Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Name, e.g. H1 2027" slotProps={{ input: { "data-testid": "cycle-name" } }} /></FormControl>
          <Input type="date" value={dates.s} onChange={(e) => setDates({ ...dates, s: e.target.value })} />
          <Input type="date" value={dates.e} onChange={(e) => setDates({ ...dates, e: e.target.value })} />
          <Select multiple value={kinds} onChange={(_, v) => setKinds(v)}><Option value="self">Self</Option><Option value="manager">Manager</Option><Option value="peer">Peer (360)</Option></Select>
          <Button sx={{ bgcolor: ACCENT }} onClick={() => run(async () => { await api.saveCycle({ name, start_date: dates.s, end_date: dates.e, kinds }); setName(""); await load(); })} data-testid="cycle-create">Create cycle</Button>
        </Box>
      )}
    </Section>
  );
}

function ReviewForm({ t, onDone }: { t: any; onDone: () => void }) {
  const [rating, setRating] = useState(3);
  const [comment, setComment] = useState("");
  const { err, run } = useErr();
  const scale: { v: number; label: string }[] = t.scale ?? [];
  return (
    <Box sx={{ p: 1.5, border: "1px solid #cfdad7", borderRadius: "md", mb: 1 }} data-testid="review-task">
      <Typography level="title-sm">{t.kind === "self" ? "Your self review" : t.kind === "manager" ? `Review of ${t.subject_name}` : `Peer feedback for ${t.subject_name}`} ({t.cycle_name})</Typography>
      <ErrorAlert err={err} />
      <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap" }}>
        <FormControl><FormLabel>Rating</FormLabel><Select value={rating} onChange={(_, v) => setRating(v ?? 3)}>{scale.map((s) => <Option key={s.v} value={s.v}>{s.v} {s.label}</Option>)}</Select></FormControl>
        <Textarea minRows={2} placeholder={(t.questions ?? [])[0] ?? "Comments"} value={comment} onChange={(e) => setComment(e.target.value)} sx={{ minWidth: 320 }} />
        <Button onClick={() => run(async () => { await api.submitReview(t.id, { rating, comment, answers: [comment] }); onDone(); })} data-testid="review-submit">Submit</Button>
      </Box>
    </Box>
  );
}

function OneOnOnes({ ctx }: { ctx: Ctx }) {
  const [rows, setRows] = useState<any[]>([]);
  const [f, setF] = useState({ with_id: "", meeting_date: new Date().toISOString().slice(0, 10), notes: "" });
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => setRows(await api.listOneOnOnes())), [run]);
  useEffect(() => { load(); }, [load]);
  const counterpart = ctx.people.filter((p) => p.manager_id === ctx.me.user_id || p.id === ctx.people.find((x) => x.id === ctx.me.user_id)?.manager_id);
  return (
    <Section title="1:1 notes (private to the two of you)">
      <ErrorAlert err={err} />
      {rows.map((r) => (
        <Typography key={r.id} level="body-sm" data-testid="oneonone">{String(r.meeting_date).slice(0, 10)} with {r.manager_id === ctx.me.user_id ? r.report_name : r.manager_name}: {r.notes}</Typography>
      ))}
      <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap", mt: 1 }}>
        <Select placeholder="With" value={f.with_id || null} onChange={(_, v) => setF({ ...f, with_id: v ?? "" })} sx={{ minWidth: 180 }}>
          {counterpart.map((p) => <Option key={p.id} value={p.id}>{p.name}</Option>)}
        </Select>
        <Input type="date" value={f.meeting_date} onChange={(e) => setF({ ...f, meeting_date: e.target.value })} />
        <Textarea minRows={1} placeholder="Notes" value={f.notes} onChange={(e) => setF({ ...f, notes: e.target.value })} sx={{ minWidth: 260 }} />
        <Button onClick={() => run(async () => { await api.saveOneOnOne("", f); setF({ ...f, notes: "" }); await load(); })} disabled={counterpart.length === 0}>Save note</Button>
      </Box>
      {counterpart.length === 0 && <Typography level="body-xs">1:1s need a manager or direct reports set in the directory.</Typography>}
    </Section>
  );
}

function Feedback({ ctx }: { ctx: Ctx }) {
  const [give, setGive] = useState<any[]>([]);
  const [recv, setRecv] = useState<any[]>([]);
  const [from, setFrom] = useState<string[]>([]);
  const [prompt, setPrompt] = useState("");
  const [text, setText] = useState<Record<string, string>>({});
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => { setGive(await api.feedbackToGive()); setRecv(await api.feedbackReceived()); }), [run]);
  useEffect(() => { load(); }, [load]);
  return (
    <Section title="Feedback">
      <ErrorAlert err={err} />
      {give.map((g) => (
        <Box key={g.id} sx={{ display: "flex", gap: 1, alignItems: "center", mb: 0.5 }}>
          <Typography level="body-sm">About {g.about_name}: {g.prompt || "Share your feedback"}</Typography>
          <Input size="sm" value={text[g.id] ?? ""} onChange={(e) => setText({ ...text, [g.id]: e.target.value })} />
          <Button size="sm" onClick={() => run(async () => { await api.answerFeedback(g.id, text[g.id] ?? ""); await load(); })}>Send</Button>
          <Button size="sm" variant="plain" onClick={() => run(async () => { await api.answerFeedback(g.id, "", true); await load(); })}>Decline</Button>
        </Box>
      ))}
      {recv.map((r) => <Typography key={r.id} level="body-sm">{r.from_name} on {r.about_name}: {r.response || `(${r.status})`}</Typography>)}
      <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap", mt: 1 }}>
        <FormControl><FormLabel>Ask for feedback about me from</FormLabel><PeopleSelect value={from} known={ctx.people.filter((p) => p.id !== ctx.me.user_id)} onChange={(ids) => setFrom(ids)} ariaLabel="Person" /></FormControl>
        <Input placeholder="Question (optional)" value={prompt} onChange={(e) => setPrompt(e.target.value)} />
        <Button disabled={from.length === 0} onClick={() => run(async () => { await api.requestFeedback({ about_id: ctx.me.user_id, from_ids: from, prompt }); setFrom([]); setPrompt(""); await load(); })}>Request</Button>
      </Box>
      <Typography level="body-xs" sx={{ mt: 0.5 }}>Answers are visible to the person who asked and to HR. {personName(ctx.people, "")}</Typography>
    </Section>
  );
}
