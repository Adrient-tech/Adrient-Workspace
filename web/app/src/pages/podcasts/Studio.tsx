/** The Write a story studio: my stories, the story editor, voices, schedule, audio and stats. */
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Box } from "@mui/joy";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { EditorContent, useEditor, type Editor } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Underline from "@tiptap/extension-underline";
import Link from "@tiptap/extension-link";
import AddRounded from "@mui/icons-material/AddRounded";
import EditNoteRounded from "@mui/icons-material/EditNoteRounded";
import AutoAwesome from "@mui/icons-material/AutoAwesome";
import ArrowUpwardRounded from "@mui/icons-material/ArrowUpwardRounded";
import ArrowDownwardRounded from "@mui/icons-material/ArrowDownwardRounded";
import PlayArrowRounded from "@mui/icons-material/PlayArrowRounded";
import type { AssistantTool } from "@/lib/ai/types";
import {
  stories, type AudioStatus, type EpisodeDetail, type EpisodeInput, type Episode, type Show, type ShowStats, type VoiceList, type Quota,
} from "./storyApi";
import { Btn, C, Chip, Cover, Empty, ErrorNote, Field, FONT_DISPLAY, IconBtn, LANGUAGES, Modal, Select, Skeleton, TextArea, TextInput, Toggle, ago, errMsg, randomCoverStyle, useToast } from "./ui";
import { charactersIn, htmlToPlain, readingMinutes, textToHtml, toLocalInput, wordCount } from "./studioUtil";
import { fmtLength } from "./playerLogic";
import { usePlayer } from "./player";

const DrivePicker = lazy(() => import("../drive/DrivePicker").then((m) => ({ default: m.DrivePicker })));
const AssistantPanel = lazy(() => import("@/lib/ai/AssistantPanel"));

// ---- my stories ----------------------------------------------------------------------------------

