/* eslint-disable @typescript-eslint/no-explicit-any */
import { PersonSelect, PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import { Box, Button, Checkbox, Chip, FormControl, FormLabel, Input, Option, Select, Table, Textarea, Typography } from "@mui/joy";
import * as api from "./api";
import { ACCENT, fmtDate } from "./logic";
import { Empty, ErrorAlert, Section, useErr } from "./ui";
import type { Ctx } from "./ui";

export function LearningView({ ctx }: { ctx: Ctx }) {
  const [courses, setCourses] = useState<any[]>([]);
  const [policies, setPolicies] = useState<any[]>([]);
  const [track, setTrack] = useState<{ title: string; rows: any[] } | null>(null);
  const [c, setC] = useState({ title: "", link: "", mandatory: false });
  const [p, setP] = useState({ title: "", link: "", summary: "" });
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => { setCourses(await api.listCourses()); setPolicies(await api.listPolicies()); }), [run]);
  useEffect(() => { load(); }, [load]);
  return (
    <>
      <ErrorAlert err={err} />
      <Section title="Policies to acknowledge">
        {policies.length === 0 && <Empty>No policies published.</Empty>}
        {policies.map((x) => (
          <Box key={x.id} sx={{ display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap", mb: 0.75 }} data-testid="policy-row">
            <Typography level="title-sm">{x.title} (v{x.version})</Typography>
            <Button size="sm" variant="plain" component="a" href={x.link} target="_blank">Read the document</Button>
            {x.acknowledged ? <Chip size="sm" color="success" variant="soft">Acknowledged {fmtDate(x.acked_at)}</Chip> :
              <Button size="sm" sx={{ bgcolor: ACCENT }} onClick={() => run(async () => { await api.acknowledge(x.id); await load(); })} data-testid="policy-ack">I have read and agree</Button>}
            {ctx.me.is_hr && <Button size="sm" variant="outlined" onClick={() => run(async () => setTrack({ title: x.title, rows: await api.policyTracking(x.id) }))}>Who has acknowledged</Button>}
            {ctx.me.is_hr && <Button size="sm" variant="plain" onClick={() => run(async () => { await api.savePolicy(x.id, { title: x.title, link: x.link, summary: x.summary, mandatory: x.mandatory, audience: x.audience ?? [], new_version: true }); await load(); })}>Publish new version</Button>}
          </Box>
        ))}
        {ctx.me.is_hr && (
          <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap", mt: 1 }}>
            <FormControl><FormLabel>New policy</FormLabel><Input value={p.title} onChange={(e) => setP({ ...p, title: e.target.value })} placeholder="Title" slotProps={{ input: { "data-testid": "policy-title" } }} /></FormControl>
            <FormControl><FormLabel>Link (Docs or Drive)</FormLabel><Input value={p.link} onChange={(e) => setP({ ...p, link: e.target.value })} placeholder="/docs/... or https://" slotProps={{ input: { "data-testid": "policy-link" } }} /></FormControl>
            <Button onClick={() => run(async () => { await api.savePolicy("", { ...p, mandatory: true }); setP({ title: "", link: "", summary: "" }); await load(); })} data-testid="policy-add">Publish</Button>
          </Box>
        )}
      </Section>
      <Section title="Course catalogue">
        {courses.length === 0 && <Empty>No courses yet.</Empty>}
        <Table size="sm" data-testid="courses">
          <tbody>
            {courses.map((x) => (
              <tr key={x.id}>
                <td><Typography level="title-sm">{x.title}</Typography>{x.mandatory && <Chip size="sm" color="warning" variant="soft">Mandatory</Chip>}</td>
                <td>{x.link && <Button size="sm" variant="plain" component="a" href={x.link} target="_blank">Open</Button>}</td>
                <td>
                  {!x.my_status && <Button size="sm" onClick={() => run(async () => { await api.enroll(x.id, {}); await load(); })} data-testid="enroll">Enroll</Button>}
                  {x.my_status && x.my_status !== "completed" && <Button size="sm" color="success" variant="soft" onClick={() => run(async () => { await api.courseProgress(x.id, "completed"); await load(); })} data-testid="complete">Mark complete</Button>}
                  {x.my_status === "completed" && <Chip size="sm" color="success">Completed {fmtDate(x.my_completed_at)}</Chip>}
                </td>
                {ctx.me.is_hr && <td>{x.completed}/{x.enrolled} done <Button size="sm" variant="plain" onClick={() => run(async () => setTrack({ title: x.title, rows: await api.courseTracking(x.id) }))}>Track</Button>
                  <Button size="sm" variant="plain" onClick={() => run(async () => { const r = await api.enroll(x.id, { everyone: true }); alert(`${r.enrolled} people enrolled`); await load(); })}>Assign to everyone</Button></td>}
              </tr>
            ))}
          </tbody>
        </Table>
        {ctx.me.is_hr && (
          <Box sx={{ display: "flex", gap: 1, alignItems: "end", flexWrap: "wrap", mt: 1 }}>
            <Input placeholder="Course title" value={c.title} onChange={(e) => setC({ ...c, title: e.target.value })} slotProps={{ input: { "data-testid": "course-title" } }} />
            <Input placeholder="Link to the material" value={c.link} onChange={(e) => setC({ ...c, link: e.target.value })} />
            <Checkbox label="Mandatory" checked={c.mandatory} onChange={(e) => setC({ ...c, mandatory: e.target.checked })} />
            <Button onClick={() => run(async () => { await api.saveCourse("", c); setC({ title: "", link: "", mandatory: false }); await load(); })} data-testid="course-add">Add course</Button>
          </Box>
        )}
      </Section>
      {track && (
        <Section title={`Tracking: ${track.title}`} actions={<Button size="sm" variant="plain" onClick={() => setTrack(null)}>Close</Button>}>
          <Table size="sm" data-testid="tracking">
            <tbody>{track.rows.map((r) => <tr key={r.user_id}><td>{r.name}</td><td>{r.acknowledged === undefined ? r.status : r.acknowledged ? `Acknowledged ${fmtDate(r.acked_at)}` : "Pending"}</td></tr>)}</tbody>
          </Table>
        </Section>
      )}
    </>
  );
}

export function FeedView({ ctx }: { ctx: Ctx }) {
  const [items, setItems] = useState<any[]>([]);
  const [cel, setCel] = useState<any[]>([]);
  const [a, setA] = useState({ title: "", body: "", pinned: false });
  const [k, setK] = useState({ to_id: "", badge: "thanks", message: "" });
  const [bd, setBd] = useState({ month: "1", day: "1" });
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => { setItems(await api.feed()); setCel(await api.celebrations()); }), [run]);
  useEffect(() => { load(); }, [load]);
  return (
    <>
      <ErrorAlert err={err} />
      <Section title="Coming up">
        {cel.length === 0 && <Typography level="body-sm">No birthdays or anniversaries in the next 30 days.</Typography>}
        {cel.map((x, i) => <Typography key={i} level="body-sm" data-testid="celebration">{x.type === "birthday" ? "Birthday" : `${x.years} year anniversary`}: {x.name}, {fmtDate(x.on_date)}</Typography>)}
        <Box sx={{ display: "flex", gap: 1, mt: 1, alignItems: "center" }}>
          <Typography level="body-xs">My birthday (month, day):</Typography>
          <Input size="sm" sx={{ width: 70 }} type="number" value={bd.month} onChange={(e) => setBd({ ...bd, month: e.target.value })} />
          <Input size="sm" sx={{ width: 70 }} type="number" value={bd.day} onChange={(e) => setBd({ ...bd, day: e.target.value })} />
          <Button size="sm" variant="outlined" onClick={() => run(async () => { await api.setBirthday({ month: Number(bd.month), day: Number(bd.day) }); await load(); })}>Save</Button>
        </Box>
      </Section>
      <Section title="Give kudos">
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", alignItems: "end" }}>
          <PersonSelect value={k.to_id || ""} known={ctx.people.filter((p) => p.id !== ctx.me.user_id)} onChange={(id) => setK({ ...k, to_id: id })} placeholder="Who?" ariaLabel="Person" testId={"kudos-to"} />
          <Select value={k.badge} onChange={(_, v) => setK({ ...k, badge: v ?? "thanks" })}>
            {["thanks", "teamwork", "great_work", "innovation", "customer_hero", "above_beyond"].map((b) => <Option key={b} value={b}>{b.replace("_", " ")}</Option>)}
          </Select>
          <Input placeholder="Say why" value={k.message} onChange={(e) => setK({ ...k, message: e.target.value })} sx={{ minWidth: 280 }} slotProps={{ input: { "data-testid": "kudos-msg" } }} />
          <Button sx={{ bgcolor: ACCENT }} onClick={() => run(async () => { await api.giveKudos(k); setK({ ...k, message: "" }); await load(); })} data-testid="kudos-send">Send</Button>
        </Box>
      </Section>
      {ctx.me.is_hr && (
        <Section title="Post an announcement">
          <Box sx={{ display: "grid", gap: 1, maxWidth: 600 }}>
            <Input placeholder="Title" value={a.title} onChange={(e) => setA({ ...a, title: e.target.value })} slotProps={{ input: { "data-testid": "ann-title" } }} />
            <Textarea minRows={2} placeholder="Details" value={a.body} onChange={(e) => setA({ ...a, body: e.target.value })} />
            <Checkbox label="Pin to the top" checked={a.pinned} onChange={(e) => setA({ ...a, pinned: e.target.checked })} />
            <Button sx={{ bgcolor: ACCENT, width: 140 }} onClick={() => run(async () => { await api.postAnnouncement(a); setA({ title: "", body: "", pinned: false }); await load(); })} data-testid="ann-post">Post</Button>
          </Box>
        </Section>
      )}
      <Section title="Company feed">
        {items.length === 0 && <Empty>Nothing here yet.</Empty>}
        {items.map((x) => (
          <Box key={x.type + x.id} sx={{ py: 1, borderBottom: "1px solid #e6efed" }} data-testid="feed-item">
            {x.type === "announcement" ? (
              <>
                <Typography level="title-sm">{x.pinned ? "Pinned: " : ""}{x.title}</Typography>
                <Typography level="body-sm" sx={{ whiteSpace: "pre-wrap" }}>{x.body}</Typography>
                <Typography level="body-xs">{x.from_name}, {fmtDate(x.created_at)}</Typography>
                {ctx.me.is_hr && <Button size="sm" variant="plain" color="danger" onClick={() => run(async () => { await api.deleteAnnouncement(x.id); await load(); })}>Remove</Button>}
              </>
            ) : (
              <>
                <Typography level="body-sm"><b>{x.from_name}</b> gave <b>{x.to_name}</b> kudos ({x.badge.replace("_", " ")})</Typography>
                <Typography level="body-sm">{x.body}</Typography>
              </>
            )}
          </Box>
        ))}
      </Section>
    </>
  );
}

