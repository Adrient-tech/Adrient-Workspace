/**
 * Teams-style channel tabs: Posts, Files, and tabs pinned by members (a Doc,
 * Sheet, Slides deck, Whiteboard, Tasks list, Project board or a link). Pinned
 * app tabs render the app embedded (?embed=1) inside the channel.
 */
import { useEffect, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Typography,
} from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import CloseIcon from "@mui/icons-material/Close";
import { appForFile, openLink, type AppId } from "../../lib/openWith";
import { withEmbed } from "../../lib/embed";
import { listRecent } from "../drive/api";
import type { DriveFile } from "../drive/types";
import { AttachmentCard, extractCardLinks, isCardAttachment, LinkCards } from "./LinkCards";
import { teams } from "./teamsApi";
import type { ChannelFile, ChannelTab, TMessage } from "./types";

export type TabId = "posts" | "files" | string;

const KIND_LABEL: Record<ChannelTab["kind"], string> = {
  doc: "Doc",
  sheet: "Sheet",
  slides: "Slides",
  whiteboard: "Whiteboard",
  tasks: "Tasks list",
  project: "Project board",
  link: "Link",
};

const KIND_APP: Partial<Record<ChannelTab["kind"], AppId>> = {
  doc: "docs",
  sheet: "sheets",
  slides: "slides",
  whiteboard: "whiteboard",
};

const KIND_ROUTE: Partial<Record<ChannelTab["kind"], string>> = { tasks: "/tasks", project: "/projects" };

export function TabBar({
  tab,
  setTab,
  tabs,
  canEdit,
  onAdd,
  onRemove,
}: {
  tab: TabId;
  setTab: (t: TabId) => void;
  tabs: ChannelTab[];
  canEdit: boolean;
  onAdd: () => void;
  onRemove: (t: ChannelTab) => void;
}) {
  const item = (id: TabId, label: string, removable?: ChannelTab) => (
    <Box
      key={id}
      role="tab"
      aria-selected={tab === id}
      tabIndex={0}
      data-testid={`channel-tab-${id === "posts" || id === "files" ? id : "pinned"}`}
      onClick={() => setTab(id)}
      onKeyDown={(e) => e.key === "Enter" && setTab(id)}
      sx={{
        display: "flex",
        alignItems: "center",
        gap: 0.5,
        px: 1.5,
        py: 0.75,
        cursor: "pointer",
        whiteSpace: "nowrap",
        fontSize: 14,
        fontWeight: tab === id ? 700 : 500,
        borderBottom: "2px solid",
        borderColor: tab === id ? "primary.500" : "transparent",
        "&:hover": { bgcolor: "background.level1" },
        "&:hover .tab-x": { opacity: 0.7 },
      }}
    >
      {label}
      {removable && canEdit && (
        <Box
          component="span"
          className="tab-x"
          role="button"
          aria-label={`Remove tab ${label}`}
          onClick={(e: React.MouseEvent) => {
            e.stopPropagation();
            onRemove(removable);
          }}
          sx={{ opacity: 0, display: "inline-flex", ml: 0.25 }}
        >
          <CloseIcon sx={{ fontSize: 14 }} />
        </Box>
      )}
    </Box>
  );
  return (
    <Box role="tablist" sx={{ display: "flex", alignItems: "center", px: 1, borderBottom: "1px solid", borderColor: "divider", overflowX: "auto" }}>
      {item("posts", "Posts")}
      {item("files", "Files")}
      {tabs.map((t) => item(t.id, t.title, t))}
      {canEdit && (
        <IconButton size="sm" variant="plain" onClick={onAdd} aria-label="Add a tab" data-testid="add-tab" sx={{ ml: 0.5 }}>
          <AddIcon />
        </IconButton>
      )}
    </Box>
  );
}

/** Embeds the pinned app (or shows a card for an external link). */
export function PinnedTabView({ tab }: { tab: ChannelTab }) {
  const t = openLink(tab.url);
  if (!t || t.external) {
    return (
      <Box sx={{ p: 3 }}>
        <Typography level="title-md">{tab.title}</Typography>
        <Typography level="body-sm" sx={{ mb: 1.5, opacity: 0.7 }}>
          External links cannot be shown inside Chat.
        </Typography>
        <Button component="a" href={tab.url} target="_blank" rel="noopener noreferrer">
          Open link
        </Button>
      </Box>
    );
  }
  return (
    <Box
      component="iframe"
      key={tab.id}
      title={tab.title}
      data-testid="pinned-tab-frame"
      src={withEmbed(t.href)}
      sx={{ flex: 1, border: 0, width: "100%", minHeight: 0 }}
    />
  );
}