export function WritePage() {
  const nav = useNavigate();
  const toast = useToast();
  const [params] = useSearchParams();
  const [list, setList] = useState<Show[] | null>(null);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(params.get("new") === "1");
  const [quota, setQuota] = useState<{ can_publish: boolean } | null>(null);
  const load = useCallback(() => {
    stories.shows({ mine: true }).then(setList).catch((e) => setError(errMsg(e)));
    stories.quota().then(setQuota).catch(() => undefined);
  }, []);
  useEffect(load, [load]);
  return (
    <Box>
      <Box sx={{ display: "flex", alignItems: "center", gap: 2, mb: 3, flexWrap: "wrap" }}>
        <Box component="h1" sx={{ m: 0, flex: 1, fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: { xs: 26, md: 32 }, letterSpacing: "-0.03em" }}>My stories</Box>
        <Btn variant="primary" onClick={() => setCreating(true)} startIcon={<AddRounded />} testId="write-new" disabled={quota?.can_publish === false} title={quota?.can_publish === false ? "Your organisation has turned off writing stories for your account" : undefined}>Write a story</Btn>
      </Box>
      {error && <ErrorNote message={error} onRetry={load} />}
      {!list && !error && <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))", gap: 2 }}>{[0, 1, 2].map((i) => <Skeleton key={i} h={120} r={18} />)}</Box>}
      {list && list.length === 0 && (
        <Empty icon={<EditNoteRounded fontSize="inherit" />} title="You have not written a story yet" hint="Write the text, pick a voice, and everyone you choose can listen. The audio is made for you from the text." action={<Btn variant="primary" size="lg" onClick={() => setCreating(true)}>Write your first story</Btn>} />
      )}
      <Box sx={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(320px, 1fr))", gap: 2 }}>
        {list?.map((s) => (
          <Box key={s.id} component="button" type="button" data-testid="my-story" onClick={() => nav(`/podcasts/write/${s.id}`)} sx={{ display: "flex", gap: 2, textAlign: "left", background: C.surface, border: `1px solid ${C.line}`, borderRadius: "18px", p: 1.6, cursor: "pointer", fontFamily: "inherit", color: C.ink, "&:hover": { borderColor: C.accent } }}>
            <Cover title={s.title} url={s.cover_url} style={s.cover_style} size={92} radius={14} />
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Box sx={{ fontWeight: 800, fontFamily: FONT_DISPLAY, fontSize: 16, lineHeight: 1.25, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{s.title}</Box>
              <Box sx={{ fontSize: 13, color: C.muted, mt: 0.4 }}>{s.genre} · {s.episodes} {s.episodes === 1 ? "episode" : "episodes"}</Box>
              <Box sx={{ display: "flex", gap: 0.6, mt: 1, flexWrap: "wrap" }}>
                <Chip>{s.published > 0 ? `${s.published} published` : "Draft"}</Chip>
                {s.audience !== "everyone" && <Chip>{s.audience === "group" ? "Group only" : "Department only"}</Chip>}
              </Box>
              <Box sx={{ fontSize: 12, color: C.faint, mt: 0.8 }}>Updated {ago(s.updated_at)}</Box>
            </Box>
          </Box>
        ))}
      </Box>
      {creating && (
        <NewStoryModal
          onClose={() => setCreating(false)}
          onCreated={(s) => { toast.show("Story created"); nav(`/podcasts/write/${s.id}`); }}
        />
      )}
    </Box>
  );
}

function NewStoryModal({ onClose, onCreated }: { onClose: () => void; onCreated: (s: Show) => void }) {
  const [title, setTitle] = useState("");
  const [genre, setGenre] = useState("General");
  const [language, setLanguage] = useState("en");
  const [genres, setGenres] = useState<string[]>(["General"]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => { stories.genres().then(setGenres).catch(() => undefined); }, []);
  const create = async () => {
    setBusy(true);
    setErr("");
    try {
      const s = await stories.createShow({ title, genre, language, cover_style: randomCoverStyle() });
      await stories.createEpisode(s.id, { title: "Episode 1" });
      onCreated(s);
    } catch (e) { setErr(errMsg(e)); setBusy(false); }
  };
  return (
    <Modal title="Write a story" onClose={onClose} footer={<><Btn variant="ghost" onClick={onClose}>Cancel</Btn><Btn variant="primary" onClick={() => void create()} disabled={busy || !title.trim()} testId="create-story">Create story</Btn></>}>
      <Field label="Title"><TextInput autoFocus value={title} onChange={(e) => setTitle(e.target.value)} maxLength={120} placeholder="For example: Your first week" testId="new-title" onKeyDown={(e) => e.key === "Enter" && title.trim() && void create()} /></Field>
      <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
        <Field label="Genre"><Select value={genre} onChange={(e) => setGenre(e.target.value)}>{genres.map((g) => <option key={g}>{g}</option>)}</Select></Field>
        <Field label="Language"><Select value={language} onChange={(e) => setLanguage(e.target.value)}>{LANGUAGES.map((l) => <option key={l.code} value={l.code}>{l.label}</option>)}</Select></Field>
      </Box>
      <Box sx={{ fontSize: 13, color: C.muted }}>You can add more episodes later to make it a series. Nothing is visible to others until you publish.</Box>
      {err && <ErrorNote message={err} />}
    </Modal>
  );
}

// ---- editor page -----------------------------------------------------------------------------------

type Tab = "write" | "settings" | "voice" | "stats";

export function EditorPage() {
  const { showId = "" } = useParams();
  const nav = useNavigate();
  const toast = useToast();
  const [show, setShow] = useState<Show | null>(null);
  const [eps, setEps] = useState<Episode[]>([]);
  const [current, setCurrent] = useState<string>("");
  const [tab, setTab] = useState<Tab>("write");
  const [error, setError] = useState("");
  const [assistant, setAssistant] = useState(false);
  const editorApi = useRef<EpisodeEditorApi | null>(null);

  const load = useCallback(async (select?: string) => {
    try {
      const r = await stories.show(showId);
      setShow(r.show);
      setEps(r.episodes);
      setCurrent((c) => select ?? (r.episodes.some((e) => e.id === c) ? c : r.episodes[0]?.id ?? ""));
    } catch (e) { setError(errMsg(e)); }
  }, [showId]);
  useEffect(() => { void load(); }, [load]);

  if (error) return <ErrorNote message={error === "not found" ? "This story does not exist or you do not have access to it." : error} />;
  if (!show) return <Skeleton h={400} r={20} />;
  if (!show.mine) return <ErrorNote message="Only the author can edit this story." />;

  const addEpisode = async () => {
    try {
      const d = await stories.createEpisode(show.id, { title: `Episode ${eps.length + 1}` });
      await load(d.id);
      setTab("write");
    } catch (e) { toast.show(errMsg(e), "error"); }
  };
  const move = async (i: number, dir: -1 | 1) => {
    const a = eps[i], b = eps[i + dir];
    if (!a || !b) return;
    try {
      await stories.updateEpisode(a.id, { position: b.position });
      await stories.updateEpisode(b.id, { position: a.position });
      await load();
    } catch (e) { toast.show(errMsg(e), "error"); }
  };

  return (
    <Box>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 2, flexWrap: "wrap" }}>
        <Box component="button" type="button" onClick={() => nav("/podcasts/write")} sx={{ border: "none", background: "none", color: C.muted, cursor: "pointer", fontFamily: "inherit", fontSize: 14 }}>← My stories</Box>
        <Box sx={{ flex: 1 }} />
        <Btn variant="soft" onClick={() => setAssistant((a) => !a)} startIcon={<AutoAwesome sx={{ fontSize: 18 }} />} testId="assistant-open" aria-pressed={assistant}>Assistant</Btn>
        <Btn variant="outline" onClick={() => nav(`/podcasts/show/${show.id}`)}>View as listener</Btn>
      </Box>
      <Box sx={{ display: "flex", gap: 2, alignItems: "center", mb: 2.5 }}>
        <Cover title={show.title} url={show.cover_url} style={show.cover_style} size={72} radius={14} />
        <Box sx={{ minWidth: 0 }}>
          <Box component="h1" sx={{ m: 0, fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: { xs: 22, md: 28 }, letterSpacing: "-0.02em", lineHeight: 1.15 }}>{show.title}</Box>
          <Box sx={{ color: C.muted, fontSize: 13.5 }}>{show.genre} · {LANGUAGES.find((l) => l.code === show.language)?.label ?? show.language} · {eps.length} {eps.length === 1 ? "episode" : "episodes"}</Box>
        </Box>
      </Box>
      <Box sx={{ display: "flex", gap: 1, mb: 2.5, flexWrap: "wrap" }}>
        {([["write", "Episodes"], ["settings", "Story settings"], ["voice", "Voice"], ["stats", "Listening stats"]] as [Tab, string][]).map(([k, label]) => (
          <Chip key={k} active={tab === k} onClick={() => setTab(k)} testId={`tab-${k}`}>{label}</Chip>
        ))}
      </Box>

      {tab === "write" && (
        <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "260px minmax(0, 1fr)" }, gap: 3, alignItems: "start" }}>
          <Box sx={{ background: C.surface, border: `1px solid ${C.line}`, borderRadius: "18px", p: 1.2 }}>
            <Box sx={{ display: "flex", alignItems: "center", px: 1, py: 0.5 }}>
              <Box sx={{ flex: 1, fontWeight: 800, fontFamily: FONT_DISPLAY, fontSize: 15 }}>Series</Box>
              <IconBtn label="Add episode" onClick={() => void addEpisode()} size={32} testId="add-episode"><AddRounded /></IconBtn>
            </Box>
            {eps.map((e, i) => (
              <Box key={e.id} data-testid="series-item" onClick={() => setCurrent(e.id)} sx={{ display: "flex", alignItems: "center", gap: 0.8, p: 1, borderRadius: "12px", cursor: "pointer", background: e.id === current ? C.accentSoft : "transparent", "&:hover": { background: e.id === current ? C.accentSoft : C.chip } }}>
                <Box sx={{ width: 22, color: C.faint, fontWeight: 700, fontSize: 13 }}>{i + 1}</Box>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                  <Box sx={{ fontSize: 14, fontWeight: 650, whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{e.title}</Box>
                  <Box sx={{ fontSize: 11.5, color: e.status === "published" ? C.accent : C.muted, textTransform: "capitalize", fontWeight: 650 }}>{e.status}{e.audio_status === "ready" ? " · audio" : ""}</Box>
                </Box>
                <Box sx={{ display: "flex", flexDirection: "column" }}>
                  <IconBtn label="Move up" size={20} disabled={i === 0} onClick={(ev) => { ev.stopPropagation(); void move(i, -1); }}><ArrowUpwardRounded sx={{ fontSize: 14 }} /></IconBtn>
                  <IconBtn label="Move down" size={20} disabled={i === eps.length - 1} onClick={(ev) => { ev.stopPropagation(); void move(i, 1); }}><ArrowDownwardRounded sx={{ fontSize: 14 }} /></IconBtn>
                </Box>
              </Box>
            ))}
          </Box>
          <Box sx={{ minWidth: 0 }}>
            {current ? (
              <EpisodeEditor key={current} id={current} show={show} apiRef={editorApi} onChanged={() => void load()} onDeleted={() => void load()} />
            ) : (
              <Empty title="No episodes yet" action={<Btn variant="primary" onClick={() => void addEpisode()}>Add the first episode</Btn>} />
            )}
          </Box>
        </Box>
      )}
      {tab === "settings" && <SettingsTab show={show} onSaved={setShow} onDeleted={() => nav("/podcasts/write")} />}
      {tab === "voice" && <VoiceTab show={show} onSaved={setShow} eps={eps} />}
      {tab === "stats" && <StatsTab show={show} />}

      {assistant && (
        <Box sx={{ position: "fixed", top: { xs: 0, sm: 65 }, right: 0, bottom: 0, width: { xs: "100%", sm: 400 }, zIndex: 1450, boxShadow: "-8px 0 24px rgba(0,0,0,.12)", background: "#fff" }} data-assistant-slot="podcasts">
          <Suspense fallback={null}>
            <AssistantPanel
              app="podcasts"
              documentId={`story-${show.id}`}
              documentTitle={show.title}
              tools={editorApi.current?.tools ?? []}
              getContext={() => editorApi.current?.context() ?? null}
              systemHint={`The story is called "${show.title}" (genre ${show.genre}, language ${show.language}). It is read aloud by a text-to-speech voice, so write for the ear: short sentences, no tables, no emoji. Mark a speaking character at the start of a paragraph like [Meera] and return to the narrator with [Narrator].`}
              suggestions={["Draft a 3 minute story about our first week at the company", "Improve the flow of this episode", "Write a one-paragraph summary for the show notes", "Add dialogue between two characters"]}
              onInsertText={(md) => editorApi.current?.insert(md)}
              onClose={() => setAssistant(false)}
            />
          </Suspense>
        </Box>
      )}
    </Box>
  );
}

