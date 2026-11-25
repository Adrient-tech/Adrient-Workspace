import { PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Alert, Box, Button, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Sheet, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import { ACCENT, SOURCES, daysInStage, fmtDateTime, groupByStage, localToISO, sourceLabel } from "./logic";
import { Empty, ErrorAlert, personName, useErr } from "./ui";
import type { Ctx } from "./ui";

export function PipelineBoard({ ctx, jobId, onBack }: { ctx: Ctx; jobId: string; onBack: () => void }) {
  const [settings, setSettings] = useState<api.Settings | null>(null);
  const [job, setJob] = useState<api.Job | null>(null);
  const [apps, setApps] = useState<api.Application[]>([]);
  const [open, setOpen] = useState("");
  const [adding, setAdding] = useState(false);
  const [drag, setDrag] = useState("");
  const { err, run } = useErr();
  const load = useCallback(
    () => run(async () => {
      const [s, js, as] = await Promise.all([api.getSettings(), api.listJobs(), api.listApplications(jobId)]);
      setSettings(s);
      setJob(js.find((j) => j.id === jobId) ?? null);
      setApps(as);
    }),
    [jobId, run],
  );
  useEffect(() => {
    load();
  }, [load]);
  const groups = useMemo(() => (settings ? groupByStage(apps, settings.stages) : {}), [apps, settings]);
  const closed = apps.filter((a) => a.status !== "active");
  const lead = !!job && (ctx.me.is_hr || [job.recruiter_id, job.hiring_manager_id, job.created_by].includes(ctx.me.user_id));
  const move = (id: string, stage: string) => run(async () => { await api.moveApplication(id, stage); await load(); });

  return (
    <Box>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5, flexWrap: "wrap" }}>
        <Button size="sm" variant="plain" onClick={onBack}>All openings</Button>
        <Typography level="h3">{job?.title ?? "Pipeline"}</Typography>
        {lead && <Button size="sm" variant="outlined" onClick={() => setAdding(true)} data-testid="add-candidate">Add candidate</Button>}
      </Box>
      <ErrorAlert err={err} />
      {settings && (
        <Box sx={{ display: "flex", gap: 1.5, overflowX: "auto", pb: 1 }} data-testid="pipeline-board">
          {settings.stages.map((s) => (
            <Sheet key={s.id} variant="soft" data-testid={`col-${s.id}`}
              onDragOver={(e) => lead && e.preventDefault()}
              onDrop={() => { if (drag && lead) move(drag, s.id); setDrag(""); }}
              sx={{ minWidth: 250, width: 250, p: 1, borderRadius: "md", flexShrink: 0 }}>
              <Typography level="title-sm" sx={{ mb: 1 }}>{s.name} ({groups[s.id]?.length ?? 0})</Typography>
              {(groups[s.id] ?? []).map((a) => (
                <Sheet key={a.id} variant="outlined" draggable={lead} onDragStart={() => setDrag(a.id)} onClick={() => setOpen(a.id)}
                  data-testid="pipeline-card" sx={{ p: 1, mb: 1, borderRadius: "sm", cursor: "pointer", bgcolor: "#fff" }}>
                  <Typography level="title-sm">{a.candidate_name}</Typography>
                  <Typography level="body-xs" sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }} title={a.candidate_email}>{a.candidate_email}</Typography>
                  <Box sx={{ display: "flex", gap: 0.5, mt: 0.5, flexWrap: "wrap" }}>
                    <Chip size="sm" variant="soft">{sourceLabel(a.source)}</Chip>
                    <Chip size="sm" variant="plain">{daysInStage(a)}d here</Chip>
                    {a.avg_score != null && <Chip size="sm" color="success" variant="soft">{a.avg_score}/5</Chip>}
                    {a.interviews > 0 && <Chip size="sm" color="primary" variant="soft">interview</Chip>}
                  </Box>
                  {lead && (
                    <Select size="sm" placeholder="Move to..." sx={{ mt: 0.75 }} onClick={(e) => e.stopPropagation()}
                      onChange={(_, v) => v && move(a.id, String(v))} slotProps={{ button: { "data-testid": "card-move" } }}>
                      {settings.stages.filter((x) => x.id !== a.stage).map((x) => <Option key={x.id} value={x.id}>{x.name}</Option>)}
                    </Select>
                  )}
                </Sheet>
              ))}
            </Sheet>
          ))}
        </Box>
      )}
      {closed.length > 0 && (
        <Box sx={{ mt: 2 }}>
          <Typography level="title-md">Closed ({closed.length})</Typography>
          {closed.map((a) => (
            <Chip key={a.id} sx={{ m: 0.5, cursor: "pointer" }} color={a.status === "hired" ? "success" : "neutral"} onClick={() => setOpen(a.id)}>
              {a.candidate_name}: {a.status}
            </Chip>
          ))}
        </Box>
      )}
      {open && settings && <CandidateModal ctx={ctx} id={open} stages={settings.stages} lead={lead} onClose={() => { setOpen(""); load(); }} />}
      {adding && <AddCandidate jobId={jobId} onClose={() => setAdding(false)} onSaved={() => { setAdding(false); load(); }} />}
    </Box>
  );
}

