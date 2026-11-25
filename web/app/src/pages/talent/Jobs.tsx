import { PersonSelect, PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Chip, FormControl, FormLabel, Input, Modal, ModalClose, ModalDialog, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import { ACCENT, JOB_TYPES, funnelPercent, jobActions, jobStatusLabel, salaryRange, sourceLabel } from "./logic";
import { Empty, ErrorAlert, Section, personName, useErr } from "./ui";
import type { Ctx } from "./ui";

export function Jobs({ ctx, onOpen }: { ctx: Ctx; onOpen: (jobId: string) => void }) {
  const [jobs, setJobs] = useState<api.Job[] | null>(null);
  const [edit, setEdit] = useState<Partial<api.Job> | null>(null);
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => setJobs(await api.listJobs())), [run]);
  useEffect(() => {
    load();
  }, [load]);
  const act = (fn: () => Promise<unknown>) => run(async () => { await fn(); await load(); });

  return (
    <Section title="Job openings" actions={<Button size="sm" sx={{ bgcolor: ACCENT }} onClick={() => setEdit({ employment_type: "full_time", currency: "INR", openings: 1, skills: [], team_ids: [] })} data-testid="job-new">New opening</Button>}>
      <ErrorAlert err={err} />
      {jobs && jobs.length === 0 && <Empty>No openings yet. Create the first one.</Empty>}
      {jobs && jobs.length > 0 && (
        <Table size="sm" data-testid="jobs-table" sx={{ "& td": { verticalAlign: "middle" } }}>
          <thead>
            <tr><th>Title</th><th>Status</th><th>Team</th><th>Applicants</th><th style={{ width: 340 }}>Actions</th></tr>
          </thead>
          <tbody>
            {jobs.map((j) => {
              const a = jobActions(j, ctx.me.is_hr);
              const mine = ctx.me.is_hr || j.recruiter_id === ctx.me.user_id || j.hiring_manager_id === ctx.me.user_id || j.created_by === ctx.me.user_id;
              return (
                <tr key={j.id} data-testid="job-row">
                  <td>
                    <Typography level="title-sm">{j.title}</Typography>
                    <Typography level="body-xs">{[j.department, j.location, JOB_TYPES[j.employment_type]].filter(Boolean).join(" · ")} {salaryRange(j) && `· ${salaryRange(j)}`}</Typography>
                  </td>
                  <td><Chip size="sm" color={j.status === "open" ? "success" : j.status === "closed" ? "neutral" : "warning"}>{jobStatusLabel(j)}</Chip></td>
                  <td><Typography level="body-xs">{personName(ctx.people, j.recruiter_id) || "No recruiter"}{j.hiring_manager_id ? ` / ${personName(ctx.people, j.hiring_manager_id)}` : ""}</Typography></td>
                  <td>{j.active_applicants ?? 0} active, {j.hired ?? 0}/{j.openings} hired</td>
                  <td>
                    <Box sx={{ display: "flex", gap: 0.5, flexWrap: "wrap" }}>
                      <Button size="sm" variant="soft" onClick={() => onOpen(j.id)} data-testid="job-pipeline">Pipeline</Button>
                      {mine && <Button size="sm" variant="plain" onClick={() => setEdit(j)}>Edit</Button>}
                      {mine && a.requestApproval && <Button size="sm" variant="outlined" onClick={() => act(() => api.jobAction(j.id, "request-approval"))}>Request approval</Button>}
                      {a.approve && <Button size="sm" color="success" variant="soft" onClick={() => act(() => api.jobAction(j.id, "approve"))} data-testid="job-approve">Approve</Button>}
                      {a.approve && <Button size="sm" color="danger" variant="soft" onClick={() => act(() => api.jobAction(j.id, "reject"))}>Reject</Button>}
                      {mine && a.publish && <Button size="sm" onClick={() => act(() => api.setJobStatus(j.id, "open"))} data-testid="job-publish">Publish</Button>}
                      {mine && a.close && <Button size="sm" variant="outlined" color="neutral" onClick={() => act(() => api.setJobStatus(j.id, "closed"))}>Close</Button>}
                      {mine && a.reopen && <Button size="sm" variant="outlined" onClick={() => act(() => api.setJobStatus(j.id, "open"))}>Reopen</Button>}
                      {mine && j.status === "draft" && (j.total_applicants ?? 0) === 0 && <Button size="sm" variant="plain" color="danger" onClick={() => act(() => api.deleteJob(j.id))}>Delete</Button>}
                    </Box>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </Table>
      )}
      {edit && <JobDialog ctx={ctx} job={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); load(); }} />}
    </Section>
  );
}

function JobDialog({ ctx, job, onClose, onSaved }: { ctx: Ctx; job: Partial<api.Job>; onClose: () => void; onSaved: () => void }) {
  const [f, setF] = useState<Partial<api.Job>>(job);
  const [skills, setSkills] = useState((job.skills ?? []).join(", "));
  const { err, run } = useErr();
  const num = (v: string) => (v === "" ? null : Number(v));
  const save = () =>
    run(async () => {
      await api.saveJob(job.id ?? "", { ...f, skills: skills.split(",").map((s) => s.trim()).filter(Boolean) });
      onSaved();
    });
  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 640, maxWidth: "95vw", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">{job.id ? "Edit opening" : "New opening"}</Typography>
        <ErrorAlert err={err} />
        <Box sx={{ display: "grid", gap: 1.25, gridTemplateColumns: "1fr 1fr" }}>
          <FormControl sx={{ gridColumn: "1 / -1" }}><FormLabel>Title</FormLabel><Input value={f.title ?? ""} onChange={(e) => setF({ ...f, title: e.target.value })} slotProps={{ input: { "data-testid": "job-title" } }} /></FormControl>
          <FormControl><FormLabel>Department</FormLabel><Input value={f.department ?? ""} onChange={(e) => setF({ ...f, department: e.target.value })} /></FormControl>
          <FormControl><FormLabel>Location</FormLabel><Input value={f.location ?? ""} onChange={(e) => setF({ ...f, location: e.target.value })} /></FormControl>
          <FormControl><FormLabel>Type</FormLabel>
            <Select value={f.employment_type ?? "full_time"} onChange={(_, v) => setF({ ...f, employment_type: v ?? "full_time" })}>
              {Object.entries(JOB_TYPES).map(([k, v]) => <Option key={k} value={k}>{v}</Option>)}
            </Select></FormControl>
          <FormControl><FormLabel>Openings</FormLabel><Input type="number" value={f.openings ?? 1} onChange={(e) => setF({ ...f, openings: Number(e.target.value) })} /></FormControl>
          <FormControl><FormLabel>Salary from</FormLabel><Input type="number" value={f.salary_min ?? ""} onChange={(e) => setF({ ...f, salary_min: num(e.target.value) })} /></FormControl>
          <FormControl><FormLabel>Salary to</FormLabel><Input type="number" value={f.salary_max ?? ""} onChange={(e) => setF({ ...f, salary_max: num(e.target.value) })} /></FormControl>
          <FormControl sx={{ gridColumn: "1 / -1" }}><FormLabel>Skills (comma separated)</FormLabel><Input value={skills} onChange={(e) => setSkills(e.target.value)} /></FormControl>
          <FormControl><FormLabel>Recruiter</FormLabel>
            <PersonSelect value={f.recruiter_id ?? ""} known={ctx.people} onChange={(id) => setF({ ...f, recruiter_id: id })} placeholder="Choose" ariaLabel="Person" /></FormControl>
          <FormControl><FormLabel>Hiring manager</FormLabel>
            <PersonSelect value={f.hiring_manager_id ?? ""} known={ctx.people} onChange={(id) => setF({ ...f, hiring_manager_id: id })} placeholder="Choose" ariaLabel="Person" /></FormControl>
          <FormControl sx={{ gridColumn: "1 / -1" }}><FormLabel>Interviewers (can read candidates and score)</FormLabel>
            <PeopleSelect value={f.team_ids ?? []} known={ctx.people} onChange={(ids) => setF({ ...f, team_ids: ids })} ariaLabel="Person" /></FormControl>
          <FormControl sx={{ gridColumn: "1 / -1" }}><FormLabel>Description (shown on the career page)</FormLabel><Textarea minRows={4} value={f.description ?? ""} onChange={(e) => setF({ ...f, description: e.target.value })} slotProps={{ textarea: { "data-testid": "job-desc" } }} /></FormControl>
        </Box>
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>Cancel</Button>
          <Button sx={{ bgcolor: ACCENT }} onClick={save} data-testid="job-save">Save</Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

export function PoolView({ ctx }: { ctx: Ctx }) {
  const [rows, setRows] = useState<any[] | null>(null); // eslint-disable-line @typescript-eslint/no-explicit-any
  const [jobs, setJobs] = useState<api.Job[]>([]);
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => { setRows(await api.listPool()); setJobs((await api.listJobs()).filter((j) => j.status === "open")); }), [run]);
  useEffect(() => { if (ctx.me.is_hr) load(); }, [ctx.me.is_hr, load]);
  if (!ctx.me.is_hr) return <Empty>The talent pool is managed by HR.</Empty>;
  return (
    <Section title="Talent pool">
      <ErrorAlert err={err} />
      {rows && rows.length === 0 && <Empty>No one is in the pool. Add rejected or promising candidates from their profile.</Empty>}
      {rows && rows.length > 0 && (
        <Table size="sm">
          <thead><tr><th>Name</th><th>Email</th><th>Source</th><th>Apply to</th><th /></tr></thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.id}>
                <td>{r.name}</td><td>{r.email}</td><td>{sourceLabel(r.source)}</td>
                <td>
                  <Select size="sm" placeholder="Choose opening" onChange={(_, v) => v && run(async () => { await api.poolApply(r.id, String(v)); await load(); })}>
                    {jobs.map((j) => <Option key={j.id} value={j.id}>{j.title}</Option>)}
                  </Select>
                </td>
                <td><Button size="sm" variant="plain" color="neutral" onClick={() => run(async () => { await api.setPool(r.id, false); await load(); })}>Remove</Button></td>
              </tr>
            ))}
          </tbody>
        </Table>
      )}
    </Section>
  );
}