// ---- episode editor ------------------------------------------------------------------------------------

interface EpisodeEditorApi {
  tools: AssistantTool[];
  context: () => { label: string; text: string } | null;
  insert: (md: string) => void;
}

function Toolbar({ editor }: { editor: Editor | null }) {
  if (!editor) return null;
  const B = (label: string, on: () => void, active: boolean, text: string) => (
    <Box key={label} component="button" type="button" aria-label={label} title={label} aria-pressed={active} onMouseDown={(e: React.MouseEvent) => e.preventDefault()} onClick={on}
      sx={{ minWidth: 34, height: 34, border: "none", borderRadius: "8px", background: active ? C.accentSoft : "transparent", color: active ? C.accentDark : C.ink, fontWeight: 700, fontSize: 14, cursor: "pointer", fontFamily: "inherit", "&:hover": { background: C.chip } }}>
      {text}
    </Box>
  );
  const link = () => {
    const url = window.prompt("Link address (https://…)", "https://");
    if (url === null) return;
    if (url === "") editor.chain().focus().unsetLink().run();
    else editor.chain().focus().setLink({ href: url }).run();
  };
  return (
    <Box sx={{ display: "flex", gap: 0.3, flexWrap: "wrap", p: 0.8, borderBottom: `1px solid ${C.line}`, background: "#FBFAF6", borderRadius: "14px 14px 0 0" }} role="toolbar" aria-label="Formatting">
      {B("Bold", () => editor.chain().focus().toggleBold().run(), editor.isActive("bold"), "B")}
      {B("Italic", () => editor.chain().focus().toggleItalic().run(), editor.isActive("italic"), "I")}
      {B("Underline", () => editor.chain().focus().toggleUnderline().run(), editor.isActive("underline"), "U")}
      {B("Heading", () => editor.chain().focus().toggleHeading({ level: 2 }).run(), editor.isActive("heading"), "H")}
      {B("Bulleted list", () => editor.chain().focus().toggleBulletList().run(), editor.isActive("bulletList"), "•")}
      {B("Numbered list", () => editor.chain().focus().toggleOrderedList().run(), editor.isActive("orderedList"), "1.")}
      {B("Quote", () => editor.chain().focus().toggleBlockquote().run(), editor.isActive("blockquote"), "“")}
      {B("Link", link, editor.isActive("link"), "🔗")}
      {B("Character voice", () => editor.chain().focus().insertContent("[Name] ").run(), false, "[ ]")}
      {B("Undo", () => editor.chain().focus().undo().run(), false, "↶")}
      {B("Redo", () => editor.chain().focus().redo().run(), false, "↷")}
    </Box>
  );
}