function GuidesEditor({ ctx }: { ctx: Ctx }) {
  const [guides, setGuides] = useState<api.Guide[]>([]);
  const [edit, setEdit] = useState<{ id: string; name: string; stage: string; format: string; questions: string } | null>(null);
  const { err, run } = useErr();
  const load = useCallback(() => run(async () => setGuides(await api.listGuides())), [run]);
  useEffect(() => { if (ctx.me.is_hr) load(); }, [ctx.me.is_hr, load]);
  const save = () => edit && run(async () => {
    await api.saveGuide(edit.id, { name: edit.name, stage: edit.stage, format: edit.format, questions: edit.questions.split("\n").map((t) => t.trim()).filter(Boolean).map((text) => ({ text })) });
    setEdit(null);
    await load();
  });
  return (
    <Box sx={{ mt: 3 }} data-testid="guides-editor">
      <Typography level="title-md">Interview guides</Typography>
      <Typography level="body-xs">Question lists interviewers can pick when they write feedback. Answers are saved on the scorecard.</Typography>
      <ErrorAlert err={err} />
      {guides.length === 0 && !edit && <Typography level="body-sm" sx={{ my: 0.5 }}>No guides yet.</Typography>}
      {guides.map((g) => (
        <Box key={g.id} sx={{ display: "flex", gap: 1, alignItems: "center", my: 0.5, flexWrap: "wrap" }}>
          <Typography level="title-sm">{g.name}</Typography>
          <Chip size="sm" variant="soft">{g.questions.length} questions</Chip>
          {g.format && <Chip size="sm" variant="plain">{g.format}</Chip>}
          <Button size="sm" variant="plain" onClick={() => setEdit({ id: g.id, name: g.name, stage: g.stage, format: g.format, questions: g.questions.map((q) => q.text).join("\n") })}>Edit</Button>
          <Button size="sm" variant="plain" color="danger" onClick={() => run(async () => { await api.deleteGuide(g.id); await load(); })}>Delete</Button>
        </Box>
      ))}
      {edit ? (
        <Box sx={{ display: "grid", gap: 1, maxWidth: 520, mt: 1 }}>
          <FormControl><FormLabel>Guide name</FormLabel><Input value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} /></FormControl>
          <FormControl><FormLabel>Format</FormLabel>
            <Select value={edit.format || null} placeholder="Any" onChange={(_, v) => setEdit({ ...edit, format: v ?? "" })}>{["phone", "video", "onsite"].map((f) => <Option key={f} value={f}>{f}</Option>)}</Select></FormControl>
          <FormControl><FormLabel>Questions (one per line)</FormLabel><Textarea minRows={5} value={edit.questions} onChange={(e) => setEdit({ ...edit, questions: e.target.value })} /></FormControl>
          <Box sx={{ display: "flex", gap: 1 }}><Button size="sm" onClick={save} sx={{ bgcolor: ACCENT }}>Save guide</Button><Button size="sm" variant="plain" onClick={() => setEdit(null)}>Cancel</Button></Box>
        </Box>
      ) : (
        <Button size="sm" variant="outlined" sx={{ mt: 0.5 }} onClick={() => setEdit({ id: "", name: "", stage: "", format: "", questions: "" })} data-testid="guide-new">New guide</Button>
      )}
    </Box>
  );
}

