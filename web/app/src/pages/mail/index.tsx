import { makeTicket } from "../../lib/makeTicket";
import { makeTask, titleFrom } from "../../lib/makeTask";
import TaskAltIcon from "@mui/icons-material/TaskAlt";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Typography,
  Input,
  IconButton,
  Button,
  Chip,
  CircularProgress,
  Avatar,
  Tooltip,
  Checkbox,
  Dropdown,
  Menu,
  MenuButton,
  MenuItem,
  ListDivider,
  ListItemDecorator,
  Snackbar,
  Drawer,
  ModalClose,
  Sheet,
} from "@mui/joy";
import SearchIcon from "@mui/icons-material/Search";
import EditIcon from "@mui/icons-material/Edit";
import InboxIcon from "@mui/icons-material/Inbox";
import StarIcon from "@mui/icons-material/Star";
import StarBorderIcon from "@mui/icons-material/StarBorder";
import SendIcon from "@mui/icons-material/Send";
import DraftsIcon from "@mui/icons-material/Drafts";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import ReplyIcon from "@mui/icons-material/Reply";
import ForwardIcon from "@mui/icons-material/Forward";
import RefreshIcon from "@mui/icons-material/Refresh";
import AccessTimeIcon from "@mui/icons-material/AccessTime";
import ReportGmailerrorredIcon from "@mui/icons-material/ReportGmailerrorred";
import SettingsIcon from "@mui/icons-material/Settings";
import MarkEmailReadIcon from "@mui/icons-material/MarkEmailRead";
import MarkEmailUnreadIcon from "@mui/icons-material/MarkEmailUnread";
import LabelIcon from "@mui/icons-material/Label";
import LabelOutlinedIcon from "@mui/icons-material/LabelOutlined";
import AddIcon from "@mui/icons-material/Add";
import CloseIcon from "@mui/icons-material/Close";
import MenuIcon from "@mui/icons-material/Menu";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import CodeIcon from "@mui/icons-material/Code";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import BlockIcon from "@mui/icons-material/Block";
import ReportOutlinedIcon from "@mui/icons-material/ReportOutlined";
import FilterAltIcon from "@mui/icons-material/FilterAlt";
import TranslateIcon from "@mui/icons-material/Translate";
import PrintIcon from "@mui/icons-material/Print";
import DownloadIcon from "@mui/icons-material/Download";
import {
  listThreads,
  getThread,
  getMessage,
  getRawSource,
  modifyMessage,
  deleteMessage,
  listLabelsWithEntities,
  sendMessage,
} from "./api";
import type {
  MailSavedSearch,
  MailLabelEntity,
  MailMessage,
  MailThread,
  SendInput,
} from "./types";
import { Compose, type ComposeInit } from "./Compose";
import { AttachmentCard } from "./AttachmentCard";
import { LinkifiedBody } from "./LinkifiedBody";
import { PersonCard } from "../../components/PersonCard";
import { RulesDialog } from "./RulesDialog";
import { LabelManagerDialog } from "./LabelManagerDialog";
import { FiltersDialog } from "./FiltersDialog";
import { MailAssistant } from "./ai/MailAssistant";
import { ChatPanel } from "../chat/ChatPanel";
import ArchiveOutlinedIcon from "@mui/icons-material/ArchiveOutlined";
import PushPinIcon from "@mui/icons-material/PushPin";
import BookmarkBorderIcon from "@mui/icons-material/BookmarkBorder";
import { MailRibbon, type ViewPrefs } from "./MailRibbon";
import { SignaturesDialog } from "./SignaturesDialog";
import { applyQuickFilters, filterThreads, parseSearch, sortThreads, type QuickFilter } from "./search";
import { replyAllRecipients } from "./replyTo";
import { applyFiltersNow, deleteSavedSearch, listMessages, listSavedSearches, listThreads as listThreadsApi, saveSearch } from "./api";
import { playSound } from "../../lib/sounds";

type FolderId =
  | "inbox"
  | "starred"
  | "snoozed"
  | "archive"
  | "sent"
  | "drafts"
  | "spam"
  | "trash";
const FOLDERS: { id: FolderId; name: string; icon: React.ReactNode }[] = [
  { id: "inbox", name: "Inbox", icon: <InboxIcon /> },
  { id: "starred", name: "Starred", icon: <StarIcon /> },
  { id: "snoozed", name: "Snoozed", icon: <AccessTimeIcon /> },
  { id: "archive", name: "Archive", icon: <ArchiveOutlinedIcon /> },
  { id: "sent", name: "Sent", icon: <SendIcon /> },
  { id: "drafts", name: "Drafts", icon: <DraftsIcon /> },
  { id: "spam", name: "Spam", icon: <ReportGmailerrorredIcon /> },
  { id: "trash", name: "Trash", icon: <DeleteOutlineIcon /> },
];

const COLORS = [
  "#3D5A80",
  "#E0777D",
  "#5B9279",
  "#C46B45",
  "#7A5980",
  "#2A9D8F",
  "#D9A441",
  "#1D8348",
];
function colorFor(s: string): string {
  let h = 0;
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0;
  return COLORS[h % COLORS.length];
}
function fmtDate(iso: string): string {
  const d = new Date(iso);
  const now = new Date();
  if (d.toDateString() === now.toDateString())
    return d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  return d.toLocaleDateString([], { month: "short", day: "numeric" });
}

// Snooze presets: label -> a function returning the target Date.
const SNOOZE_PRESETS: { label: string; when: () => Date }[] = [
  {
    label: "Later today (3 hours)",
    when: () => new Date(Date.now() + 3 * 3600_000),
  },
  {
    label: "Tomorrow morning",
    when: () => {
      const d = new Date();
      d.setDate(d.getDate() + 1);
      d.setHours(8, 0, 0, 0);
      return d;
    },
  },
  {
    label: "Next week",
    when: () => {
      const d = new Date();
      d.setDate(d.getDate() + 7);
      d.setHours(8, 0, 0, 0);
      return d;
    },
  },
];

const UNDO_SEND_MS = 5000;

interface MailAppProps {
  user: User;
}