function EpisodeEditor({ id, show, apiRef, onChanged, onDeleted }: { id: string; show: Show; apiRef: React.MutableRefObject<EpisodeEditorApi | null>; onChanged: () => void; onDeleted: () => void }) {
  const toast = useToast();
  const player = usePlayer();
  const [ep, setEp] = useState<EpisodeDetail | null>(null);
  const [title, setTitle] = useState("");
  const [summary, setSummary] = useState("");
  const [saving, setSaving] = useState<"idle" | "dirty" | "saving" | "saved" | "error">("idle");
  const [plain, setPlain] = useState("");
  const [at, setAt] = useState(toLocalInput());
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [err, setErr] = useState("");
  const pending = useRef<EpisodeInput>({});
  const timer = useRef<ReturnType<typeof setTimeout>>();
  const loaded = useRef(false);

  const flush = useCallback(async () => {
    clearTimeout(timer.current);
    const body = pending.current;
    if (Object.keys(body).length === 0) return;
    pending.current = {};
    setSaving("saving");
    try {
      const d = await stories.updateEpisode(id, body);
      setEp((prev) => (prev ? { ...d, body_html: prev.body_html } : d));
      setSaving("saved");
      if (body.title !== undefined) onChanged();
    } catch (e) { setSaving("error"); setErr(errMsg(e)); pending.current = { ...body, ...pending.current }; }
  }, [id, onChanged]);
  const queue = useCallback((patch: EpisodeInput) => {
    pending.current = { ...pending.current, ...patch };
    setSaving("dirty");
    clearTimeout(timer.current);
    timer.current = setTimeout(() => void flush(), 1200);
  }, [flush]);
  useEffect(() => () => { void flush(); }, [flush]);

  const editor = useEditor({
    extensions: [StarterKit, Underline, Link.configure({ openOnClick: false, autolink: false })],
    content: "",
    editorProps: { attributes: { "aria-label": "Story text", "data-testid": "story-editor", style: "outline:none;min-height:340px;" } },
    onUpdate: ({ editor: ed }) => {
      if (!loaded.current) return;
      const html = ed.getHTML();
      setPlain(htmlToPlain(html));
      queue({ body_html: html });
    },
  });

  useEffect(() => {
    let live = true;
    stories.episode(id).then((d) => {
      if (!live) return;
      setEp(d);
      setTitle(d.title);
      setSummary(d.summary);
      if (d.status === "scheduled" && d.publish_at) setAt(toLocalInput(d.publish_at));
      setPlain(htmlToPlain(d.body_html));
      editor?.commands.setContent(d.body_html || "", false);
      loaded.current = true;
    }).catch((e) => live && setErr(errMsg(e)));
    return () => { live = false; };
  }, [id, editor]);

  // poll while audio is being made
  useEffect(() => {
    if (!ep || !(ep.audio_status === "queued" || ep.audio_status === "generating" || ep.job?.status === "queued" || ep.job?.status === "running")) return;
    const t = setInterval(() => {
      stories.episode(id).then((d) => setEp((prev) => (prev ? { ...d, body_html: prev.body_html } : d))).catch(() => undefined);
    }, 2000);
    return () => clearInterval(t);
  }, [ep, id]);

  // assistant bridge
  useEffect(() => {
    if (!editor) return;
    const setText = (md: string) => {
      const before = editor.getHTML();
      editor.commands.setContent(textToHtml(md), true);
      return () => { editor.commands.setContent(before, true); };
    };
    apiRef.current = {
      context: () => {
        const { from, to } = editor.state.selection;
        const sel = from !== to ? editor.state.doc.textBetween(from, to, "\n") : "";
        return { label: sel ? "Selected text" : `Episode: ${title}`, text: sel || htmlToPlain(editor.getHTML()) };
      },
      insert: (md) => { editor.chain().focus().insertContent(textToHtml(md)).run(); toast.show("Inserted into the story"); },
      tools: [
        { name: "story_read_text", description: "Read the full text of the episode being edited.", risk: "read", parameters: { type: "object", properties: {} }, handler: () => ({ result: { title, text: htmlToPlain(editor.getHTML()) } }) },
        {
          name: "story_replace_text", description: "Replace the whole episode text. Use blank lines between paragraphs and [Name] at the start of a paragraph for a speaking character.", risk: "destructive",
          parameters: { type: "object", properties: { text: { type: "string", description: "The new story text" } }, required: ["text"] },
          handler: (a) => { const undo = setText(String(a.text ?? "")); return { summary: "Replaced the episode text", result: { ok: true }, undo }; },
        },
        {
          name: "story_append_text", description: "Add text at the end of the episode.", risk: "write",
          parameters: { type: "object", properties: { text: { type: "string" } }, required: ["text"] },
          handler: (a) => { const before = editor.getHTML(); editor.commands.insertContentAt(editor.state.doc.content.size, textToHtml(String(a.text ?? ""))); return { summary: "Added text to the episode", result: { ok: true }, undo: () => { editor.commands.setContent(before, true); } }; },
        },
        {
          name: "story_set_summary", description: "Set the short show-notes summary of the episode.", risk: "write",
          parameters: { type: "object", properties: { summary: { type: "string" } }, required: ["summary"] },
          handler: (a) => { const old = summary; const s = String(a.summary ?? "").slice(0, 2000); setSummary(s); queue({ summary: s }); return { summary: "Updated the summary", result: { ok: true }, undo: () => { setSummary(old); queue({ summary: old }); } }; },
        },
      ],
    };
    return () => { apiRef.current = null; };
  }, [editor, title, summary, apiRef, queue, toast]);

  const publish = async (when?: string) => {
    await flush();
    try {
      const d = await stories.publish(id, when ? new Date(when).toISOString() : undefined);
      setEp((prev) => (prev ? { ...d, body_html: prev.body_html } : d));
      toast.show(d.status === "scheduled" ? "Scheduled" : "Published");
      onChanged();
    } catch (e) { toast.show(errMsg(e), "error"); }
  };
  const unpublish = async () => {
    try { const d = await stories.unpublish(id); setEp((prev) => (prev ? { ...d, body_html: prev.body_html } : d)); toast.show("Unpublished"); onChanged(); } catch (e) { toast.show(errMsg(e), "error"); }
  };
  const generate = async (force: boolean) => {
    await flush();
    try {
      await stories.generateAudio(id, force);
      const d = await stories.episode(id);
      setEp((prev) => (prev ? { ...d, body_html: prev.body_html } : d));
      toast.show("Audio is being made. You can keep writing.");
    } catch (e) { toast.show(errMsg(e), "error"); }
  };
  const remove = async () => {
    try { await stories.deleteEpisode(id); setConfirmDelete(false); toast.show("Episode deleted"); onDeleted(); } catch (e) { toast.show(errMsg(e), "error"); }
  };

  if (!ep) return err ? <ErrorNote message={err} /> : <Skeleton h={420} r={18} />;
  const chars = charactersIn(plain);
  const words = wordCount(plain);
  return (
    <Box data-testid="episode-editor">
      <Box sx={{ display: "flex", gap: 1, alignItems: "center", mb: 1.5, flexWrap: "wrap" }}>
        <Chip>{ep.status === "scheduled" && ep.publish_at ? `Scheduled · ${new Date(ep.publish_at).toLocaleString()}` : ep.status[0].toUpperCase() + ep.status.slice(1)}</Chip>
        <Box sx={{ fontSize: 12.5, color: saving === "error" ? C.danger : C.muted }} data-testid="save-state">
          {saving === "saving" ? "Saving…" : saving === "dirty" ? "Unsaved changes" : saving === "saved" ? "All changes saved" : saving === "error" ? `Could not save: ${err}` : ""}
        </Box>
        <Box sx={{ flex: 1 }} />
        <Btn variant="ghost" size="sm" onClick={() => void player.play(id)} startIcon={<PlayArrowRounded sx={{ fontSize: 18 }} />} testId="preview">Listen</Btn>
        <Btn variant="danger" size="sm" onClick={() => setConfirmDelete(true)}>Delete</Btn>
      </Box>
      <Box sx={{ display: "grid", gap: 1.5, gridTemplateColumns: "1fr", mb: 1.5 }}>
        <TextInput value={title} onChange={(e) => { setTitle(e.target.value); queue({ title: e.target.value }); }} placeholder="Episode title" aria-label="Episode title" testId="episode-title" style={{ fontSize: 20, fontWeight: 800, fontFamily: FONT_DISPLAY }} />
      </Box>
      <Box sx={{ border: `1px solid ${C.line}`, borderRadius: "14px", background: C.surface, mb: 1.5 }}>
        <Toolbar editor={editor} />
        <Box sx={{ p: 2.2, fontSize: 17, lineHeight: 1.75, color: C.ink, "& .ProseMirror p": { margin: "0 0 .9em" }, "& .ProseMirror h2": { fontFamily: FONT_DISPLAY, fontSize: 22, margin: "1em 0 .4em" }, "& .ProseMirror blockquote": { borderLeft: `3px solid ${C.line}`, margin: "0 0 1em", paddingLeft: "1em", color: C.muted }, "& .ProseMirror a": { color: C.accent } }}>
          <EditorContent editor={editor} />
        </Box>
        <Box sx={{ px: 2, py: 1, borderTop: `1px solid ${C.line}`, fontSize: 12.5, color: C.muted, display: "flex", gap: 2, flexWrap: "wrap" }}>
          <span>{words} words</span><span>about {Math.max(1, readingMinutes(plain))} min to listen</span><span>{ep.chars} characters of {ep.max_chars} allowed for audio</span>
          {chars.length > 0 && <span>Characters: {chars.join(", ")}</span>}
        </Box>
      </Box>
      <Field label="Summary / show notes" hint="Shown next to the player and in the episode list.">
        <TextArea value={summary} onChange={(e) => { setSummary(e.target.value); queue({ summary: e.target.value }); }} maxLength={2000} style={{ minHeight: 64 }} testId="episode-summary" />
      </Field>

      <AudioPanel ep={ep} show={show} onGenerate={generate} />

      <Box sx={{ background: C.surface, border: `1px solid ${C.line}`, borderRadius: "16px", p: 2, mt: 2 }} data-testid="publish-panel">
        <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 800, mb: 1 }}>Publishing</Box>
        <Box sx={{ display: "flex", gap: 1.2, flexWrap: "wrap", alignItems: "center" }}>
          {ep.status !== "published" && <Btn variant="primary" onClick={() => void publish()} testId="publish" disabled={wordCount(plain) === 0}>Publish now</Btn>}
          {ep.status === "published" && <Btn variant="outline" onClick={() => void unpublish()} testId="unpublish">Unpublish</Btn>}
          {ep.status === "scheduled" && <Btn variant="outline" onClick={() => void unpublish()}>Cancel schedule</Btn>}
          {ep.status !== "published" && (
            <>
              <Box component="input" type="datetime-local" value={at} onChange={(e: React.ChangeEvent<HTMLInputElement>) => setAt(e.target.value)} aria-label="Publish at" sx={{ border: `1px solid ${C.line}`, borderRadius: "10px", padding: "7px 10px", fontFamily: "inherit", fontSize: 14 }} data-testid="schedule-at" />
              <Btn variant="soft" onClick={() => void publish(at)} testId="schedule" disabled={wordCount(plain) === 0}>Schedule</Btn>
            </>
          )}
        </Box>
        <Box sx={{ fontSize: 12.5, color: C.muted, mt: 1 }}>
          Who can listen: {show.audience === "everyone" ? "everyone in the company" : show.audience === "group" ? "a group" : "a department"} (change under Story settings). {show.comments_on ? "Comments are on." : "Comments are off."}
        </Box>
      </Box>

      {confirmDelete && (
        <Modal title="Delete this episode?" onClose={() => setConfirmDelete(false)} width={420} footer={<><Btn variant="ghost" onClick={() => setConfirmDelete(false)}>Keep it</Btn><Btn variant="danger" onClick={() => void remove()} testId="confirm-delete">Delete episode</Btn></>}>
          <Box sx={{ fontSize: 14.5 }}>The text, the audio and the listening history of “{ep.title}” will be removed. This cannot be undone.</Box>
        </Modal>
      )}
    </Box>
  );
}