export function SettingsView({ ctx }: { ctx: Ctx }) {
  const [s, setS] = useState<api.Settings | null>(null);
  const [saved, setSaved] = useState(false);
  const { err, run } = useErr();
  useEffect(() => { if (ctx.me.is_hr) run(async () => setS(await api.getSettings())); }, [ctx.me.is_hr, run]);
  if (!ctx.me.is_hr) return <Empty>Talent settings are for HR.</Empty>;
  if (!s) return <ErrorAlert err={err} />;
  const setStage = (i: number, name: string) => setS({ ...s, stages: s.stages.map((x, j) => (j === i ? { ...x, name } : x)) });
  const slug = window.location.host + "/careers/default";
  return (
    <Section title="Talent settings">
      <ErrorAlert err={err} />
      <Typography level="title-sm">Pipeline stages (Hired and Rejected are always available)</Typography>
      {s.stages.map((st, i) => (
        <Box key={st.id} sx={{ display: "flex", gap: 1, my: 0.5 }}>
          <Input size="sm" value={st.name} onChange={(e) => setStage(i, e.target.value)} />
          <Button size="sm" variant="plain" disabled={i === 0} onClick={() => { const a = [...s.stages]; [a[i - 1], a[i]] = [a[i], a[i - 1]]; setS({ ...s, stages: a }); }}>Up</Button>
          <Button size="sm" variant="plain" color="danger" disabled={s.stages.length <= 2} onClick={() => setS({ ...s, stages: s.stages.filter((_, j) => j !== i) })}>Remove</Button>
        </Box>
      ))}
      <Button size="sm" variant="outlined" onClick={() => setS({ ...s, stages: [...s.stages, { id: "s" + Math.random().toString(36).slice(2, 7), name: "New stage" }] })}>Add stage</Button>
      <FormControl sx={{ mt: 2, maxWidth: 420 }}><FormLabel>HR users (besides admins)</FormLabel>
        <PeopleSelect value={s.hr_user_ids} known={ctx.people} onChange={(ids) => setS({ ...s, hr_user_ids: ids })} ariaLabel="Person" /></FormControl>
      <Checkbox sx={{ mt: 2 }} label="Public career page enabled" checked={s.career_enabled} onChange={(e) => setS({ ...s, career_enabled: e.target.checked })} />
      <Typography level="body-xs">Career page: {slug} (replace "default" with your organisation slug)</Typography>
      <FormControl sx={{ mt: 1 }}><FormLabel>Career page title</FormLabel><Input value={s.career_title} onChange={(e) => setS({ ...s, career_title: e.target.value })} /></FormControl>
      <FormControl sx={{ mt: 1 }}><FormLabel>Privacy / consent text shown on the form</FormLabel><Textarea minRows={3} value={s.privacy_text} onChange={(e) => setS({ ...s, privacy_text: e.target.value })} /></FormControl>
      <FormControl sx={{ mt: 1 }}><FormLabel>Offer letter template ({"{{name}} {{job_title}} {{department}} {{salary}} {{start_date}} {{company}} {{sender}}"}; empty = default)</FormLabel>
        <Textarea minRows={6} value={s.offer_template} onChange={(e) => setS({ ...s, offer_template: e.target.value })} /></FormControl>
      <Box sx={{ mt: 1.5, display: "flex", gap: 1, alignItems: "center" }}>
        <Button sx={{ bgcolor: ACCENT }} onClick={() => run(async () => { setS(await api.saveSettings(s)); setSaved(true); })} data-testid="settings-save">Save settings</Button>
        {saved && <Chip color="success" size="sm">Saved</Chip>}
      </Box>
      <GuidesEditor ctx={ctx} />
    </Section>
  );
}