export default function MailApp({ user }: MailAppProps) {
  const [folder, setFolder] = useState<FolderId>("inbox");
  const [labelFilter, setLabelFilter] = useState<string | null>(null);
  const [labels, setLabels] = useState<string[]>([]);
  const [labelEntities, setLabelEntities] = useState<MailLabelEntity[]>([]);
  const [unread, setUnread] = useState<Record<string, number>>({});
  // New-mail tone: while Mail is open, check the inbox count once a minute.
  const inboxSeen = useRef<number | null>(null);
  useEffect(() => {
    const tick = () => {
      if (document.hidden && inboxSeen.current === null) return;
      listThreadsApi("inbox")
        .then((r) => {
          const n = (r.unread ?? {}).inbox ?? 0;
          if (inboxSeen.current !== null && n > inboxSeen.current) playSound("mail");
          inboxSeen.current = n;
        })
        .catch(() => {});
    };
    tick();
    const t = window.setInterval(tick, 60_000);
    return () => window.clearInterval(t);
  }, []);
  const [loading, setLoading] = useState(true);
  // open thread (all messages, oldest first) + the representative summary.
  const [openThread, setOpenThread] = useState<{
    summary: MailThread;
    messages: MailMessage[];
  } | null>(null);
  const [openLoading, setOpenLoading] = useState(false);
  const [compose, setCompose] = useState<ComposeInit | null>(null);
  // PersonCard "Email": /mail?compose=1&to=<addr> on arrival, or an event when Mail is open.
  useEffect(() => {
    const q = new URLSearchParams(window.location.search);
    if (q.get("compose") === "1") {
      setCompose({ to: q.get("to") ?? "", attachDriveIds: q.getAll("attach"), subject: q.get("subject") ?? "", body: q.get("body") ?? "" });
      window.history.replaceState(null, "", window.location.pathname);
    }
    // CRM and other apps deep-link a conversation: /mail?thread=<thread id>.
    const deepThread = q.get("thread");
    if (deepThread) {
      window.history.replaceState(null, "", window.location.pathname);
      void getThread(deepThread).then((msgs) => {
        if (msgs.length === 0) return;
        const latest = msgs[msgs.length - 1];
        setOpenThread({
          summary: { thread_id: deepThread, latest, message_count: msgs.length, any_unread: false, starred: false, labels: [], participants: [] },
          messages: msgs,
        });
      }).catch(() => undefined);
    }
    const onAct = (e: Event) => {
      const d = (e as CustomEvent<{ action?: string; email?: string }>).detail;
      if (d?.action === "email" && d.email) setCompose({ to: d.email });
    };
    window.addEventListener("adrient:person-action", onAct);
    return () => window.removeEventListener("adrient:person-action", onAct);
  }, []);
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const parsed = useMemo(() => parseSearch(search), [search]);
  const [rawThreads, setRawThreads] = useState<MailThread[]>([]);
  const [view, setViewState] = useState<ViewPrefs>(() => {
    try {
      const v = JSON.parse(localStorage.getItem("adrient.mail.view") || "{}");
      return { density: v.density === "compact" ? "compact" : "comfortable", pane: ["right", "bottom", "off"].includes(v.pane) ? v.pane : "right", group: v.group !== false, sort: v.sort || "date-desc" };
    } catch {
      return { density: "comfortable", pane: "right", group: true, sort: "date-desc" };
    }
  });
  const setView = (v: ViewPrefs) => {
    setViewState(v);
    try { localStorage.setItem("adrient.mail.view", JSON.stringify(v)); } catch { /* ignore */ }
  };
  const [quick, setQuick] = useState<Set<QuickFilter>>(new Set());
  const toggleQuick = (f: QuickFilter) =>
    setQuick((q) => {
      const n = new Set(q);
      if (n.has(f)) n.delete(f); else n.add(f);
      return n;
    });
  const setThreads = setRawThreads;
  const isPinned = (t: MailThread) => (t.labels ?? []).includes("Pinned") || (t.latest.labels ?? []).includes("Pinned");
  const threads = useMemo(
    () =>
      sortThreads(
        applyQuickFilters(filterThreads(rawThreads, parsed), quick, user.email ?? "", ""),
        view.sort,
        isPinned,
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [rawThreads, parsed, quick, view.sort, user.email],
  );
  const [spell, setSpellState] = useState(() => localStorage.getItem("adrient.mail.spellcheck") !== "off");
  const setSpell = (b: boolean) => {
    setSpellState(b);
    try { localStorage.setItem("adrient.mail.spellcheck", b ? "on" : "off"); } catch { /* ignore */ }
  };
  const [savedSearches, setSavedSearches] = useState<MailSavedSearch[]>([]);
  const refreshSaved = useCallback(() => {
    listSavedSearches().then(setSavedSearches).catch(() => undefined);
  }, []);
  useEffect(() => { refreshSaved(); }, [refreshSaved]);
  const [sigsOpen, setSigsOpen] = useState(false);
  const [isNarrow, setIsNarrow] = useState(() => window.innerWidth < 900);
  useEffect(() => {
    const f = () => setIsNarrow(window.innerWidth < 900);
    window.addEventListener("resize", f);
    return () => window.removeEventListener("resize", f);
  }, []);
  const split = view.pane !== "off" && !isNarrow;
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [rulesOpen, setRulesOpen] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [labelManagerOpen, setLabelManagerOpen] = useState(false);
  const [filtersOpen, setFiltersOpen] = useState(false);
  // Undo-send toast state. The pending timeout is held in a ref so Undo can cancel it.
  const [undo, setUndo] = useState<{ subject: string } | null>(null);
  const undoTimer = useRef<number | null>(null);
  const undoCancelled = useRef(false);
  // Mobile: sidebar drawer
  const [drawerOpen, setDrawerOpen] = useState(false);

  // Resolve label color: use named entity color if available, otherwise hash-based.
  const labelColor = (name: string): string => {
    const entity = labelEntities.find((l) => l.name === name);
    return entity?.color ?? colorFor(name);
  };

  const effectiveFolder = labelFilter ? "" : folder;

  const reload = useCallback(async () => {
    setLoading(true);
    setOpenThread(null);
    setSelected(new Set());
    try {
      const opts = { query: parsed.text, label: labelFilter ?? undefined };
      const starred = folder === "starred" && !labelFilter;
      const f = starred || labelFilter ? "" : folder;
      let list: MailThread[];
      let unreadMap: Record<string, number>;
      if (view.group) {
        const r = await listThreads(f, starred ? { ...opts, starred: true } : opts);
        list = r.threads ?? [];
        unreadMap = r.unread ?? {};
      } else {
        const r = await listMessages(f, starred ? { ...opts, starred: true } : opts);
        list = (r.messages ?? []).map((m) => ({
          thread_id: m.id, latest: m, message_count: 1, any_unread: !m.is_read, starred: m.starred, labels: m.labels ?? [], participants: [],
        }));
        unreadMap = r.unread ?? {};
      }
      setRawThreads(list);
      setUnread(unreadMap);
    } catch {
      /* ignore */
    } finally {
      setLoading(false);
    }
  }, [folder, parsed.text, labelFilter, view.group]);
  useEffect(() => {
    reload();
  }, [reload]);

  const refreshLabels = useCallback(async () => {
    try {
      const r = await listLabelsWithEntities();
      setLabels(r.labels ?? []);
      setLabelEntities(r.label_objects ?? []);
    } catch {
      /* ignore */
    }
  }, []);
  useEffect(() => {
    refreshLabels();
  }, [refreshLabels]);

  // ---- open / read a thread ----
  async function openThreadFor(t: MailThread) {
    setOpenLoading(true);
    setOpenThread({ summary: t, messages: [] });
    try {
      // getThread is unavailable on the IMAP bridge (no server-side threading);
      // fall back to fetching the single message by id.
      let msgs: MailMessage[] = [];
      try {
        msgs = await getThread(t.thread_id, effectiveFolder || undefined);
      } catch {
        msgs = [];
      }
      const full = msgs.length > 0 ? msgs : [await getMessage(t.latest.id)];
      setOpenThread({ summary: t, messages: full });
      if (t.any_unread) {
        setThreads((cur) =>
          cur.map((x) =>
            x.thread_id === t.thread_id ? { ...x, any_unread: false } : x,
          ),
        );
        setUnread((u) => ({
          ...u,
          [folder]: Math.max(0, (u[folder] || 1) - 1),
        }));
      }
    } catch {
      setOpenThread(null);
    } finally {
      setOpenLoading(false);
    }
  }

  // latest message of the open thread, used for thread-level actions.
  const openLatest = (): MailMessage | null => {
    const t = openThread;
    if (!t || t.messages.length === 0) return null;
    return t.messages[t.messages.length - 1];
  };

  async function toggleStarThread(t: MailThread, e?: React.MouseEvent) {
    e?.stopPropagation();
    const next = !t.starred;
    setThreads((cur) =>
      cur.map((x) =>
        x.thread_id === t.thread_id ? { ...x, starred: next } : x,
      ),
    );
    try {
      await modifyMessage(t.latest.id, {
        is_read: !t.any_unread,
        starred: next,
      });
    } catch {
      reload();
    }
  }
  async function toggleStarMsg(m: MailMessage) {
    const next = !m.starred;
    setOpenThread(
      (cur) =>
        cur && {
          ...cur,
          messages: cur.messages.map((x) =>
            x.id === m.id ? { ...x, starred: next } : x,
          ),
        },
    );
    try {
      await modifyMessage(m.id, { is_read: m.is_read, starred: next });
    } catch {
      /* ignore */
    }
  }

  async function trashThread(t: MailThread) {
    await Promise.all(
      [
        t.latest,
        ...(openThread?.summary.thread_id === t.thread_id
          ? openThread.messages
          : []),
      ]
        .filter((m, i, a) => a.findIndex((x) => x.id === m.id) === i)
        .map((m) =>
          folder === "trash"
            ? deleteMessage(m.id).catch(() => {})
            : modifyMessage(m.id, {
                is_read: true,
                starred: m.starred,
                folder: "trash",
              }).catch(() => {}),
        ),
    );
    setOpenThread(null);
    reload();
  }

  // ---- snooze ----
  async function snooze(ids: string[], until: Date | null) {
    const snooze_until = until ? until.toISOString() : "";
    await Promise.all(
      ids.map((id) =>
        modifyMessage(id, {
          set_snooze: true,
          snooze_until,
          is_read: true,
        }).catch(() => {}),
      ),
    );
    setOpenThread(null);
    setSelected(new Set());
    reload();
  }
  function promptCustomSnooze(ids: string[]) {
    const v = window.prompt("Snooze until (YYYY-MM-DD HH:MM):");
    if (!v) return;
    const d = new Date(v.replace(" ", "T"));
    if (isNaN(d.getTime())) {
      window.alert("Couldn't parse that date/time.");
      return;
    }
    snooze(ids, d);
  }

  // ---- labels ----
  async function applyLabel(m: MailMessage, label: string, add: boolean) {
    const next = add
      ? [...new Set([...m.labels, label])]
      : m.labels.filter((l) => l !== label);
    setOpenThread(
      (cur) =>
        cur && {
          ...cur,
          messages: cur.messages.map((x) =>
            x.id === m.id ? { ...x, labels: next } : x,
          ),
        },
    );
    try {
      await modifyMessage(m.id, {
        is_read: m.is_read,
        starred: m.starred,
        set_labels: true,
        labels: next,
      });
      refreshLabels();
    } catch {
      /* ignore */
    }
  }
  async function createLabel() {
    const name = window.prompt("New label name:");
    if (!name || !name.trim()) return;
    const n = name.trim();
    if (!labels.includes(n))
      setLabels((l) => [...l, n].sort((a, b) => a.localeCompare(b)));
    // Apply to the open message or current selection, if any, so the label persists.
    const m = openLatest();
    if (m) {
      await applyLabel(m, n, true);
    } else if (selected.size > 0) {
      await bulkSet((sm) => ({
        is_read: sm.any_unread === false,
        starred: sm.starred,
        set_labels: true,
        labels: [...new Set([...sm.latest.labels, n])],
      }));
    }
  }

  // ---- bulk selection (operates on thread summaries) ----
  function toggleSel(id: string, e?: React.MouseEvent) {
    e?.stopPropagation();
    setSelected((s) => {
      const n = new Set(s);
      n.has(id) ? n.delete(id) : n.add(id);
      return n;
    });
  }
  const selectedThreads = () =>
    threads.filter((t) => selected.has(t.thread_id));
  async function bulkSet(
    fn: (t: MailThread) => Parameters<typeof modifyMessage>[1],
  ) {
    const ts = selectedThreads();
    setSelected(new Set());
    await Promise.all(
      ts.map((t) => modifyMessage(t.latest.id, fn(t)).catch(() => {})),
    );
    reload();
  }
  function bulkLabel() {
    const l = window.prompt("Label selected conversations as:");
    if (l && l.trim()) {
      bulkSet((t) => ({
        is_read: !t.any_unread,
        starred: t.starred,
        set_labels: true,
        labels: [...new Set([...t.latest.labels, l.trim()])],
      }));
      refreshLabels();
    }
  }
  async function bulkTrash() {
    const ts = selectedThreads();
    setSelected(new Set());
    await Promise.all(
      ts.map((t) =>
        folder === "trash"
          ? deleteMessage(t.latest.id).catch(() => {})
          : modifyMessage(t.latest.id, {
              is_read: true,
              starred: t.starred,
              folder: "trash",
            }).catch(() => {}),
      ),
    );
    reload();
  }
  function bulkSnooze() {
    snooze(
      selectedThreads().map((t) => t.latest.id),
      SNOOZE_PRESETS[0].when(),
    );
  }

  // ---- ribbon actions: operate on the open conversation, else the ticked rows ----
  const ribbonTargets = (): MailThread[] =>
    openThread ? [openThread.summary] : selectedThreads();
  async function ribbonModify(
    fn: (t: MailThread) => Parameters<typeof modifyMessage>[1],
    note?: string,
  ) {
    const ts = ribbonTargets();
    if (ts.length === 0) return;
    await Promise.all(ts.map((t) => modifyMessage(t.latest.id, fn(t)).catch(() => {})));
    setSelected(new Set());
    setOpenThread(null);
    if (note) setNotice(note);
    reload();
  }
  const baseMod = (t: MailThread) => ({ is_read: !t.any_unread, starred: t.starred });
  const ribbonMove = (dest: string, note: string) => ribbonModify((t) => ({ ...baseMod(t), is_read: true, folder: dest }), note);
  function ribbonDelete() {
    const ts = ribbonTargets();
    if (folder === "trash") {
      if (!window.confirm(`Permanently delete ${ts.length} conversation(s)?`)) return;
      void Promise.all(ts.map((t) => deleteMessage(t.latest.id).catch(() => {}))).then(() => {
        setOpenThread(null);
        setSelected(new Set());
        reload();
      });
      return;
    }
    void ribbonMove("trash", "Moved to Trash");
  }
  function ribbonLabel(name: string) {
    void ribbonModify((t) => {
      const cur = t.latest.labels ?? [];
      const next = cur.includes(name) ? cur.filter((l) => l !== name) : [...cur, name];
      return { ...baseMod(t), set_labels: true, labels: next };
    }, `Category “${name}” toggled`);
    refreshLabels();
  }
  function ribbonPin() {
    const all = ribbonTargets();
    const pin = !all.every(isPinned);
    void ribbonModify((t) => {
      const cur = (t.latest.labels ?? []).filter((l) => l !== "Pinned");
      return { ...baseMod(t), set_labels: true, labels: pin ? [...cur, "Pinned"] : cur };
    }, pin ? "Pinned to top" : "Unpinned");
  }
  function ribbonFlag() {
    const ts = ribbonTargets();
    const next = !ts.every((t) => t.starred);
    void ribbonModify((t) => ({ is_read: !t.any_unread, starred: next }), next ? "Flagged" : "Flag removed");
  }
  function ribbonMarkRead() {
    const ts = ribbonTargets();
    const readNow = ts.every((t) => !t.any_unread);
    void ribbonModify((t) => ({ starred: t.starred, is_read: !readNow }), readNow ? "Marked unread" : "Marked read");
  }
  async function ribbonSweep(mode: "sender-archive" | "sender-delete" | "older-delete") {
    const base = ribbonTargets()[0];
    if (!base) return;
    const sender = base.latest.from_addr.toLowerCase();
    const cutoff = Date.now() - 10 * 86400_000;
    const victims = rawThreads.filter((t) =>
      mode === "older-delete" ? new Date(t.latest.sent_at).getTime() < cutoff : t.latest.from_addr.toLowerCase() === sender,
    );
    if (victims.length === 0) { setNotice("Nothing to sweep."); return; }
    const dest = mode === "sender-archive" ? "archive" : "trash";
    if (!window.confirm(`${dest === "archive" ? "Archive" : "Move to Trash"} ${victims.length} conversation(s) in ${folder}?`)) return;
    await Promise.all(victims.map((t) => modifyMessage(t.latest.id, { is_read: true, starred: t.starred, folder: dest }).catch(() => {})));
    setOpenThread(null);
    setNotice(`Swept ${victims.length} conversation(s)`);
    reload();
  }
  async function runRules() {
    try {
      const r = await applyFiltersNow();
      setNotice(`Rules ran: ${r.modified} message(s) changed`);
      reload();
    } catch (e) {
      setNotice(`Could not run rules: ${(e as Error).message}`);
    }
  }
  function ribbonReplyAll() {
    const m = openLatest();
    if (!m) return;
    const { to, cc } = replyAllRecipients(m, user.email ?? "");
    setCompose({
      to: to.join(", "),
      cc: cc.join(", "),
      subject: m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
      body: `\n\n---\nOn ${new Date(m.sent_at).toLocaleString()}, ${m.from_name || m.from_addr} wrote:\n> ${m.body.replace(/\n/g, "\n> ")}`,
    });
  }
  async function ribbonTask() {
    const sm = openThread?.summary;
    if (!sm) return;
    try {
      await makeTask({ title: titleFrom(sm.latest.subject, "Mail follow-up"), notes: `From: ${sm.latest.from_name || sm.latest.from_addr || ""}\n${sm.latest.snippet || ""}`.trim(), link: "/mail" });
      setNotice("Task added to your Inbox list in Tasks.");
    } catch (e) {
      setNotice(`Could not add the task: ${(e as Error).message}`);
    }
  }
  async function ribbonTicket() {
    const m = openLatest();
    if (!m) return;
    try {
      const t = await makeTicket({ source: "mail", message_id: m.id });
      setNotice(`Ticket ${t.ref} created. Open it in Tickets.`);
    } catch (e) {
      setNotice(`Could not create the ticket: ${(e as Error).message}`);
    }
  }
  async function saveCurrentSearch() {
    const q = query.trim();
    if (!q) return;
    const name = window.prompt("Name this search:", q.length > 30 ? q.slice(0, 30) : q);
    if (!name || !name.trim()) return;
    try {
      await saveSearch(name.trim(), q);
      refreshSaved();
      setNotice("Search saved");
    } catch {
      setNotice("Could not save the search.");
    }
  }
  function runSaved(q: string) {
    setLabelFilter(null);
    setFolder("inbox");
    setQuery(q);
    setSearch(q);
    setDrawerOpen(false);
  }

  // ---- undo-send: delay the actual send by UNDO_SEND_MS, show an Undo toast ----
  const deliver = useCallback(
    (input: SendInput): Promise<void> => {
      // Drafts send immediately (no undo window).
      if (input.draft)
        return sendMessage(input).then(() => {
          reload();
        });
      undoCancelled.current = false;
      setUndo({ subject: input.subject || "(no subject)" });
      return new Promise<void>((resolve, reject) => {
        undoTimer.current = window.setTimeout(async () => {
          undoTimer.current = null;
          setUndo(null);
          if (undoCancelled.current) {
            reject(new Error("undo"));
            return;
          }
          try {
            await sendMessage(input);
            reload();
            resolve();
          } catch (e) {
            reject(e as Error);
          }
        }, UNDO_SEND_MS);
      });
    },
    [reload],
  );

  function undoSend() {
    undoCancelled.current = true;
    if (undoTimer.current) {
      window.clearTimeout(undoTimer.current);
      undoTimer.current = null;
    }
    setUndo(null);
  }
  useEffect(
    () => () => {
      if (undoTimer.current) window.clearTimeout(undoTimer.current);
    },
    [],
  );

  function reply(m: MailMessage) {
    setCompose({
      to: m.from_addr,
      subject: m.subject.startsWith("Re:") ? m.subject : `Re: ${m.subject}`,
      body: `\n\n---\nOn ${new Date(m.sent_at).toLocaleString()}, ${m.from_name || m.from_addr} wrote:\n> ${m.body.replace(/\n/g, "\n> ")}`,
    });
  }
  // Per-message ⋮ menu actions (mirrors Gmail's open-message overflow menu).
  async function onMessageAction(action: string, m: MailMessage) {
    const move = async (folder: string, note: string) => {
      await modifyMessage(m.id, {
        is_read: m.is_read,
        starred: m.starred,
        folder,
      }).catch(() => {});
      setNotice(note);
      reload();
    };
    switch (action) {
      case "reply":
        reply(m);
        break;
      case "forward":
        forward(m);
        break;
      case "delete":
        await move("trash", "Moved to Trash");
        break;
      case "unread":
        await modifyMessage(m.id, { is_read: false, starred: m.starred }).catch(
          () => {},
        );
        setOpenThread(null);
        reload();
        break;
      case "spam":
        await move("spam", "Reported spam");
        break;
      case "phishing":
        await move("spam", "Reported phishing");
        break;
      case "block":
        await move("spam", `Blocked ${m.from_name || m.from_addr}`);
        break;
      case "filter":
        setFiltersOpen(true);
        break;
      case "translate":
        setNotice("Translation isn’t available yet.");
        break;
    }
  }

  function forward(m: MailMessage) {
    setCompose({
      subject: m.subject.startsWith("Fwd:") ? m.subject : `Fwd: ${m.subject}`,
      body: `\n\n---------- Forwarded message ----------\nFrom: ${m.from_name || m.from_addr}\nSubject: ${m.subject}\n\n${m.body}`,
    });
  }

  const isSentLike = folder === "sent" || folder === "drafts";
  const peerName = (t: MailThread) =>
    isSentLike
      ? `To: ${t.latest.to_addrs[0] || "(no recipient)"}`
      : t.participants.join(", ") || t.latest.from_addr || "(unknown)";

  const labelMenuItems = (m: MailMessage) => {
    const all = [...new Set([...labels, ...m.labels])].sort((a, b) =>
      a.localeCompare(b),
    );
    if (all.length === 0) return <MenuItem disabled>No labels yet</MenuItem>;
    return all.map((l) => {
      const on = m.labels.includes(l);
      return (
        <MenuItem key={l} onClick={() => applyLabel(m, l, !on)}>
          <Checkbox
            size="sm"
            checked={on}
            sx={{ pointerEvents: "none", mr: 1 }}
          />
          {l}
        </MenuItem>
      );
    });
  };

  // Sidebar content — shared between desktop sidebar and mobile Drawer
  const sidebarContent = (
    <Box sx={{ flex: 1, p: 1.5, overflowY: "auto" }}>
      <Button
        startDecorator={<EditIcon />}
        onClick={() => {
          setCompose({});
          setDrawerOpen(false);
        }}
        sx={{ borderRadius: "xl", px: 2.5, py: 1.25, mb: 2, boxShadow: "sm" }}
      >
        Compose
      </Button>
      {FOLDERS.map((f) => {
        const count =
          f.id === "inbox"
            ? unread.inbox || 0
            : f.id === "snoozed"
              ? unread.snoozed || 0
              : 0;
        const active = !labelFilter && folder === f.id;
        return (
          <Box
            key={f.id}
            onClick={() => {
              setLabelFilter(null);
              setFolder(f.id);
              setDrawerOpen(false);
            }}
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1.5,
              pl: 1.5,
              pr: 1,
              py: 0.75,
              borderRadius: "0 16px 16px 0",
              cursor: "pointer",
              bgcolor: active ? "primary.softBg" : "transparent",
              fontWeight: active ? 700 : 400,
              "&:hover": {
                bgcolor: active ? "primary.softBg" : "background.level1",
              },
            }}
          >
            <Box
              sx={{
                color: active ? "primary.plainColor" : "text.secondary",
                display: "flex",
              }}
            >
              {f.icon}
            </Box>
            <Typography level="body-sm" sx={{ flex: 1, fontWeight: "inherit" }}>
              {f.name}
            </Typography>
            {count > 0 && (
              <Typography level="body-xs" sx={{ fontWeight: 700 }}>
                {count}
              </Typography>
            )}
          </Box>
        );
      })}

      {savedSearches.length > 0 && (
        <>
          <Typography level="body-xs" sx={{ mt: 2, mb: 0.5, pl: 1.5, textTransform: "uppercase", letterSpacing: 0.5, opacity: 0.7 }}>
            Saved searches
          </Typography>
          {savedSearches.map((ss) => (
            <Box
              key={ss.id}
              onClick={() => runSaved(ss.query)}
              title={ss.query}
              sx={{ display: "flex", alignItems: "center", gap: 1, pl: 1.5, pr: 0.5, py: 0.5, borderRadius: "0 16px 16px 0", cursor: "pointer", "&:hover": { bgcolor: "background.level1" } }}
            >
              <SearchIcon sx={{ fontSize: 18, opacity: 0.7 }} />
              <Typography level="body-sm" sx={{ flex: 1 }} noWrap>{ss.name}</Typography>
              <IconButton size="sm" variant="plain" aria-label={`Delete saved search ${ss.name}`} onClick={(e) => { e.stopPropagation(); void deleteSavedSearch(ss.id).then(refreshSaved); }}>
                <CloseIcon sx={{ fontSize: 14 }} />
              </IconButton>
            </Box>
          ))}
        </>
      )}

      {/* Labels section */}
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          mt: 2,
          mb: 0.5,
          pl: 1.5,
          pr: 0.5,
        }}
      >
        <Typography
          level="body-xs"
          sx={{
            flex: 1,
            textTransform: "uppercase",
            letterSpacing: 0.5,
            opacity: 0.7,
          }}
        >
          Labels
        </Typography>
        <Tooltip title="Manage labels">
          <IconButton
            size="sm"
            variant="plain"
            onClick={() => setLabelManagerOpen(true)}
            aria-label="Manage labels"
          >
            <AddIcon sx={{ fontSize: 18 }} />
          </IconButton>
        </Tooltip>
      </Box>
      {labels.length === 0 ? (
        <Typography level="body-xs" sx={{ pl: 1.5, opacity: 0.5 }}>
          No labels yet
        </Typography>
      ) : (
        labels.map((l) => {
          const active = labelFilter === l;
          return (
            <Box
              key={l}
              onClick={() => {
                setLabelFilter(l);
                setOpenThread(null);
                setDrawerOpen(false);
              }}
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 1.5,
                pl: 1.5,
                pr: 1,
                py: 0.5,
                borderRadius: "0 16px 16px 0",
                cursor: "pointer",
                bgcolor: active ? "primary.softBg" : "transparent",
                fontWeight: active ? 700 : 400,
                "&:hover": {
                  bgcolor: active ? "primary.softBg" : "background.level1",
                },
              }}
            >
              <LabelOutlinedIcon sx={{ fontSize: 18, color: labelColor(l) }} />
              <Typography
                level="body-sm"
                sx={{ flex: 1, fontWeight: "inherit" }}
                noWrap
              >
                {l}
              </Typography>
            </Box>
          );
        })
      )}
    </Box>
  );

  return (
    <>
      <Header user={user} />
      <Box
        sx={{
          display: "flex",
          flexDirection: "column",
          height: "calc(100vh - 56px)",
        }}
      >
        {/* Search bar + quick settings */}
        <Box
          sx={{
            p: 1.5,
            borderBottom: "1px solid",
            borderColor: "divider",
            display: "flex",
            alignItems: "center",
            gap: 1,
          }}
        >
          {/* Hamburger: only on mobile */}
          <IconButton
            size="sm"
            variant="plain"
            onClick={() => setDrawerOpen(true)}
            aria-label="Open folders"
            sx={{
              display: { xs: "inline-flex", md: "none" },
              minWidth: 40,
              minHeight: 40,
            }}
          >
            <MenuIcon />
          </IconButton>
          <Input
            size="lg"
            startDecorator={<SearchIcon />}
            placeholder="Search mail  (from:  to:  has:attachment  before:2026-01-31  label:Work  is:unread)"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") setSearch(query);
            }}
            endDecorator={
              query && (
                <>
                  <Tooltip title="Save this search">
                    <IconButton size="sm" variant="plain" onClick={saveCurrentSearch} aria-label="Save search">
                      <BookmarkBorderIcon />
                    </IconButton>
                  </Tooltip>
                  <IconButton
                    size="sm"
                    variant="plain"
                    aria-label="Clear search"
                    onClick={() => {
                      setQuery("");
                      setSearch("");
                    }}
                  >
                    ✕
                  </IconButton>
                </>
              )
            }
            sx={{
              flex: "1 1 720px",
              maxWidth: 720,
              bgcolor: "background.level1",
              borderRadius: "xl",
            }}
          />
          <Box sx={{ flex: 1 }} />
          <MailAssistant
            openThread={openThread}
            openCompose={(init) => setCompose(init)}
            closeCompose={() => setCompose(null)}
            reload={() => void reload()}
          />
          <Dropdown>
            <MenuButton
              slots={{ root: IconButton }}
              slotProps={{
                root: {
                  variant: "plain",
                  color: "neutral",
                  "aria-label": "Settings",
                },
              }}
            >
              <SettingsIcon />
            </MenuButton>
            <Menu placement="bottom-end" sx={{ minWidth: 220 }}>
              <Typography level="title-sm" sx={{ px: 1.5, py: 0.5 }}>
                Quick settings
              </Typography>
              <ListDivider />
              <MenuItem onClick={() => setFiltersOpen(true)}>
                Mail filters
              </MenuItem>
              <MenuItem onClick={() => setRulesOpen(true)}>
                Rules (legacy)
              </MenuItem>
              <MenuItem onClick={() => setLabelManagerOpen(true)}>
                Manage labels
              </MenuItem>
              <MenuItem onClick={() => setSigsOpen(true)}>
                Signatures
              </MenuItem>
            </Menu>
          </Dropdown>
        </Box>

        <MailRibbon
          hasTarget={!!openThread || selected.size > 0}
          canReply={!!openThread && openThread.messages.length > 0}
          folder={folder}
          labels={labels.map((n) => ({ name: n, color: labelColor(n) })).filter((l) => l.name !== "Pinned")}
          flagged={ribbonTargets().length > 0 && ribbonTargets().every((t) => t.starred)}
          pinned={ribbonTargets().length > 0 && ribbonTargets().every(isPinned)}
          allRead={ribbonTargets().length > 0 && ribbonTargets().every((t) => !t.any_unread)}
          view={view}
          setView={setView}
          filters={quick}
          toggleFilter={toggleQuick}
          spell={spell}
          setSpell={setSpell}
          onNew={() => setCompose({})}
          onDelete={ribbonDelete}
          onArchive={() => void ribbonMove("archive", "Archived")}
          onMove={(f) => void ribbonMove(f, `Moved to ${f}`)}
          onReply={() => { const m = openLatest(); if (m) reply(m); }}
          onReplyAll={ribbonReplyAll}
          onForward={() => { const m = openLatest(); if (m) forward(m); }}
          onLabel={ribbonLabel}
          onNewLabel={() => setLabelManagerOpen(true)}
          onFlag={ribbonFlag}
          onPin={ribbonPin}
          onSnooze={(d) => void snooze(ribbonTargets().map((t) => t.latest.id), d)}
          onMarkRead={ribbonMarkRead}
          onSweep={(m) => void ribbonSweep(m)}
          onRules={() => setFiltersOpen(true)}
          onRunRules={() => void runRules()}
          onMakeTask={() => void ribbonTask()}
          onMakeTicket={() => void ribbonTicket()}
          onAttachDrive={() => setCompose({ openDrive: true })}
          onSignatures={() => setSigsOpen(true)}
        />
        <Box sx={{ flex: 1, display: "flex", minHeight: 0 }}>
          {/* Desktop Sidebar — hidden on mobile */}
          <Box
            sx={{
              width: 220,
              flexShrink: 0,
              display: { xs: "none", md: "flex" },
              flexDirection: "column",
              overflowY: "hidden",
            }}
          >
            {sidebarContent}
            {/* Gmail-style Chat panel at the bottom of the left rail */}
            <ChatPanel user={user} />
          </Box>

          {/* Mobile sidebar Drawer */}
          <Drawer
            open={drawerOpen}
            onClose={() => setDrawerOpen(false)}
            size="sm"
            sx={{ display: { xs: "flex", md: "none" } }}
          >
            <Box
              sx={{
                display: "flex",
                flexDirection: "column",
                height: "100%",
                overflowY: "auto",
              }}
            >
              <Box
                sx={{ display: "flex", alignItems: "center", px: 1.5, py: 1 }}
              >
                <Typography level="title-sm" sx={{ flex: 1 }}>
                  Mail
                </Typography>
                <ModalClose sx={{ position: "static" }} />
              </Box>
              {sidebarContent}
              <ChatPanel user={user} />
            </Box>
          </Drawer>

          {/* Main: list or reading view */}
          <Box
            sx={{
              flex: 1,
              minWidth: 0,
              borderLeft: { xs: "none", md: "1px solid" },
              borderColor: "divider",
              display: "flex",
              flexDirection: "column",
            }}
          >
            {/* toolbar */}
            <Box
              sx={{
                display: "flex",
                alignItems: "center",
                gap: 0.5,
                px: 1.5,
                py: 0.75,
                borderBottom: "1px solid",
                borderColor: "divider",
              }}
            >
              {openThread && !split ? (
                <>
                  <IconButton
                    size="sm"
                    variant="plain"
                    onClick={() => setOpenThread(null)}
                    aria-label="Back"
                  >
                    <ArrowBackIcon />
                  </IconButton>
                  <IconButton
                    size="sm"
                    variant="plain"
                    color="danger"
                    onClick={() => trashThread(openThread.summary)}
                    aria-label="Delete"
                  >
                    <DeleteOutlineIcon />
                  </IconButton>
                  <Tooltip title="Make a task">
                    <IconButton
                      size="sm"
                      variant="plain"
                      aria-label="Make a task"
                      data-testid="mail-make-task"
                      onClick={async () => {
                        const sm = openThread.summary;
                        try {
                          await makeTask({ title: titleFrom(sm.latest.subject, "Mail follow-up"), notes: `From: ${sm.latest.from_name || sm.latest.from_addr || ""}\n${sm.latest.snippet || ""}`.trim(), link: "/mail" });
                          window.alert("Task added to your Inbox list in Tasks.");
                        } catch (e) {
                          window.alert(`Could not add the task: ${(e as Error).message}`);
                        }
                      }}
                    >
                      <TaskAltIcon />
                    </IconButton>
                  </Tooltip>
                  <Dropdown>
                    <MenuButton
                      slots={{ root: IconButton }}
                      slotProps={{
                        root: {
                          size: "sm",
                          variant: "plain",
                          "aria-label": "Snooze",
                        },
                      }}
                    >
                      <AccessTimeIcon />
                    </MenuButton>
                    <Menu size="sm" placement="bottom-start">
                      <Typography
                        level="body-xs"
                        sx={{ px: 1.5, py: 0.5, opacity: 0.7 }}
                      >
                        Snooze until…
                      </Typography>
                      {SNOOZE_PRESETS.map((p) => {
                        const m = openLatest();
                        return (
                          <MenuItem
                            key={p.label}
                            onClick={() => m && snooze([m.id], p.when())}
                          >
                            {p.label}
                          </MenuItem>
                        );
                      })}
                      <ListDivider />
                      <MenuItem
                        onClick={() => {
                          const m = openLatest();
                          if (m) promptCustomSnooze([m.id]);
                        }}
                      >
                        Pick date & time…
                      </MenuItem>
                    </Menu>
                  </Dropdown>
                  <Dropdown>
                    <MenuButton
                      slots={{ root: IconButton }}
                      slotProps={{
                        root: {
                          size: "sm",
                          variant: "plain",
                          "aria-label": "Labels",
                        },
                      }}
                    >
                      <LabelIcon />
                    </MenuButton>
                    <Menu
                      size="sm"
                      placement="bottom-start"
                      sx={{ minWidth: 200 }}
                    >
                      <Typography
                        level="body-xs"
                        sx={{ px: 1.5, py: 0.5, opacity: 0.7 }}
                      >
                        Label as
                      </Typography>
                      {openLatest() && labelMenuItems(openLatest()!)}
                      <ListDivider />
                      <MenuItem onClick={createLabel}>
                        <AddIcon sx={{ fontSize: 16, mr: 1 }} />
                        Create new label
                      </MenuItem>
                    </Menu>
                  </Dropdown>
                </>
              ) : selected.size > 0 ? (
                <>
                  <Checkbox
                    size="sm"
                    checked
                    indeterminate
                    onChange={() => setSelected(new Set())}
                    sx={{ mx: 0.5 }}
                  />
                  <Typography level="body-sm" sx={{ mr: 1 }}>
                    {selected.size} selected
                  </Typography>
                  <Tooltip title="Mark read">
                    <IconButton
                      size="sm"
                      variant="plain"
                      onClick={() =>
                        bulkSet((t) => ({ is_read: true, starred: t.starred }))
                      }
                      aria-label="Mark read"
                    >
                      <MarkEmailReadIcon />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="Mark unread">
                    <IconButton
                      size="sm"
                      variant="plain"
                      onClick={() =>
                        bulkSet((t) => ({ is_read: false, starred: t.starred }))
                      }
                      aria-label="Mark unread"
                    >
                      <MarkEmailUnreadIcon />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="Snooze">
                    <IconButton
                      size="sm"
                      variant="plain"
                      onClick={bulkSnooze}
                      aria-label="Snooze"
                    >
                      <AccessTimeIcon />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="Add star">
                    <IconButton
                      size="sm"
                      variant="plain"
                      onClick={() =>
                        bulkSet((t) => ({
                          is_read: !t.any_unread,
                          starred: true,
                        }))
                      }
                      aria-label="Star"
                    >
                      <StarIcon />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="Label">
                    <IconButton
                      size="sm"
                      variant="plain"
                      onClick={bulkLabel}
                      aria-label="Label"
                    >
                      <LabelIcon />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="Delete">
                    <IconButton
                      size="sm"
                      variant="plain"
                      color="danger"
                      onClick={bulkTrash}
                      aria-label="Delete"
                    >
                      <DeleteOutlineIcon />
                    </IconButton>
                  </Tooltip>
                </>
              ) : (
                <>
                  <IconButton
                    size="sm"
                    variant="plain"
                    onClick={reload}
                    aria-label="Refresh"
                  >
                    <RefreshIcon />
                  </IconButton>
                  <Typography
                    level="body-xs"
                    sx={{ ml: 1, opacity: 0.6, textTransform: "capitalize" }}
                  >
                    {labelFilter ? `Label: ${labelFilter}` : folder}
                  </Typography>
                  {labelFilter && (
                    <IconButton
                      size="sm"
                      variant="plain"
                      onClick={() => {
                        setLabelFilter(null);
                        setFolder("inbox");
                      }}
                      aria-label="Clear label filter"
                    >
                      <CloseIcon sx={{ fontSize: 16 }} />
                    </IconButton>
                  )}
                </>
              )}
            </Box>

            {/* Narrow screens (or reading pane off): list OR conversation. Otherwise split. */}
            <Box sx={{ flex: 1, minHeight: 0, display: "flex", flexDirection: split && view.pane === "bottom" ? "column" : "row" }}>
            {(!openThread || split) && (
            <Box
              data-testid="mail-list"
              sx={{
                overflowY: "auto",
                minWidth: 0,
                ...(split
                  ? view.pane === "bottom"
                    ? { flex: "0 0 40%", borderBottom: "1px solid", borderColor: "divider" }
                    : { flex: "0 0 400px", borderRight: "1px solid", borderColor: "divider" }
                  : { flex: 1 }),
              }}
            >
              {loading ? (
                <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
                  <CircularProgress />
                </Box>
              ) : threads.length === 0 ? (
                <Box sx={{ textAlign: "center", py: 8, opacity: 0.6 }}>
                  <Typography level="body-lg">
                    {search
                      ? "No matching mail."
                      : labelFilter
                        ? `No mail labeled "${labelFilter}".`
                        : `No mail in ${folder}.`}
                  </Typography>
                </Box>
              ) : (
                threads.map((t) => (
                  <Box
                    key={t.thread_id}
                    onClick={() => openThreadFor(t)}
                    data-testid={`mail-${t.latest.id}`}
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      gap: 1,
                      px: { xs: 1, sm: 2 },
                      py: view.density === "compact" ? 0.25 : 1,
                      cursor: "pointer",
                      borderBottom: "1px solid",
                      borderColor: "divider",
                      bgcolor: selected.has(t.thread_id)
                        ? "primary.softBg"
                        : t.any_unread
                          ? "background.body"
                          : "background.level1",
                      "&:hover": { boxShadow: "sm", zIndex: 1 },
                    }}
                  >
                    <Checkbox
                      size="sm"
                      checked={selected.has(t.thread_id)}
                      onChange={() => toggleSel(t.thread_id)}
                      onClick={(e) => e.stopPropagation()}
                      aria-label="Select"
                      sx={{ display: { xs: "none", sm: "inline-flex" } }}
                    />
                    <IconButton
                      size="sm"
                      variant="plain"
                      onClick={(e) => toggleStarThread(t, e)}
                      aria-label="Star"
                    >
                      {t.starred ? (
                        <StarIcon sx={{ color: "#f9ab00", fontSize: 18 }} />
                      ) : (
                        <StarBorderIcon sx={{ fontSize: 18 }} />
                      )}
                    </IconButton>
                    <Typography
                      level="body-sm"
                      sx={{
                        width: { xs: 120, sm: 180 },
                        fontWeight: t.any_unread ? 700 : 400,
                      }}
                      noWrap
                    >
                      {peerName(t)}
                      {t.message_count > 1 && (
                        <Typography
                          component="span"
                          level="body-xs"
                          sx={{ ml: 0.5, opacity: 0.6 }}
                        >
                          ({t.message_count})
                        </Typography>
                      )}
                    </Typography>
                    <Box
                      sx={{
                        flex: 1,
                        minWidth: 0,
                        display: "flex",
                        gap: 1,
                        alignItems: "center",
                      }}
                    >
                      {isPinned(t) && <PushPinIcon sx={{ fontSize: 14, color: "primary.plainColor", flexShrink: 0 }} titleAccess="Pinned" />}
                      <Typography
                        level="body-sm"
                        sx={{ fontWeight: t.any_unread ? 700 : 400 }}
                        noWrap
                      >
                        {t.latest.subject || "(no subject)"}
                      </Typography>
                      <Typography
                        level="body-sm"
                        sx={{
                          opacity: 0.6,
                          flexShrink: 1,
                          minWidth: 0,
                          display: { xs: "none", sm: "block" },
                        }}
                        noWrap
                      >
                        — {t.latest.snippet}
                      </Typography>
                      {t.labels.map((l) => (
                        <Chip
                          key={l}
                          size="sm"
                          variant="soft"
                          sx={{
                            flexShrink: 0,
                            "--Chip-radius": "6px",
                            bgcolor: "transparent",
                            border: "1px solid",
                            borderColor: labelColor(l),
                            color: labelColor(l),
                            display: split ? "none" : { xs: "none", sm: "inline-flex" },
                          }}
                        >
                          {l}
                        </Chip>
                      ))}
                    </Box>
                    {folder === "snoozed" && t.latest.snooze_until && (
                      <Tooltip
                        title={`Snoozed until ${new Date(t.latest.snooze_until).toLocaleString()}`}
                      >
                        <Chip
                          size="sm"
                          variant="soft"
                          color="warning"
                          startDecorator={
                            <AccessTimeIcon sx={{ fontSize: 14 }} />
                          }
                        >
                          {fmtDate(t.latest.snooze_until)}
                        </Chip>
                      </Tooltip>
                    )}
                    <Typography
                      level="body-xs"
                      sx={{
                        width: 56,
                        textAlign: "right",
                        fontWeight: t.any_unread ? 700 : 400,
                        opacity: 0.8,
                      }}
                    >
                      {fmtDate(t.latest.sent_at)}
                    </Typography>
                    <Tooltip title="Snooze">
                      <IconButton
                        className="row-del"
                        size="sm"
                        variant="plain"
                        onClick={(e) => {
                          e.stopPropagation();
                          snooze([t.latest.id], SNOOZE_PRESETS[0].when());
                        }}
                        aria-label="Snooze"
                        sx={{ display: { xs: "none", sm: "inline-flex" } }}
                      >
                        <AccessTimeIcon sx={{ fontSize: 18 }} />
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Delete">
                      <IconButton
                        className="row-del"
                        size="sm"
                        variant="plain"
                        onClick={(e) => {
                          e.stopPropagation();
                          trashThread(t);
                        }}
                        aria-label="Delete"
                        sx={{ display: { xs: "none", sm: "inline-flex" } }}
                      >
                        <DeleteOutlineIcon sx={{ fontSize: 18 }} />
                      </IconButton>
                    </Tooltip>
                  </Box>
                ))
              )}
            </Box>
            )}
            {openThread && (
              <Box data-testid="mail-reading" sx={{ flex: 1, minWidth: 0, minHeight: 0, overflowY: "auto" }}>
                <ThreadView
                  t={openThread}
                  loading={openLoading}
                  onStar={toggleStarMsg}
                  onReply={reply}
                  onForward={forward}
                  onMessageAction={onMessageAction}
                  resolveColor={labelColor}
                  onNotice={setNotice}
                />
              </Box>
            )}
            {split && !openThread && (
              <Box sx={{ flex: 1, display: { xs: "none", md: "flex" }, alignItems: "center", justifyContent: "center", opacity: 0.55 }}>
                <Typography level="body-md">Select a conversation to read it here</Typography>
              </Box>
            )}
            </Box>
          </Box>
        </Box>
      </Box>

      {compose !== null && (
        <Compose
          init={compose}
          sendFn={deliver}
          onClose={() => setCompose(null)}
          onSent={() => {
            /* delivery handles reload */
          }}
        />
      )}
      {rulesOpen && <RulesDialog onClose={() => setRulesOpen(false)} />}
      {labelManagerOpen && (
        <LabelManagerDialog
          onClose={() => setLabelManagerOpen(false)}
          onChange={refreshLabels}
        />
      )}
      {filtersOpen && <FiltersDialog onClose={() => setFiltersOpen(false)} />}
      {sigsOpen && <SignaturesDialog onClose={() => setSigsOpen(false)} />}

      {/* Undo-send toast */}
      <Snackbar
        open={!!undo}
        variant="solid"
        color="neutral"
        autoHideDuration={UNDO_SEND_MS}
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
        endDecorator={
          <Button
            onClick={undoSend}
            size="sm"
            variant="plain"
            color="primary"
            sx={{ color: "#8ab4f8" }}
          >
            Undo
          </Button>
        }
      >
        Sending "{undo?.subject}"…
      </Snackbar>
      <Snackbar
        open={!!notice}
        variant="soft"
        color="neutral"
        autoHideDuration={3000}
        onClose={() => setNotice(null)}
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
      >
        {notice}
      </Snackbar>
    </>
  );
}

