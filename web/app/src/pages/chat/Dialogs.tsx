import { PeopleSelect } from "../../components/PeoplePicker";
import { useState } from "react";
import {
  Box,
  Button,
  Checkbox,
  Dropdown,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  List,
  ListItem,
  Menu,
  MenuButton,
  MenuItem,
  Modal,
  ModalClose,
  ModalDialog,
  Radio,
  RadioGroup,
  Tab,
  TabList,
  TabPanel,
  Tabs,
  Textarea,
  Typography,
  ListDivider,
  Divider,
  Tooltip } from "@mui/joy";
import ScheduleSendIcon from "@mui/icons-material/ScheduleSend";
import TagIcon from "@mui/icons-material/Tag";
import LockIcon from "@mui/icons-material/Lock";
import { UserAvatar } from "./Message";
import { channelLabel, localInputValue, presets, type PeopleMap } from "./lib";
import { teams } from "./teamsApi";
import type { Person, TChannel } from "./types";

// ---- send later -------------------------------------------------------------

export function SendLaterButton({
  value,
  onSchedule,
}: {
  value: string;
  onSchedule: (at: Date) => void;
}) {
  const [custom, setCustom] = useState(false);
  const [when, setWhen] = useState(localInputValue(new Date(Date.now() + 3600_000)));
  return (
    <>
      <Dropdown>
        <Tooltip title="Send later" placement="top">
          <MenuButton
            slots={{ root: IconButton }}
            slotProps={{ root: { size: "sm", variant: "plain", disabled: !value.trim(), "aria-label": "Send later" } }}
          >
            <ScheduleSendIcon sx={{ fontSize: 18 }} />
          </MenuButton>
        </Tooltip>
        <Menu size="sm" placement="top-end" sx={{ zIndex: 1500 }}>
          <Typography level="body-xs" sx={{ px: 1.5, py: 0.5, opacity: 0.6 }}>
            Schedule message
          </Typography>
          {presets().map((p) => (
            <MenuItem key={p.label} onClick={() => onSchedule(p.at)}>
              {p.label}
            </MenuItem>
          ))}
          <ListDivider />
          <MenuItem onClick={() => setCustom(true)}>Custom time…</MenuItem>
        </Menu>
      </Dropdown>
      <Modal open={custom} onClose={() => setCustom(false)}>
        <ModalDialog sx={{ minWidth: 320 }}>
          <ModalClose />
          <Typography level="title-md">Schedule message</Typography>
          <Input type="datetime-local" value={when} onChange={(e) => setWhen(e.target.value)} slotProps={{ input: { "aria-label": "Send at" } }} />
          <Button
            disabled={!when || new Date(when).getTime() <= Date.now()}
            onClick={() => {
              setCustom(false);
              onSchedule(new Date(when));
            }}
          >
            Schedule
          </Button>
        </ModalDialog>
      </Modal>
    </>
  );
}

// ---- people picker ----------------------------------------------------------

export function PeoplePicker({
  people,
  me,
  selected,
  onChange,
  exclude = [],
  max,
}: {
  people: Person[];
  me: string;
  selected: string[];
  onChange: (ids: string[]) => void;
  exclude?: string[];
  max?: number;
}) {
  // Searches the whole directory on the server (paged); `people` only gives
  // names for chips that are already known.
  return (
    <Box>
      <PeopleSelect
        value={selected}
        known={people}
        onChange={(ids) => {
          const clean = ids.filter((id) => id !== me && !exclude.includes(id));
          onChange(max !== undefined ? clean.slice(0, max) : clean);
        }}
        ariaLabel="Search people"
        placeholder="Search people by name or email"
        maxChips={8}
      />
      {max !== undefined && selected.length >= max && (
        <Typography level="body-xs" sx={{ mt: 0.5, opacity: 0.7 }}>
          Up to {max} people.
        </Typography>
      )}
    </Box>
  );
}

// ---- new conversation / channel --------------------------------------------