const AUDIO_LABEL: Record<AudioStatus, string> = { none: "No audio yet", queued: "Waiting in the queue", generating: "Making the audio", ready: "Audio is ready", failed: "Audio failed" };

function AudioPanel({ ep, show, onGenerate }: { ep: EpisodeDetail; show: Show; onGenerate: (force: boolean) => Promise<void> }) {
  const [quota, setQuota] = useState<Quota | null>(null);
  useEffect(() => { stories.quota().then((q) => setQuota(q.quota)).catch(() => undefined); }, [ep.audio_status]);
  const eff = ep.effective;
  const busy = ep.audio_status === "queued" || ep.audio_status === "generating" || ep.job?.status === "queued" || ep.job?.status === "running";
  return (
    <Box sx={{ background: C.surface, border: `1px solid ${C.line}`, borderRadius: "16px", p: 2 }} data-testid="audio-panel">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 0.6 }}>
        <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 800 }}>Audio</Box>
        <Chip sx={{ background: ep.audio_status === "ready" ? C.accentSoft : ep.audio_status === "failed" ? C.dangerSoft : C.chip, color: ep.audio_status === "failed" ? C.danger : C.ink, border: "none" }}>
          <span data-testid="audio-status">{AUDIO_LABEL[ep.audio_status]}</span>
        </Chip>
      </Box>
      {eff.mode === "browser" ? (
        <Box sx={{ fontSize: 14, color: C.muted }}>
          No server voice is set up, so listeners hear this story read aloud by their own browser. It works everywhere with no setup. An administrator can add better voices under Admin → Voices.
        </Box>
      ) : (
        <Box sx={{ fontSize: 14, color: C.muted }}>
          Made from your text by <b style={{ color: C.ink }}>{eff.provider_name}</b>{eff.voice ? <> with voice <b style={{ color: C.ink }}>{eff.voice}</b></> : null}, speed {eff.speed}×.
          {show.language !== "en" && " Check that the chosen voice speaks this story's language."}
        </Box>
      )}
      {ep.job && busy && (
        <Box sx={{ mt: 1 }}>
          <Box sx={{ height: 6, background: C.chip, borderRadius: 3, overflow: "hidden" }}><Box sx={{ width: `${ep.job.progress || 4}%`, height: "100%", background: C.accent, transition: "width .4s" }} /></Box>
          <Box sx={{ fontSize: 12.5, color: C.muted, mt: 0.4 }}>{ep.job.status === "queued" ? "Waiting for a free voice worker…" : `Generating… ${ep.job.progress}%`}</Box>
        </Box>
      )}
      {ep.audio_error && <Box sx={{ mt: 1 }}><ErrorNote message={`Last attempt failed: ${ep.audio_error}`} /></Box>}
      {ep.audio_status === "ready" && (
        <Box sx={{ fontSize: 13, color: C.muted, mt: 0.8 }}>
          Length {fmtLength(ep.duration_ms)}. {ep.audio_stale ? <b style={{ color: C.danger }}>The text or voice changed after this audio was made.</b> : "Matches the current text."}
        </Box>
      )}
      <Box sx={{ display: "flex", gap: 1, mt: 1.4, alignItems: "center", flexWrap: "wrap" }}>
        {eff.mode === "server" && (
          <>
            <Btn variant="soft" onClick={() => void onGenerate(false)} disabled={busy} testId="generate-audio">{ep.audio_status === "ready" && !ep.audio_stale ? "Check audio is up to date" : ep.audio_status === "ready" ? "Update audio" : "Generate audio"}</Btn>
            {ep.audio_status === "ready" && <Btn variant="ghost" size="sm" onClick={() => void onGenerate(true)} disabled={busy}>Regenerate anyway</Btn>}
          </>
        )}
        {quota && eff.mode === "server" && quota.daily_chars > 0 && <Box sx={{ fontSize: 12.5, color: C.muted }}>You have used {quota.used_chars.toLocaleString()} of {quota.daily_chars.toLocaleString()} audio characters in the last 24 hours.</Box>}
      </Box>
    </Box>
  );
}