function AddCandidate({ jobId, onClose, onSaved }: { jobId: string; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState({ name: "", email: "", phone: "", source: "referral", source_detail: "" });
  const [dup, setDup] = useState("");
  const { err, run } = useErr();
  const check = () => f.email.includes("@") && run(async () => {
    const r = await api.checkDuplicate(f.email);
    setDup(r.duplicate ? `${r.duplicate.name} is already a candidate (${r.duplicate.applications} application${r.duplicate.applications === 1 ? "" : "s"}). Adding will reuse that record.` : "");
  });
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 460, maxWidth: "95vw" }}>
        <ModalClose />
        <Typography level="title-lg">Add candidate</Typography>
        <ErrorAlert err={err} />
        {dup && <Alert color="warning" data-testid="dup-warning">{dup}</Alert>}
        <FormControl><FormLabel>Name</FormLabel><Input value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} slotProps={{ input: { "data-testid": "add-name" } }} /></FormControl>
        <FormControl><FormLabel>Email</FormLabel><Input value={f.email} onChange={(e) => setF({ ...f, email: e.target.value })} onBlur={check} slotProps={{ input: { "data-testid": "add-email" } }} /></FormControl>
        <FormControl><FormLabel>Phone</FormLabel><Input value={f.phone} onChange={(e) => setF({ ...f, phone: e.target.value })} /></FormControl>
        <FormControl><FormLabel>Source</FormLabel>
          <Select value={f.source} onChange={(_, v) => setF({ ...f, source: v ?? "other" })}>{SOURCES.filter((s) => s !== "career_page").map((s) => <Option key={s} value={s}>{sourceLabel(s)}</Option>)}</Select></FormControl>
        <FormControl><FormLabel>Source detail (who referred, which board)</FormLabel><Input value={f.source_detail} onChange={(e) => setF({ ...f, source_detail: e.target.value })} /></FormControl>
        <Button sx={{ bgcolor: ACCENT }} onClick={() => run(async () => { await api.addCandidate({ ...f, job_id: jobId }); onSaved(); })} data-testid="add-save">Add</Button>
      </ModalDialog>
    </Modal>
  );
}