export function ReportsView({ ctx }: { ctx: Ctx }) {
  const [r, setR] = useState<api.Reports | null>(null);
  const { err, run } = useErr();
  useEffect(() => { if (ctx.me.is_hr) run(async () => setR(await api.getReports())); }, [ctx.me.is_hr, run]);
  if (!ctx.me.is_hr) return <Empty>Recruitment reports are visible to HR.</Empty>;
  const first = r?.funnel[0]?.count ?? 0;
  return (
    <>
      <ErrorAlert err={err} />
      {r && (
        <>
          <Section title="Time to hire">
            <Typography level="h3" data-testid="report-avg">{r.avg_days_to_hire} days on average</Typography>
            {r.time_to_hire.length === 0 ? <Typography level="body-sm">No hires yet.</Typography> : (
              <Table size="sm"><thead><tr><th>Opening</th><th>Hired</th><th>Average days</th></tr></thead>
                <tbody>{r.time_to_hire.map((t) => <tr key={t.job_id}><td>{t.title}</td><td>{t.hired}</td><td>{t.avg_days}</td></tr>)}</tbody></Table>
            )}
          </Section>
          <Section title="Funnel">
            {r.funnel.map((s) => (
              <Box key={s.stage} sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.5 }} data-testid="funnel-row">
                <Typography level="body-sm" sx={{ width: 110 }}>{s.name}</Typography>
                <Box sx={{ flex: 1, bgcolor: "#e6efed", borderRadius: 4, height: 18 }}><Box sx={{ width: `${funnelPercent(s.count, first)}%`, bgcolor: ACCENT, height: 18, borderRadius: 4 }} /></Box>
                <Typography level="body-sm" sx={{ width: 90 }}>{s.count} ({funnelPercent(s.count, first)}%)</Typography>
              </Box>
            ))}
          </Section>
          <Section title="Source quality">
            <Table size="sm"><thead><tr><th>Source</th><th>Applications</th><th>Interviewed</th><th>Hired</th><th>Hire rate</th></tr></thead>
              <tbody>{r.sources.map((s) => <tr key={s.source}><td>{sourceLabel(s.source)}</td><td>{s.applications}</td><td>{s.interviewed}</td><td>{s.hired}</td><td>{s.hire_rate}%</td></tr>)}</tbody></Table>
          </Section>
        </>
      )}
    </>
  );
}