// ---- settings tab -----------------------------------------------------------------------------------------

function SettingsTab({ show, onSaved, onDeleted }: { show: Show; onSaved: (s: Show) => void; onDeleted: () => void }) {
  const toast = useToast();
  const [f, setF] = useState({ title: show.title, description: show.description, genre: show.genre, language: show.language, audience: show.audience, audience_id: show.audience_id ?? "", comments_on: show.comments_on, allow_download: show.allow_download, cover_style: show.cover_style });
  const [genres, setGenres] = useState<string[]>([]);
  const [auds, setAuds] = useState<{ kind: string; id: string; name: string }[]>([]);
  const [picker, setPicker] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const file = useRef<HTMLInputElement>(null);
  useEffect(() => { stories.genres().then(setGenres).catch(() => undefined); stories.audiences().then(setAuds).catch(() => undefined); }, []);
  const save = async () => {
    setBusy(true);
    try { const s = await stories.updateShow(show.id, { ...f, audience_id: f.audience === "everyone" ? "" : f.audience_id }); onSaved(s); toast.show("Story settings saved"); } catch (e) { toast.show(errMsg(e), "error"); } finally { setBusy(false); }
  };
  const upload = async (blob: Blob) => {
    try { const s = await stories.setCover(show.id, blob); onSaved(s); toast.show("Cover updated"); } catch (e) { toast.show(errMsg(e), "error"); }
  };
  const fromDrive = async (id: string) => {
    setPicker(false);
    try {
      const r = await fetch(`/api/v1/drive/files/${id}/content`, { credentials: "same-origin" });
      if (!r.ok) throw new Error("Could not read that file from Drive");
      await upload(await r.blob());
    } catch (e) { toast.show(errMsg(e), "error"); }
  };
  const options = auds.filter((a) => a.kind === f.audience);
  return (
    <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", md: "280px minmax(0, 1fr)" }, gap: 4, alignItems: "start", maxWidth: 980 }}>
      <Box>
        <Cover title={f.title || show.title} url={show.cover_url} style={f.cover_style} size="100%" radius={20} sx={{ maxWidth: 280 }} />
        <Box sx={{ display: "flex", gap: 1, flexWrap: "wrap", mt: 1.5 }}>
          <Btn size="sm" variant="soft" onClick={() => file.current?.click()} testId="upload-cover">Upload image</Btn>
          <Btn size="sm" variant="soft" onClick={() => setPicker(true)}>From Drive</Btn>
          <Btn size="sm" variant="outline" onClick={() => setF({ ...f, cover_style: randomCoverStyle() })} disabled={!!show.cover_url} title={show.cover_url ? "Remove the image to use a generated cover" : undefined}>Shuffle colours</Btn>
          {show.cover_url && <Btn size="sm" variant="ghost" onClick={async () => { try { onSaved(await stories.clearCover(show.id)); } catch (e) { toast.show(errMsg(e), "error"); } }}>Use generated cover</Btn>}
        </Box>
        <Box sx={{ fontSize: 12.5, color: C.muted, mt: 1 }}>Without an image, the cover is made from the title. PNG, JPEG, WebP or GIF up to 3 MB.</Box>
        <input ref={file} type="file" accept="image/png,image/jpeg,image/webp,image/gif" hidden data-testid="cover-file" onChange={(e) => { const x = e.target.files?.[0]; if (x) void upload(x); e.target.value = ""; }} />
        {picker && <Suspense fallback={null}><DrivePicker mode="file" title="Choose a cover image" confirmLabel="Use as cover" onClose={() => setPicker(false)} onPick={(x) => void fromDrive((x as { id: string }).id)} /></Suspense>}
      </Box>
      <Box>
        <Field label="Title"><TextInput value={f.title} onChange={(e) => setF({ ...f, title: e.target.value })} maxLength={120} testId="settings-title" /></Field>
        <Field label="Description"><TextArea value={f.description} onChange={(e) => setF({ ...f, description: e.target.value })} maxLength={4000} /></Field>
        <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 1.5 }}>
          <Field label="Genre"><Select value={f.genre} onChange={(e) => setF({ ...f, genre: e.target.value })}>{[...new Set([f.genre, ...genres])].map((g) => <option key={g}>{g}</option>)}</Select></Field>
          <Field label="Language"><Select value={f.language} onChange={(e) => setF({ ...f, language: e.target.value })}>{LANGUAGES.map((l) => <option key={l.code} value={l.code}>{l.label}</option>)}</Select></Field>
        </Box>
        <Field label="Who can listen">
          <Box sx={{ display: "grid", gridTemplateColumns: f.audience === "everyone" ? "1fr" : "1fr 1fr", gap: 1.5 }}>
            <Select value={f.audience} onChange={(e) => setF({ ...f, audience: e.target.value as Show["audience"], audience_id: "" })} testId="audience">
              <option value="everyone">Everyone in the company</option>
              <option value="group">A group</option>
              <option value="department">A department (and its sub-departments)</option>
            </Select>
            {f.audience !== "everyone" && (
              <Select value={f.audience_id} onChange={(e) => setF({ ...f, audience_id: e.target.value })} testId="audience-id">
                <option value="">Choose…</option>
                {options.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
              </Select>
            )}
          </Box>
        </Field>
        <Toggle checked={f.comments_on} onChange={(v) => setF({ ...f, comments_on: v })} label="Allow comments" />
        <Toggle checked={f.allow_download} onChange={(v) => setF({ ...f, allow_download: v })} label="Allow listeners to download the audio" />
        <Box sx={{ display: "flex", gap: 1.2, mt: 2 }}>
          <Btn variant="primary" onClick={() => void save()} disabled={busy || !f.title.trim() || (f.audience !== "everyone" && !f.audience_id)} testId="save-settings">Save settings</Btn>
          <Box sx={{ flex: 1 }} />
          <Btn variant="danger" onClick={() => setConfirm(true)}>Delete story</Btn>
        </Box>
      </Box>
      {confirm && (
        <Modal title="Delete this story?" onClose={() => setConfirm(false)} width={430} footer={<><Btn variant="ghost" onClick={() => setConfirm(false)}>Keep it</Btn><Btn variant="danger" onClick={async () => { try { await stories.deleteShow(show.id); onDeleted(); } catch (e) { toast.show(errMsg(e), "error"); } }}>Delete everything</Btn></>}>
          <Box sx={{ fontSize: 14.5 }}>“{show.title}” with all its episodes, audio, comments and listening history will be removed for everyone.</Box>
        </Modal>
      )}
    </Box>
  );
}