function CandidateModal({ ctx, id, stages, lead, onClose }: { ctx: Ctx; id: string; stages: api.Stage[]; lead: boolean; onClose: () => void }) {
  const [d, setD] = useState<api.AppDetail | null>(null);
  const [note, setNote] = useState("");
  const [msg, setMsg] = useState("");
  const [hired, setHired] = useState<api.HireToHR | null>(null);
  const { err, run } = useErr();
  const reload = useCallback(() => run(async () => setD(await api.getApplication(id))), [id, run]);
  useEffect(() => { reload(); }, [reload]);
  const doit = (fn: () => Promise<unknown>) => run(async () => { await fn(); await reload(); });

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 760, maxWidth: "96vw", maxHeight: "92vh", overflow: "auto" }} data-testid="candidate-modal">
        <ModalClose />
        {d && (
          <>
            <Typography level="title-lg">{d.candidate_name}</Typography>
            <Typography level="body-sm">{d.candidate_email} {d.candidate_phone && `· ${d.candidate_phone}`} · applied for {d.job_title}</Typography>
            <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap", my: 0.5 }}>
              <Chip size="sm">{sourceLabel(d.source)}{d.source_detail ? `: ${d.source_detail}` : ""}</Chip>
              <Chip size="sm" color={d.status === "hired" ? "success" : d.status === "rejected" ? "danger" : "primary"} variant="soft">{d.status === "active" ? stages.find((s) => s.id === d.stage)?.name ?? d.stage : d.status}</Chip>
              {d.consent_at && <Chip size="sm" variant="outlined">Consent given {new Date(d.consent_at).toLocaleDateString()}</Chip>}
              {d.has_resume && <Button size="sm" variant="outlined" component="a" href={api.resumeUrl(d.candidate_id)} data-testid="resume-link">Resume: {d.resume_name}</Button>}
            </Box>
            <ErrorAlert err={err} />
            {msg && <Alert color="success">{msg}</Alert>}
            {hired && (
              <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", my: 0.5 }} data-testid="hire-links">
                {hired.links.map((l) => <Button key={l.href} size="sm" variant="soft" component="a" href={l.href}>{l.label}</Button>)}
                {hired.notes.map((n) => <Typography key={n} level="body-xs">{n}</Typography>)}
              </Box>
            )}
            {d.cover_note && <Typography sx={{ whiteSpace: "pre-wrap" }} level="body-sm">{d.cover_note}</Typography>}
            {lead && d.status === "active" && (
              <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", my: 1 }}>
                <Select size="sm" placeholder="Move to stage" onChange={(_, v) => v && doit(() => api.moveApplication(id, String(v)))}>{stages.filter((s) => s.id !== d.stage).map((s) => <Option key={s.id} value={s.id}>{s.name}</Option>)}</Select>
                <Button size="sm" color="success" onClick={() => doit(() => api.hireApplication(id))} data-testid="hire">Mark hired</Button>
                <Input size="sm" placeholder="Reject reason" value={note} onChange={(e) => setNote(e.target.value)} slotProps={{ input: { "data-testid": "reject-reason" } }} />
                <Button size="sm" color="danger" variant="soft" onClick={() => doit(() => api.rejectApplication(id, note))} data-testid="reject">Reject</Button>
              </Box>
            )}
            {lead && d.status !== "active" && d.status !== "hired" && <Button size="sm" onClick={() => doit(() => api.restoreApplication(id))}>Restore to pipeline</Button>}
            {ctx.me.is_hr && (
              <Box sx={{ display: "flex", gap: 0.75, my: 0.5, flexWrap: "wrap" }}>
                <Button size="sm" variant="outlined" onClick={() => doit(() => api.setPool(d.candidate_id, !d.talent_pool))}>{d.talent_pool ? "Remove from talent pool" : "Add to talent pool"}</Button>
                {(d.status === "hired" || d.offers.some((o) => o.status === "accepted")) && (
                  <Button size="sm" variant="solid" onClick={() => run(async () => { const r = await api.hireToHR(id, true); setHired(r); setMsg(r.created ? `Added to HR as ${r.employee.emp_code} (${r.employee.name}) with an onboarding checklist.` : `Already in HR as ${r.employee.emp_code}; nothing was duplicated.`); })} data-testid="hire-to-hr">Hire into HR</Button>
                )}
                <Button size="sm" variant="plain" color="danger" onClick={() => { if (confirm("Erase this candidate and all their applications?")) run(async () => { await api.eraseCandidate(d.candidate_id); onClose(); }); }}>Erase candidate</Button>
              </Box>
            )}
            <Interviews ctx={ctx} d={d} lead={lead} onChange={reload} />
            <Scorecards d={d} onSaved={reload} />
            {lead && <Offers d={d} onChange={reload} onMsg={setMsg} />}
            <Typography level="title-md" sx={{ mt: 1.5 }}>History</Typography>
            {d.history.map((h, i) => (
              <Typography key={i} level="body-xs">{fmtDateTime(h.at)}: {h.from_stage ? `${h.from_stage} to ${h.to_stage}` : `applied (${h.to_stage})`} {h.by_name && `by ${h.by_name}`} {h.note && `(${h.note})`}</Typography>
            ))}
          </>
        )}
      </ModalDialog>
    </Modal>
  );
}