/** Everything shared in the channel: attachments plus Drive/app file links in messages. */
export function FilesPanel({ channelId, messages }: { channelId: string; messages: TMessage[] }) {
  const [files, setFiles] = useState<ChannelFile[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    setFiles(null);
    teams.files(channelId).then(setFiles).catch((e: Error) => setError(e.message));
  }, [channelId, messages.length]);
  const links = useMemo(() => {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const m of messages) {
      for (const l of extractCardLinks(m.body ?? "")) {
        if (l.target.app === "meet" || seen.has(l.target.href)) continue;
        if (!["docs", "sheets", "slides", "pdf", "whiteboard", "3d", "drive"].includes(l.target.app)) continue;
        seen.add(l.target.href);
        out.push(l.url);
      }
    }
    return out;
  }, [messages]);
  if (error) return <Alert color="danger" sx={{ m: 2 }}>{error}</Alert>;
  if (!files) {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 4 }}>
        <CircularProgress size="sm" />
      </Box>
    );
  }
  return (
    <Box sx={{ flex: 1, overflowY: "auto", p: 2 }} data-testid="files-panel">
      {files.length === 0 && links.length === 0 && (
        <Typography level="body-sm" sx={{ opacity: 0.7 }}>
          No files shared in this conversation yet.
        </Typography>
      )}
      {files.map((f) =>
        isCardAttachment(f) ? (
          <AttachmentCard key={f.id} att={f} size={`${new Date(f.sent_at).toLocaleDateString()}`} />
        ) : (
          <Box key={f.id} sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.75, borderBottom: "1px solid", borderColor: "divider" }}>
            <Typography level="body-sm" sx={{ flex: 1 }} noWrap>
              {f.name}
            </Typography>
            <Typography level="body-xs" sx={{ opacity: 0.6 }}>
              {new Date(f.sent_at).toLocaleDateString()}
            </Typography>
            <Button size="sm" variant="plain" component="a" href={f.url} target="_blank" rel="noopener noreferrer">
              Open
            </Button>
          </Box>
        ),
      )}
      {links.length > 0 && (
        <>
          <Typography level="title-sm" sx={{ mt: 2 }}>
            Links to files
          </Typography>
          {links.map((u) => (
            <LinkCards key={u} body={u} />
          ))}
        </>
      )}
    </Box>
  );
}

/** Dialog to pin a Doc/Sheet/Slides/Whiteboard/Tasks/Project/link as a channel tab. */
export function AddTabDialog({ channelId, onClose, onAdded }: { channelId: string; onClose: () => void; onAdded: (t: ChannelTab) => void }) {
  const [kind, setKind] = useState<ChannelTab["kind"]>("sheet");
  const [title, setTitle] = useState("");
  const [url, setUrl] = useState("");
  const [recent, setRecent] = useState<DriveFile[]>([]);
  const [pick, setPick] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  useEffect(() => {
    listRecent().then(setRecent).catch(() => setRecent([]));
  }, []);
  const app = KIND_APP[kind];
  const candidates = useMemo(
    () => (app ? recent.filter((f) => appForFile({ name: f.name, mime_type: f.mime_type }) === app || (kind === "doc" && /\.(docx?|md|txt)$/i.test(f.name))) : []),
    [recent, app, kind],
  );
  const isFile = !!app;
  const fixed = KIND_ROUTE[kind];

  const submit = async () => {
    let target = fixed ?? url.trim();
    let name = title.trim();
    if (isFile && pick) {
      target = `/${app}/${pick}`;
      name = name || (recent.find((f) => f.id === pick)?.name ?? "");
    }
    if (!target) {
      setErr(isFile ? "Pick a file or paste its link." : "Enter a link.");
      return;
    }
    // Accept same-workspace absolute links: store them as in-app paths.
    const o = openLink(target);
    if (o && !o.external) target = o.href;
    setBusy(true);
    setErr("");
    try {
      const t = await teams.addTab(channelId, { kind, title: name || KIND_LABEL[kind], url: target });
      onAdded(t);
    } catch (e) {
      setErr((e as Error).message);
      setBusy(false);
    }
  };

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 480, maxWidth: "95vw" }} data-testid="add-tab-dialog">
        <ModalClose />
        <Typography level="title-lg">Add a tab</Typography>
        <Select value={kind} onChange={(_e, v) => v && (setKind(v), setPick(""))} slotProps={{ button: { "aria-label": "Tab type" } as never }}>
          {(Object.keys(KIND_LABEL) as ChannelTab["kind"][]).map((k) => (
            <Option key={k} value={k}>
              {KIND_LABEL[k]}
            </Option>
          ))}
        </Select>
        {isFile && (
          <Select placeholder="Pick a recent Drive file" value={pick || null} onChange={(_e, v) => setPick(v ?? "")} slotProps={{ button: { "aria-label": "Drive file" } as never }}>
            {candidates.map((f) => (
              <Option key={f.id} value={f.id}>
                {f.name}
              </Option>
            ))}
          </Select>
        )}
        {!fixed && !pick && (
          <Input
            placeholder={isFile ? "...or paste a link to the file" : "https://…"}
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            slotProps={{ input: { "aria-label": "Tab link" } }}
          />
        )}
        <Input placeholder="Tab name (optional)" value={title} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "aria-label": "Tab name" } }} />
        {err && <Alert color="danger" size="sm">{err}</Alert>}
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={busy} onClick={() => void submit()} data-testid="add-tab-save">
            Add tab
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