// ---- voice tab ----------------------------------------------------------------------------------------------

function VoiceTab({ show, onSaved, eps }: { show: Show; onSaved: (s: Show) => void; eps: Episode[] }) {
  const toast = useToast();
  const [providers, setProviders] = useState<VoiceList[] | null>(null);
  const [f, setF] = useState({ provider: show.provider, voice: show.voice, speed: show.speed, pitch: show.pitch, voice_map: { ...show.voice_map } });
  const [characters, setCharacters] = useState<string[]>([]);
  useEffect(() => { stories.voices().then(setProviders).catch((e) => toast.show(errMsg(e), "error")); }, [toast]);
  useEffect(() => {
    (async () => {
      const names = new Set<string>(Object.keys(show.voice_map));
      for (const e of eps.slice(0, 12)) {
        try { const d = await stories.episode(e.id); charactersIn(htmlToPlain(d.body_html)).forEach((n) => names.add(n)); } catch { /* ignore */ }
      }
      setCharacters([...names]);
    })();
  }, [eps, show.voice_map]);
  const active = useMemo(() => {
    if (!providers) return null;
    if (f.provider === "browser") return null;
    if (f.provider === "default") return providers.find((p) => p.is_default) ?? providers[0] ?? null;
    return providers.find((p) => p.provider_id === f.provider) ?? null;
  }, [providers, f.provider]);
  const save = async () => {
    try { const s = await stories.updateShow(show.id, { provider: f.provider, voice: f.voice, speed: f.speed, pitch: f.pitch, voice_map: f.voice_map }); onSaved(s); toast.show("Voice settings saved. Published episodes get new audio."); } catch (e) { toast.show(errMsg(e), "error"); }
  };
  if (!providers) return <Skeleton h={200} r={16} />;
  const voiceSelect = (value: string, onChange: (v: string) => void, none: string) => (
    active && active.voices.length > 0 ? (
      <Select value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">{none}</option>
        {active.voices.map((v) => <option key={v.id} value={v.id}>{v.name}{v.language ? ` (${v.language})` : ""}</option>)}
      </Select>
    ) : (
      <TextInput value={value} onChange={(e) => onChange(e.target.value)} placeholder={none} />
    )
  );
  return (
    <Box sx={{ maxWidth: 760 }} data-testid="voice-tab">
      <Box sx={{ fontSize: 14, color: C.muted, mb: 2 }}>
        The audio is made from your text by a text-to-speech voice. Choose who reads this story. With the browser voice nothing is generated: each listener's device reads the text aloud.
      </Box>
      <Field label="Voice engine">
        <Select value={f.provider} onChange={(e) => setF({ ...f, provider: e.target.value, voice: "" })} testId="voice-provider">
          <option value="default">Company default{providers.find((p) => p.is_default) ? ` (${providers.find((p) => p.is_default)!.name})` : " (browser voice)"}</option>
          {providers.map((p) => <option key={p.provider_id} value={p.provider_id}>{p.name}</option>)}
          <option value="browser">Browser voice (on the listener's device)</option>
        </Select>
      </Field>
      {active?.error && <ErrorNote message={`This engine is not answering: ${active.error}`} />}
      {f.provider !== "browser" && active && (
        <>
          <Field label="Narrator voice" hint={active.voices.length === 0 ? "The engine did not list voices. Type a voice name." : `${active.voices.length} voices available.`}>{voiceSelect(f.voice, (v) => setF({ ...f, voice: v }), `Engine default${active.default_voice ? ` (${active.default_voice})` : ""}`)}</Field>
          <Box sx={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 2 }}>
            <Field label={`Speed ${f.speed.toFixed(2)}×`}><input type="range" min={0.5} max={2} step={0.05} value={f.speed} onChange={(e) => setF({ ...f, speed: Number(e.target.value) })} style={{ width: "100%", accentColor: C.accent }} aria-label="Speed" /></Field>
            <Field label={`Pitch ${f.pitch.toFixed(2)}`} hint="Only some engines support pitch."><input type="range" min={0.5} max={1.5} step={0.05} value={f.pitch} onChange={(e) => setF({ ...f, pitch: Number(e.target.value) })} style={{ width: "100%", accentColor: C.accent }} aria-label="Pitch" /></Field>
          </Box>
          <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 800, mt: 1, mb: 0.5 }}>Character voices</Box>
          {characters.length === 0 ? (
            <Box sx={{ fontSize: 13.5, color: C.muted, mb: 2 }}>Start a paragraph with a name in square brackets, like <code>[Meera] Hello!</code>, and that character can have its own voice. <code>[Narrator]</code> goes back to the narrator.</Box>
          ) : characters.map((c) => (
            <Field key={c} label={c}>{voiceSelect(f.voice_map[c] ?? "", (v) => setF({ ...f, voice_map: { ...f.voice_map, [c]: v } }), "Same as narrator")}</Field>
          ))}
        </>
      )}
      {f.provider === "browser" && <Box sx={{ fontSize: 14, color: C.muted, mb: 2 }}>Listeners need a browser that can read aloud (all current browsers can). Hindi and Tamil depend on the voices installed on their device.</Box>}
      <Btn variant="primary" onClick={() => void save()} testId="save-voice">Save voice settings</Btn>
    </Box>
  );
}