function Interviews({ ctx, d, lead, onChange }: { ctx: Ctx; d: api.AppDetail; lead: boolean; onChange: () => void }) {
  const [f, setF] = useState({ title: "Interview", start: "", end: "", people: [] as string[] });
  const { err, run } = useErr();
  return (
    <Box sx={{ mt: 1.5 }}>
      <Typography level="title-md">Interviews</Typography>
      {d.interviews.length === 0 && <Typography level="body-sm">None scheduled.</Typography>}
      {d.interviews.map((i) => (
        <Box key={i.id} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }} data-testid="interview-row">
          <Typography level="body-sm">{i.title}, {fmtDateTime(i.starts_at)} ({i.status})</Typography>
          {i.meet_code && <Button size="sm" variant="plain" component="a" href={`/meet/${i.meet_code}`} target="_blank">Join Meet</Button>}
          {i.calendar_event_id && <Button size="sm" variant="plain" component="a" href="/calendar" target="_blank">In Calendar</Button>}
          {!i.mail_sent && i.status === "scheduled" && <Chip size="sm" color="warning" variant="soft">Candidate was not emailed (mail is not configured)</Chip>}
          {lead && i.status === "scheduled" && <Button size="sm" variant="plain" color="danger" onClick={() => run(async () => { await api.cancelInterview(i.id); onChange(); })}>Cancel</Button>}
        </Box>
      ))}
      {lead && d.status === "active" && (
        <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mt: 0.75, alignItems: "end" }}>
          <ErrorAlert err={err} />
          <FormControl><FormLabel>Title</FormLabel><Input size="sm" value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} /></FormControl>
          <FormControl><FormLabel>Start</FormLabel><Input size="sm" type="datetime-local" value={f.start} onChange={(e) => setF({ ...f, start: e.target.value })} slotProps={{ input: { "data-testid": "iv-start" } }} /></FormControl>
          <FormControl><FormLabel>End</FormLabel><Input size="sm" type="datetime-local" value={f.end} onChange={(e) => setF({ ...f, end: e.target.value })} slotProps={{ input: { "data-testid": "iv-end" } }} /></FormControl>
          <FormControl><FormLabel>Interviewers</FormLabel>
            <PeopleSelect value={f.people} known={ctx.people} onChange={(ids) => setF({ ...f, people: ids })} ariaLabel="Person" /></FormControl>
          <Button size="sm" onClick={() => run(async () => { await api.scheduleInterview(d.id, { title: f.title, starts_at: localToISO(f.start), ends_at: localToISO(f.end), interviewer_ids: f.people }); onChange(); })} data-testid="iv-schedule">Schedule (creates Calendar event + Meet link)</Button>
        </Box>
      )}
    </Box>
  );
}