export function NewChannelDialog({
  open,
  onClose,
  people,
  me,
  onCreated,
  initialTab = 0,
}: {
  open: boolean;
  onClose: () => void;
  people: Person[];
  me: string;
  onCreated: (ch: TChannel) => void;
  initialTab?: number;
}) {
  const [tab, setTab] = useState(initialTab);
  const [name, setName] = useState("");
  const [topic, setTopic] = useState("");
  const [visibility, setVisibility] = useState<"public" | "private">("public");
  const [members, setMembers] = useState<string[]>([]);
  const [dmPeople, setDmPeople] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const create = async (input: Parameters<typeof teams.createChannel>[0]) => {
    setBusy(true);
    setErr("");
    try {
      const ch = await teams.createChannel(input);
      onCreated(ch);
      setName("");
      setTopic("");
      setMembers([]);
      setDmPeople([]);
      onClose();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: 480, maxWidth: "95vw", maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">{tab === 0 ? "Create a channel" : "New chat"}</Typography>
        <Tabs value={tab} onChange={(_e, v) => setTab(v as number)} sx={{ bgcolor: "transparent" }}>
          <TabList>
            <Tab>Channel</Tab>
            <Tab>1:1 or group chat</Tab>
          </TabList>
          <TabPanel value={0} sx={{ px: 0 }}>
            <FormControl sx={{ mb: 1 }}>
              <FormLabel>Channel name</FormLabel>
              <Input
                autoFocus
                startDecorator={visibility === "public" ? <TagIcon sx={{ fontSize: 16 }} /> : <LockIcon sx={{ fontSize: 16 }} />}
                placeholder="e.g. plan-budget"
                value={name}
                onChange={(e) => setName(e.target.value)}
                slotProps={{ input: { "aria-label": "Channel name", maxLength: 60 } }}
              />
            </FormControl>
            <FormControl sx={{ mb: 1 }}>
              <FormLabel>Topic (optional)</FormLabel>
              <Input value={topic} onChange={(e) => setTopic(e.target.value)} slotProps={{ input: { "aria-label": "Topic", maxLength: 250 } }} />
            </FormControl>
            <RadioGroup value={visibility} onChange={(e) => setVisibility(e.target.value as "public" | "private")} sx={{ mb: 1 }}>
              <Radio value="public" label="Public: anyone in the workspace can find and join" />
              <Radio value="private" label="Private: only people you add" />
            </RadioGroup>
            <FormLabel>Add people</FormLabel>
            <PeoplePicker people={people} me={me} selected={members} onChange={setMembers} />
            {err && (
              <Typography level="body-sm" color="danger" sx={{ mt: 1 }}>
                {err}
              </Typography>
            )}
            <Button
              fullWidth
              sx={{ mt: 1.5 }}
              loading={busy}
              disabled={!name.trim()}
              onClick={() => void create({ kind: visibility, name, topic, member_ids: members })}
            >
              Create channel
            </Button>
          </TabPanel>
          <TabPanel value={1} sx={{ px: 0 }}>
            <Typography level="body-sm" sx={{ mb: 1, opacity: 0.7 }}>
              Pick one person for a DM, or 2 to 8 for a group conversation. Pick nobody for a notes-to-self space.
            </Typography>
            <PeoplePicker people={people} me={me} selected={dmPeople} onChange={setDmPeople} max={8} />
            {err && (
              <Typography level="body-sm" color="danger" sx={{ mt: 1 }}>
                {err}
              </Typography>
            )}
            <Button
              fullWidth
              sx={{ mt: 1.5 }}
              loading={busy}
              disabled={dmPeople.length > 8}
              onClick={() => void create({ kind: dmPeople.length >= 2 ? "group" : "dm", member_ids: dmPeople })}
            >
              {dmPeople.length >= 2 ? "Start group conversation" : dmPeople.length === 1 ? "Open DM" : "Open notes to self"}
            </Button>
          </TabPanel>
        </Tabs>
      </ModalDialog>
    </Modal>
  );
}

// ---- channel details --------------------------------------------------------

export function DetailsDialog({
  open,
  onClose,
  channel,
  me,
  people,
  peopleList,
  presence,
  onChanged,
  onLeft,
}: {
  open: boolean;
  onClose: () => void;
  channel: TChannel;
  me: string;
  people: PeopleMap;
  peopleList: Person[];
  presence: (id: string) => "online" | "away" | "offline";
  onChanged: () => void;
  onLeft: () => void;
}) {
  const isChan = channel.kind === "public" || channel.kind === "private";
  const [name, setName] = useState(channel.name);
  const [topic, setTopic] = useState(channel.topic);
  const [desc, setDesc] = useState(channel.description);
  const [add, setAdd] = useState<string[]>([]);
  const [err, setErr] = useState("");
  const [bmTitle, setBmTitle] = useState("");
  const [bmUrl, setBmUrl] = useState("");

  const run = async (fn: () => Promise<unknown>, after?: () => void) => {
    setErr("");
    try {
      await fn();
      onChanged();
      after?.();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: 560, maxWidth: "95vw", maxHeight: "90vh", overflow: "auto" }}>
        <ModalClose />
        <Typography level="title-lg">{channelLabel(channel, me, people)}</Typography>
        <Tabs defaultValue={0} sx={{ bgcolor: "transparent" }}>
          <TabList>
            <Tab>About</Tab>
            <Tab>Members {channel.member_ids.length}</Tab>
            {isChan && <Tab>Settings</Tab>}
          </TabList>
          <TabPanel value={0} sx={{ px: 0 }}>
            {isChan ? (
              <>
                <Typography level="body-xs">Topic</Typography>
                <Input value={topic} onChange={(e) => setTopic(e.target.value)} onBlur={() => topic !== channel.topic && void run(() => teams.updateChannel(channel.id, { topic }))} slotProps={{ input: { "aria-label": "Channel topic" } }} sx={{ mb: 1 }} />
                <Typography level="body-xs">Description</Typography>
                <Textarea minRows={3} value={desc} onChange={(e) => setDesc(e.target.value)} onBlur={() => desc !== channel.description && void run(() => teams.updateChannel(channel.id, { description: desc }))} slotProps={{ textarea: { "aria-label": "Channel description" } }} />
              </>
            ) : (
              <Typography level="body-sm">This is a {channel.kind === "dm" ? "direct message" : "group conversation"}.</Typography>
            )}
            <Divider sx={{ my: 1.5 }} />
            <Typography level="body-xs">Created {new Date(channel.created_at).toLocaleDateString()}{channel.created_by && people[channel.created_by] ? ` by ${people[channel.created_by].name}` : ""}</Typography>
            {isChan && (
              <>
                <Divider sx={{ my: 1.5 }} />
                <Typography level="title-sm">Bookmarks</Typography>
                <Box sx={{ display: "flex", gap: 1, mt: 0.5 }}>
                  <Input size="sm" placeholder="Title" value={bmTitle} onChange={(e) => setBmTitle(e.target.value)} sx={{ flex: 1 }} slotProps={{ input: { "aria-label": "Bookmark title" } }} />
                  <Input size="sm" placeholder="https://…" value={bmUrl} onChange={(e) => setBmUrl(e.target.value)} sx={{ flex: 2 }} slotProps={{ input: { "aria-label": "Bookmark URL" } }} />
                  <Button
                    size="sm"
                    disabled={!bmUrl.trim()}
                    onClick={() =>
                      void run(() => teams.addBookmark(channel.id, bmTitle, bmUrl), () => {
                        setBmTitle("");
                        setBmUrl("");
                      })
                    }
                  >
                    Add
                  </Button>
                </Box>
              </>
            )}
            {channel.kind !== "dm" && (
              <Button sx={{ mt: 2 }} color="danger" variant="soft" onClick={() => void run(() => teams.leave(channel.id), () => { onClose(); onLeft(); })}>
                Leave {isChan ? "channel" : "conversation"}
              </Button>
            )}
          </TabPanel>
          <TabPanel value={1} sx={{ px: 0 }}>
            <List size="sm" sx={{ maxHeight: 240, overflow: "auto" }}>
              {channel.member_ids.map((id) => (
                <ListItem
                  key={id}
                  endAction={
                    channel.kind !== "dm" && id !== me ? (
                      <Button size="sm" variant="plain" color="danger" onClick={() => void run(() => teams.removeMember(channel.id, id))}>
                        Remove
                      </Button>
                    ) : undefined
                  }
                >
                  <UserAvatar id={id} name={people[id]?.name ?? "?"} size={28} />
                  <Box sx={{ ml: 1 }}>
                    <Typography level="body-sm">
                      {people[id]?.name ?? id} {id === me && "(you)"}
                    </Typography>
                    <Typography level="body-xs" sx={{ opacity: 0.6 }}>
                      {presence(id)}
                    </Typography>
                  </Box>
                </ListItem>
              ))}
            </List>
            {channel.kind !== "dm" && (
              <>
                <Divider sx={{ my: 1 }} />
                <Typography level="title-sm" sx={{ mb: 0.5 }}>
                  Add people
                </Typography>
                <PeoplePicker people={peopleList} me={me} exclude={channel.member_ids} selected={add} onChange={setAdd} />
                <Button sx={{ mt: 1 }} disabled={add.length === 0} onClick={() => void run(() => teams.addMembers(channel.id, add), () => setAdd([]))}>
                  Add {add.length || ""} {add.length === 1 ? "person" : "people"}
                </Button>
              </>
            )}
          </TabPanel>
          {isChan && (
            <TabPanel value={2} sx={{ px: 0 }}>
              <FormControl sx={{ mb: 1 }}>
                <FormLabel>Name</FormLabel>
                <Input value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "aria-label": "Channel name" } }} />
              </FormControl>
              <Button size="sm" disabled={!name.trim() || name === channel.name} onClick={() => void run(() => teams.updateChannel(channel.id, { name }))}>
                Rename
              </Button>
              <Divider sx={{ my: 2 }} />
              <Checkbox
                label="Announcement channel (only the channel owner posts; everyone can reply in threads)"
                checked={!!channel.announce}
                onChange={(e) => void run(() => teams.updateChannel(channel.id, { announce: e.target.checked }))}
                slotProps={{ input: { "data-testid": "announce-toggle" } }}
              />
              <Divider sx={{ my: 2 }} />
              <Checkbox
                label="Archive this channel (read-only, hidden from the sidebar)"
                checked={channel.archived}
                onChange={(e) => void run(() => teams.updateChannel(channel.id, { archived: e.target.checked }))}
              />
            </TabPanel>
          )}
        </Tabs>
        {err && (
          <Typography level="body-sm" color="danger">
            {err}
          </Typography>
        )}
      </ModalDialog>
    </Modal>
  );
}