// ---- stats tab -------------------------------------------------------------------------------------------------

function StatsTab({ show }: { show: Show }) {
  const [s, setS] = useState<ShowStats | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => { stories.stats(show.id).then(setS).catch((e) => setErr(errMsg(e))); }, [show.id]);
  if (err) return <ErrorNote message={err} />;
  if (!s) return <Skeleton h={200} r={16} />;
  const max = Math.max(1, ...s.daily.map((d) => d.plays));
  const tile = (label: string, v: string) => (
    <Box sx={{ background: C.surface, border: `1px solid ${C.line}`, borderRadius: "16px", p: 2 }}>
      <Box sx={{ fontSize: 12.5, color: C.muted, fontWeight: 650 }}>{label}</Box>
      <Box sx={{ fontFamily: FONT_DISPLAY, fontWeight: 800, fontSize: 28 }}>{v}</Box>
    </Box>
  );
  return (
    <Box data-testid="stats-tab">
      <Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr 1fr", md: "repeat(4, 1fr)" }, gap: 2, mb: 3 }}>
        {tile("Plays", String(s.plays))}{tile("Listeners", String(s.listeners))}{tile("Completion rate", `${Math.round(s.completion_rate * 100)}%`)}{tile("Followers", String(s.followers))}
      </Box>
      <Box sx={{ background: C.surface, border: `1px solid ${C.line}`, borderRadius: "16px", p: 2, mb: 3 }}>
        <Box sx={{ fontWeight: 800, fontFamily: FONT_DISPLAY, mb: 1 }}>Plays in the last 14 days</Box>
        <Box sx={{ display: "flex", alignItems: "flex-end", gap: 0.6, height: 90 }}>
          {s.daily.map((d) => <Box key={d.day} title={`${d.day}: ${d.plays}`} sx={{ flex: 1, height: `${Math.max(3, (d.plays / max) * 100)}%`, background: d.plays ? C.accent : C.line, borderRadius: "4px 4px 0 0" }} />)}
        </Box>
      </Box>
      <Box sx={{ background: C.surface, border: `1px solid ${C.line}`, borderRadius: "16px", p: 2, overflowX: "auto" }}>
        <Box component="table" sx={{ width: "100%", borderCollapse: "collapse", fontSize: 14, "& th": { textAlign: "left", color: C.muted, fontWeight: 650, pb: 1 }, "& td": { py: 0.8, borderTop: `1px solid ${C.line}` } }}>
          <thead><tr><th>Episode</th><th>Plays</th><th>Listeners</th><th>Finished</th><th>Avg. listened</th><th>Likes</th><th>Saves</th></tr></thead>
          <tbody>
            {s.episodes.map((e) => (
              <tr key={e.id}><td>{e.title} <span style={{ color: C.faint }}>({e.status})</span></td><td>{e.plays}</td><td>{e.listeners}</td><td>{Math.round(e.completion_rate * 100)}%</td><td>{Math.round(e.avg_percent * 100)}%</td><td>{e.likes}</td><td>{e.saves}</td></tr>
            ))}
          </tbody>
        </Box>
      </Box>
    </Box>
  );
}