function Scorecards({ d, onSaved }: { d: api.AppDetail; onSaved: () => void }) {
  const [overall, setOverall] = useState(3);
  const [rec, setRec] = useState("yes");
  const [comment, setComment] = useState("");
  const [guides, setGuides] = useState<api.Guide[]>([]);
  const [guideId, setGuideId] = useState("");
  const [answers, setAnswers] = useState<Record<number, string>>({});
  const { err, run } = useErr();
  useEffect(() => { api.listGuides().then(setGuides).catch(() => setGuides([])); }, []);
  const guide = guides.find((g) => g.id === guideId);
  return (
    <Box sx={{ mt: 1.5 }}>
      <Typography level="title-md">Scorecards</Typography>
      {d.scorecards.map((s) => (
        <Typography key={s.id} level="body-sm" data-testid="scorecard-row">{s.reviewer_name}: {s.overall}/5, {s.recommendation.replace("_", " ")}. {s.comment}
          {(s.answers ?? []).map((x, i) => <Box component="span" key={i} sx={{ display: "block", pl: 1.5 }}><b>{x.question}</b> {x.answer}</Box>)}</Typography>
      ))}
      {d.scorecards.length === 0 && <Typography level="body-sm">No feedback visible yet. Interviewers see others' feedback after submitting their own.</Typography>}
      {d.status === "active" && (
        <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mt: 0.75, alignItems: "end" }}>
          <ErrorAlert err={err} />
          {guides.length > 0 && (
            <FormControl sx={{ minWidth: 200 }}><FormLabel>Interview guide</FormLabel>
              <Select size="sm" placeholder="No guide" value={guideId || null} onChange={(_, v) => { setGuideId(v ?? ""); setAnswers({}); }}>
                {guides.map((g) => <Option key={g.id} value={g.id}>{g.name}</Option>)}
              </Select></FormControl>
          )}
          {guide && (
            <Box sx={{ width: "100%", display: "grid", gap: 0.75 }} data-testid="guide-questions">
              {guide.questions.map((q, i) => (
                <FormControl key={i}><FormLabel>{q.text}{q.dimension ? ` (${q.dimension})` : ""}</FormLabel>
                  <Textarea size="sm" minRows={1} value={answers[i] ?? ""} onChange={(e) => setAnswers({ ...answers, [i]: e.target.value })} /></FormControl>
              ))}
            </Box>
          )}
          <FormControl><FormLabel>Overall (1-5)</FormLabel><Select size="sm" value={overall} onChange={(_, v) => setOverall(v ?? 3)}>{[1, 2, 3, 4, 5].map((n) => <Option key={n} value={n}>{n}</Option>)}</Select></FormControl>
          <FormControl><FormLabel>Recommendation</FormLabel><Select size="sm" value={rec} onChange={(_, v) => setRec(v ?? "yes")}>{["strong_no", "no", "yes", "strong_yes"].map((n) => <Option key={n} value={n}>{n.replace("_", " ")}</Option>)}</Select></FormControl>
          <Textarea size="sm" minRows={1} placeholder="Feedback" value={comment} onChange={(e) => setComment(e.target.value)} sx={{ minWidth: 220 }} />
          <Button size="sm" onClick={() => run(async () => { await api.saveScorecard(d.id, { overall, recommendation: rec, comment, ...(guide ? { guide_id: guide.id, answers: guide.questions.map((q, i) => ({ question: q.text, answer: answers[i] ?? "" })) } : {}) }); onSaved(); })} data-testid="score-save">Save my feedback</Button>
        </Box>
      )}
    </Box>
  );
}

function Offers({ d, onChange, onMsg }: { d: api.AppDetail; onChange: () => void; onMsg: (m: string) => void }) {
  const [salary, setSalary] = useState("");
  const [start, setStart] = useState("");
  const { err, run } = useErr();
  return (
    <Box sx={{ mt: 1.5 }}>
      <Typography level="title-md">Offer</Typography>
      <ErrorAlert err={err} />
      {d.offers.map((o) => (
        <Box key={o.id} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }} data-testid="offer-row">
          <Typography level="body-sm">{o.job_title}: {o.currency} {o.salary.toFixed(2)} ({o.status}{o.responded_name ? `, signed by ${o.responded_name}` : ""})</Typography>
          <Button size="sm" variant="outlined" component="a" href={api.offerPdfUrl(o.id)} target="_blank" data-testid="offer-pdf">PDF</Button>
          {o.status === "draft" && <Button size="sm" onClick={() => run(async () => { const r = await api.sendOffer(o.id); onMsg(r.mail_sent ? "Offer emailed to the candidate." : `Email is not configured on this server. Share this e-accept link: ${r.link}`); onChange(); })} data-testid="offer-send">Send e-accept link</Button>}
          {(o.status === "draft" || o.status === "sent") && <Button size="sm" variant="plain" color="danger" onClick={() => run(async () => { await api.withdrawOffer(o.id); onChange(); })}>Withdraw</Button>}
        </Box>
      ))}
      {d.status === "active" && (
        <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mt: 0.75, alignItems: "end" }}>
          <FormControl><FormLabel>Annual salary</FormLabel><Input size="sm" type="number" value={salary} onChange={(e) => setSalary(e.target.value)} slotProps={{ input: { "data-testid": "offer-salary" } }} /></FormControl>
          <FormControl><FormLabel>Start date</FormLabel><Input size="sm" type="date" value={start} onChange={(e) => setStart(e.target.value)} /></FormControl>
          <Button size="sm" onClick={() => run(async () => { await api.createOffer(d.id, { salary: Number(salary), start_date: start }); onChange(); })} data-testid="offer-create">Create offer letter</Button>
        </Box>
      )}
    </Box>
  );
}

export { Empty, personName };