// ---- profile ----------------------------------------------------------------

export function ProfileDialog({
  open,
  onClose,
  person,
  editable,
  presence,
  onDM,
  onSaved,
}: {
  open: boolean;
  onClose: () => void;
  person: Person | null;
  editable: boolean;
  presence: "online" | "away" | "offline";
  onDM: (id: string) => void;
  onSaved: () => void;
}) {
  const [title, setTitle] = useState(person?.title ?? "");
  const [status, setStatus] = useState(person?.status_text ?? "");
  const [emoji, setEmoji] = useState(person?.status_emoji ?? "");
  if (!person) return null;
  return (
    <Modal open={open} onClose={onClose}>
      <ModalDialog sx={{ width: 380, maxWidth: "95vw" }}>
        <ModalClose />
        <Box sx={{ display: "flex", gap: 2, alignItems: "center" }}>
          <UserAvatar id={person.id} name={person.name} size={64} />
          <Box>
            <Typography level="title-lg">{person.name}</Typography>
            <Typography level="body-sm">{person.title || "No title set"}</Typography>
            <Typography level="body-xs" sx={{ color: presence === "online" ? "success.600" : "neutral.500" }}>
              ● {presence === "online" ? "Active" : presence === "away" ? "Away" : "Offline"}
            </Typography>
          </Box>
        </Box>
        {person.status_text && (
          <Typography level="body-sm">
            {person.status_emoji} {person.status_text}
          </Typography>
        )}
        <Typography level="body-sm">{person.email}</Typography>
        {editable ? (
          <>
            <FormControl>
              <FormLabel>Title</FormLabel>
              <Input value={title} onChange={(e) => setTitle(e.target.value)} slotProps={{ input: { "aria-label": "Title", maxLength: 80 } }} />
            </FormControl>
            <Box sx={{ display: "flex", gap: 1 }}>
              <Input sx={{ width: 70 }} placeholder="🙂" value={emoji} onChange={(e) => setEmoji(e.target.value)} slotProps={{ input: { "aria-label": "Status emoji", maxLength: 8 } }} />
              <Input sx={{ flex: 1 }} placeholder="What's your status?" value={status} onChange={(e) => setStatus(e.target.value)} slotProps={{ input: { "aria-label": "Status", maxLength: 100 } }} />
            </Box>
            <Button
              onClick={async () => {
                await teams.setProfile({ title, status_text: status, status_emoji: emoji });
                onSaved();
                onClose();
              }}
            >
              Save profile
            </Button>
          </>
        ) : (
          <Button onClick={() => { onDM(person.id); onClose(); }}>Message</Button>
        )}
      </ModalDialog>
    </Modal>
  );
}