// ThreadView renders all messages in a conversation, oldest first. The newest is
// expanded; older messages are collapsed to a one-line summary that expands on click.
function ThreadView({
  t,
  loading,
  onStar,
  onReply,
  onForward,
  onMessageAction,
  resolveColor,
  onNotice,
}: {
  t: { summary: MailThread; messages: MailMessage[] };
  onNotice?: (m: string) => void;
  loading: boolean;
  onStar: (m: MailMessage) => void;
  onReply: (m: MailMessage) => void;
  onForward: (m: MailMessage) => void;
  onMessageAction: (action: string, m: MailMessage) => void;
  resolveColor?: (name: string) => string;
}) {
  const getColor = resolveColor ?? colorFor;
  const msgs = t.messages;
  const lastIdx = msgs.length - 1;
  const [expanded, setExpanded] = useState<Set<number>>(new Set());
  // Per-message "Show original" raw-source inspection.
  const [rawFor, setRawFor] = useState<string | null>(null);
  const [rawText, setRawText] = useState("");
  const [rawLoading, setRawLoading] = useState(false);
  async function toggleRaw(m: MailMessage) {
    if (rawFor === m.id) {
      setRawFor(null);
      return;
    }
    setRawFor(m.id);
    setRawText("");
    setRawLoading(true);
    try {
      setRawText(await getRawSource(m.id));
    } catch {
      setRawText("(could not load original source)");
    } finally {
      setRawLoading(false);
    }
  }
  async function downloadEml(m: MailMessage) {
    try {
      const raw = await getRawSource(m.id);
      const blob = new Blob([raw], { type: "message/rfc822" });
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${(m.subject || "message").replace(/[^\w.-]+/g, "_")}.eml`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      /* ignore */
    }
  }
  // The newest message always starts expanded.
  const isOpen = (i: number) => i === lastIdx || expanded.has(i);
  const toggle = (i: number) =>
    setExpanded((s) => {
      const n = new Set(s);
      n.has(i) ? n.delete(i) : n.add(i);
      return n;
    });
  const subject = useMemo(
    () => msgs.find((m) => m.subject)?.subject || "(no subject)",
    [msgs],
  );

  if (loading && msgs.length === 0) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box sx={{ p: { xs: 1.5, sm: 3 }, maxWidth: 860 }}>
      <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
        <Typography
          level="h4"
          sx={{ flex: 1, fontSize: { xs: "lg", sm: "xl2" } }}
        >
          {subject}
        </Typography>
        {msgs.length > 1 && (
          <Chip size="sm" variant="soft" sx={{ mr: 1 }}>
            {msgs.length} messages
          </Chip>
        )}
      </Box>
      {msgs.map((m, i) => (
        <Box
          key={m.id}
          sx={{
            mb: 2,
            borderBottom: i < lastIdx ? "1px solid" : "none",
            borderColor: "divider",
            pb: 2,
          }}
        >
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1.5,
              cursor: i === lastIdx ? "default" : "pointer",
            }}
            onClick={() => {
              if (i !== lastIdx) toggle(i);
            }}
          >
            <PersonCard email={m.from_addr} name={m.from_name || m.from_addr}>
              <Avatar
                size="sm"
                sx={{ bgcolor: colorFor(m.from_addr), color: "#fff" }}
              >
                {(m.from_name || m.from_addr || "?").charAt(0).toUpperCase()}
              </Avatar>
            </PersonCard>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <PersonCard email={m.from_addr} name={m.from_name || m.from_addr}>
                <Typography level="body-sm" sx={{ fontWeight: 600 }} noWrap>
                  {m.from_name || m.from_addr}
                </Typography>
              </PersonCard>
              {isOpen(i) ? (
                <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                  to {m.to_addrs.join(", ") || "me"}
                  {m.cc_addrs.length ? ` · cc ${m.cc_addrs.join(", ")}` : ""}
                </Typography>
              ) : (
                <Typography level="body-xs" sx={{ opacity: 0.6 }} noWrap>
                  {m.snippet}
                </Typography>
              )}
            </Box>
            <Typography level="body-xs" sx={{ opacity: 0.7, flexShrink: 0 }}>
              {new Date(m.sent_at).toLocaleString()}
            </Typography>
            <Dropdown>
              <MenuButton
                slots={{ root: IconButton }}
                slotProps={{
                  root: {
                    size: "sm",
                    variant: "plain",
                    color: "neutral",
                    onClick: (e: React.MouseEvent) => e.stopPropagation(),
                    "aria-label": "More",
                  },
                }}
              >
                <MoreVertIcon />
              </MenuButton>
              <Menu
                placement="bottom-end"
                size="sm"
                onClick={(e) => e.stopPropagation()}
                sx={{ "--ListItemDecorator-size": "28px", minWidth: 240 }}
              >
                <MenuItem onClick={() => onMessageAction("reply", m)}>
                  <ListItemDecorator>
                    <ReplyIcon />
                  </ListItemDecorator>
                  Reply
                </MenuItem>
                <MenuItem onClick={() => onMessageAction("forward", m)}>
                  <ListItemDecorator>
                    <ForwardIcon />
                  </ListItemDecorator>
                  Forward
                </MenuItem>
                <ListDivider />
                <MenuItem onClick={() => onMessageAction("delete", m)}>
                  <ListItemDecorator>
                    <DeleteOutlineIcon />
                  </ListItemDecorator>
                  Delete
                </MenuItem>
                <MenuItem onClick={() => onMessageAction("unread", m)}>
                  <ListItemDecorator>
                    <MarkEmailUnreadIcon />
                  </ListItemDecorator>
                  Mark unread from here
                </MenuItem>
                <ListDivider />
                <MenuItem onClick={() => onMessageAction("block", m)}>
                  <ListItemDecorator>
                    <BlockIcon />
                  </ListItemDecorator>
                  Block &quot;{m.from_name || m.from_addr}&quot;
                </MenuItem>
                <MenuItem onClick={() => onMessageAction("spam", m)}>
                  <ListItemDecorator>
                    <ReportOutlinedIcon />
                  </ListItemDecorator>
                  Report spam
                </MenuItem>
                <MenuItem onClick={() => onMessageAction("phishing", m)}>
                  <ListItemDecorator>
                    <ReportGmailerrorredIcon />
                  </ListItemDecorator>
                  Report phishing
                </MenuItem>
                <ListDivider />
                <MenuItem onClick={() => onMessageAction("filter", m)}>
                  <ListItemDecorator>
                    <FilterAltIcon />
                  </ListItemDecorator>
                  Filter messages like this
                </MenuItem>
                <MenuItem onClick={() => onMessageAction("translate", m)}>
                  <ListItemDecorator>
                    <TranslateIcon />
                  </ListItemDecorator>
                  Translate
                </MenuItem>
                <MenuItem onClick={() => window.print()}>
                  <ListItemDecorator>
                    <PrintIcon />
                  </ListItemDecorator>
                  Print
                </MenuItem>
                <MenuItem onClick={() => downloadEml(m)}>
                  <ListItemDecorator>
                    <DownloadIcon />
                  </ListItemDecorator>
                  Download message
                </MenuItem>
                <MenuItem onClick={() => toggleRaw(m)}>
                  <ListItemDecorator>
                    <CodeIcon />
                  </ListItemDecorator>
                  Show original
                </MenuItem>
              </Menu>
            </Dropdown>
            <IconButton
              size="sm"
              variant="plain"
              onClick={(e) => {
                e.stopPropagation();
                onStar(m);
              }}
              aria-label="Star"
            >
              {m.starred ? (
                <StarIcon sx={{ color: "#f9ab00" }} />
              ) : (
                <StarBorderIcon />
              )}
            </IconButton>
          </Box>
          {isOpen(i) && (
            <>
              {m.labels.length > 0 && (
                <Box sx={{ display: "flex", gap: 0.5, mt: 1, ml: 5 }}>
                  {m.labels.map((l) => (
                    <Chip
                      key={l}
                      size="sm"
                      variant="soft"
                      sx={{
                        bgcolor: "transparent",
                        border: "1px solid",
                        borderColor: getColor(l),
                        color: getColor(l),
                      }}
                    >
                      {l}
                    </Chip>
                  ))}
                </Box>
              )}
              {rawFor === m.id ? (
                <Box sx={{ mt: 1.5, ml: { xs: 0, sm: 5 } }}>
                  <Box
                    sx={{
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "space-between",
                      mb: 0.5,
                    }}
                  >
                    <Typography level="body-xs" sx={{ opacity: 0.7 }}>
                      Original message source
                    </Typography>
                    <Box sx={{ display: "flex", gap: 0.5 }}>
                      <Button
                        size="sm"
                        variant="plain"
                        onClick={() =>
                          navigator.clipboard?.writeText(rawText).catch(() => {})
                        }
                      >
                        Copy
                      </Button>
                      <Button
                        size="sm"
                        variant="plain"
                        onClick={() => setRawFor(null)}
                      >
                        Close
                      </Button>
                    </Box>
                  </Box>
                  <Sheet
                    variant="soft"
                    sx={{
                      p: 1.5,
                      borderRadius: "sm",
                      maxHeight: 480,
                      overflow: "auto",
                      fontFamily: "monospace",
                      fontSize: "12px",
                      whiteSpace: "pre-wrap",
                      wordBreak: "break-word",
                    }}
                  >
                    {rawLoading ? "Loading…" : rawText}
                  </Sheet>
                </Box>
              ) : (
                <Typography
                  sx={{
                    whiteSpace: "pre-wrap",
                    lineHeight: 1.6,
                    mt: 1.5,
                    ml: { xs: 0, sm: 5 },
                    wordBreak: "break-word",
                  }}
                >
                  <LinkifiedBody text={m.body} />
                </Typography>
              )}
              {m.attachments && m.attachments.length > 0 && (
                <Box sx={{ mt: 2, ml: { xs: 0, sm: 5 } }}>
                  <Typography level="body-xs" sx={{ opacity: 0.6, mb: 0.5 }}>
                    {m.attachments.length} attachment
                    {m.attachments.length === 1 ? "" : "s"}
                  </Typography>
                  <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                    {m.attachments.map((a) => (
                      <AttachmentCard
                        key={a.id}
                        messageId={m.id}
                        att={a}
                        onNotice={onNotice}
                      />
                    ))}
                  </Box>
                </Box>
              )}
            </>
          )}
        </Box>
      ))}
      <Box sx={{ display: "flex", gap: 1, mt: 1, flexWrap: "wrap" }}>
        <Button
          variant="outlined"
          color="neutral"
          startDecorator={<ReplyIcon />}
          onClick={() => onReply(msgs[lastIdx])}
          sx={{ minHeight: 40 }}
        >
          Reply
        </Button>
        <Button
          variant="outlined"
          color="neutral"
          startDecorator={<ForwardIcon />}
          onClick={() => onForward(msgs[lastIdx])}
          sx={{ minHeight: 40 }}
        >
          Forward
        </Button>
      </Box>
    </Box>
  );
}
