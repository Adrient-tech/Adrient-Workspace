// ---------------------------------------------------------------------------
// Groups — admin view of the org's Groups (Google Groups clone: mailing
// lists / forums). Reuses the existing member-facing GroupsService REST gateway
// (web/app/src/pages/groups/api.ts) since admins are org members; the admin
// console page is admin-gated at the nav level. Mirrors the styling of the
// other admin sections (Users, Sessions) in ./index.tsx.
// ---------------------------------------------------------------------------
import { LoadOlder } from "../../components/LoadOlder";
import { searchDirectoryPage, type DirectoryEntry } from "../../api/directorySearch";
import { PeopleSelect } from "../../components/PeoplePicker";
import { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Avatar,
  Box,
  Button,
  Chip,
  CircularProgress,
  Divider,
  Dropdown,
  FormControl,
  FormHelperText,
  FormLabel,
  IconButton,
  Checkbox,
  Input,
  ListItemDecorator,
  Menu,
  MenuButton,
  MenuItem,
  Modal,
  ModalClose,
  ModalDialog,
  Sheet,
  Table,
  Typography,
} from "@mui/joy";
import * as Icons from "@mui/icons-material";
import SearchIcon from "@mui/icons-material/Search";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import GroupIcon from "@mui/icons-material/Group";
import {
  createGroup,
  updateGroup,
  deleteGroup,
  listMembers,
  getGroup,
} from "../groups/api";
import type { Group, GroupMember } from "../groups/types";
import { GroupPanel } from "./GroupPanel";

/** A directory search hit as a table row (enough for the list; edit loads the full group). */
function groupRow(e: DirectoryEntry): Group {
  return {
    id: e.id,
    org_id: "",
    name: e.name,
    email: e.email ?? "",
    description: e.detail ?? "",
    member_ids: [],
    member_count: e.count ?? 0,
    topic_count: e.topics ?? 0,
    post_count: 0,
    created_at: "",
    updated_at: "",
  };
}

export function GroupsSection() {
  const [groups, setGroups] = useState<Group[] | null>(null);
  const [members, setMembers] = useState<GroupMember[]>([]);
  const [query, setQuery] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<Group | null>(null);
  const [panel, setPanel] = useState<Group | null>(null);
  const [picked, setPicked] = useState<Set<string>>(new Set());
  const [nextGroups, setNextGroups] = useState("");
  const [queryD, setQueryD] = useState("");
  useEffect(() => {
    const t = setTimeout(() => setQueryD(query.trim()), 250);
    return () => clearTimeout(t);
  }, [query]);
  async function openEdit(id: string) {
    try {
      setEditing(await getGroup(id));
    } catch (e) {
      setError((e as Error).message);
    }
  }

  const load = useCallback(async () => {
    setError(null);
    try {
      // Groups come a page at a time from the directory search (server-side
      // filter by name or address), so thousands of groups never load at once.
      const [page, ms] = await Promise.all([searchDirectoryPage("group", queryD, undefined, undefined, { limit: 50 }), listMembers()]);
      setGroups(page.items.map(groupRow));
      setNextGroups(page.next_cursor ?? "");
      setMembers(ms);
    } catch (e) {
      setGroups([]);
      setError((e as Error).message);
    }
  }, [queryD]);

  useEffect(() => {
    void load();
  }, [load]);

  async function onDelete(g: Group) {
    if (
      !window.confirm(
        `Delete the group “${g.name || g.email || "this group"}”?\n\n` +
          `This removes the group and all of its topics and posts. This can’t be undone.`,
      )
    )
      return;
    setBusyId(g.id);
    setError(null);
    try {
      await deleteGroup(g.id);
      setNotice(`Deleted group “${g.name || g.email}”.`);
      await load();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusyId(null);
    }
  }

  async function onBulkDelete() {
    const list = (groups ?? []).filter((g) => picked.has(g.id));
    if (!list.length) return;
    if (!window.confirm(`Delete ${list.length} ${list.length === 1 ? "group" : "groups"}?\n\n${list.map((g) => g.name || g.email).join(", ")}\n\nThis removes their topics and posts and can't be undone.`)) return;
    setError(null);
    let ok = 0;
    const failed: string[] = [];
    for (const g of list) {
      try {
        await deleteGroup(g.id);
        ok++;
      } catch (e) {
        failed.push(`${g.name || g.email}: ${(e as Error).message}`);
      }
    }
    setPicked(new Set());
    setNotice(`Deleted ${ok} ${ok === 1 ? "group" : "groups"}.`);
    if (failed.length) setError(failed.join("; "));
    await load();
  }

  const shown = groups ?? [];

  return (
    <>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1.5,
          mb: 2,
          flexWrap: "wrap",
        }}
      >
        <Box sx={{ flex: 1, minWidth: 180 }}>
          <Typography level="h4">Groups</Typography>
          <Typography level="body-sm" sx={{ opacity: 0.7 }}>
            Mailing lists and forums for your organization.
          </Typography>
        </Box>
        <Input
          size="sm"
          startDecorator={<SearchIcon />}
          placeholder="Search groups"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          sx={{ width: { xs: "100%", sm: 220 }, order: { xs: 10, sm: "unset" } }}
        />
        <Button
          size="sm"
          startDecorator={<GroupIcon />}
          onClick={() => setCreateOpen(true)}
          data-testid="admin-create-group"
        >
          Create group
        </Button>
      </Box>

      {notice && (
        <Alert
          color="success"
          variant="soft"
          sx={{ mb: 2 }}
          endDecorator={
            <IconButton
              size="sm"
              variant="plain"
              onClick={() => setNotice(null)}
            >
              <Icons.Close />
            </IconButton>
          }
        >
          {notice}
        </Alert>
      )}
      {error && (
        <Alert color="danger" variant="soft" sx={{ mb: 2 }}>
          {error}
        </Alert>
      )}

      {groups === null && !error && (
        <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
          <CircularProgress />
        </Box>
      )}

      {picked.size > 0 && (
        <Box data-testid="group-bulk-bar" sx={{ display: "flex", alignItems: "center", gap: 1, p: 1, mb: 1, borderRadius: "md", bgcolor: "primary.softBg" }}>
          <Typography level="body-sm" sx={{ fontWeight: 600, mr: 1 }}>{picked.size} selected</Typography>
          <Button size="sm" variant="outlined" color="danger" onClick={() => void onBulkDelete()} data-testid="group-bulk-delete">Delete</Button>
          <Button size="sm" variant="plain" color="neutral" onClick={() => setPicked(new Set())}>Clear selection</Button>
        </Box>
      )}
      <GroupPanel group={panel} members={members} onClose={() => setPanel(null)}
        actions={panel && <><Button size="sm" variant="outlined" onClick={() => { setEditing(panel); setPanel(null); }}>Edit</Button></>} />
      {groups !== null && (
        <Sheet
          variant="outlined"
          sx={{ borderRadius: "md", overflow: "hidden", overflowX: "auto" }}
        >
          <Table
            hoverRow
            sx={{ "--TableCell-paddingX": { xs: "8px", sm: "16px" }, minWidth: 520 }}
          >
            <thead>
              <tr>
                <th style={{ width: 40 }}>
                  <Checkbox size="sm" aria-label="Select all groups" checked={shown.length > 0 && shown.every((g) => picked.has(g.id))} indeterminate={picked.size > 0 && !shown.every((g) => picked.has(g.id))}
                    onChange={(e) => setPicked(e.target.checked ? new Set(shown.map((g) => g.id)) : new Set())} />
                </th>
                <th style={{ width: "40%" }}>Name</th>
                <th>Email</th>
                <th style={{ width: 90 }}>Members</th>
                <th style={{ width: 90 }}>Topics</th>
                <th style={{ width: 48 }} aria-label="Actions" />
              </tr>
            </thead>
            <tbody>
              {shown.map((g) => (
                <tr key={g.id} data-testid={`admin-group-${g.id}`}>
                  <td>
                    <Checkbox size="sm" aria-label={`Select ${g.name || g.email}`} checked={picked.has(g.id)}
                      onChange={(e) => setPicked((c) => { const n = new Set(c); if (e.target.checked) n.add(g.id); else n.delete(g.id); return n; })} />
                  </td>
                  <td>
                    <Box
                      onClick={() => setPanel(g)}
                      data-testid={`open-group-${g.id}`}
                      sx={{ display: "flex", alignItems: "center", gap: 1.25, cursor: "pointer", "&:hover p": { textDecoration: "underline" } }}
                    >
                      <Avatar size="sm">
                        {(g.name || g.email || "?").charAt(0).toUpperCase()}
                      </Avatar>
                      <Box sx={{ minWidth: 0 }}>
                        <Typography
                          level="body-sm"
                          sx={{ fontWeight: 500 }}
                          noWrap
                        >
                          {g.name || "(unnamed group)"}
                        </Typography>
                        {g.description && (
                          <Typography
                            level="body-xs"
                            sx={{ opacity: 0.6 }}
                            noWrap
                          >
                            {g.description}
                          </Typography>
                        )}
                      </Box>
                    </Box>
                  </td>
                  <td>
                    <Typography level="body-sm" noWrap>
                      {g.email || "—"}
                    </Typography>
                  </td>
                  <td>
                    <Chip size="sm" variant="soft">
                      {g.member_count}
                    </Chip>
                  </td>
                  <td>
                    <Typography level="body-sm">{g.topic_count}</Typography>
                  </td>
                  <td>
                    <Dropdown>
                      <MenuButton
                        slots={{ root: IconButton }}
                        slotProps={{
                          root: {
                            size: "sm",
                            variant: "plain",
                            color: "neutral",
                            loading: busyId === g.id,
                          },
                        }}
                        data-testid={`admin-group-menu-${g.id}`}
                      >
                        <MoreVertIcon />
                      </MenuButton>
                      <Menu placement="bottom-end" size="sm">
                        <MenuItem onClick={() => void openEdit(g.id)}>
                          <ListItemDecorator>
                            <Icons.Edit />
                          </ListItemDecorator>{" "}
                          Edit
                        </MenuItem>
                        <Divider />
                        <MenuItem color="danger" onClick={() => onDelete(g)}>
                          <ListItemDecorator>
                            <Icons.DeleteForever />
                          </ListItemDecorator>{" "}
                          Delete
                        </MenuItem>
                      </Menu>
                    </Dropdown>
                  </td>
                </tr>
              ))}
            </tbody>
          </Table>
          <LoadOlder
            next={nextGroups}
            shown={shown.length}
            label="Load more groups"
            testId="admin-groups-more"
            loadMore={async (c) => {
              const page = await searchDirectoryPage("group", queryD, c, undefined, { limit: 50 });
              setGroups((gs) => [...(gs ?? []), ...page.items.map(groupRow)]);
              setNextGroups(page.next_cursor ?? "");
            }}
          />
          {shown.length === 0 && (
            <Box sx={{ p: 4, textAlign: "center" }}>
              <Typography level="body-sm" sx={{ opacity: 0.6 }}>
                {query
                  ? "No groups match your search."
                  : "No groups yet. Create the first one."}
              </Typography>
            </Box>
          )}
        </Sheet>
      )}

      {createOpen && (
        <GroupFormDialog
          mode="create"
          members={members}
          onClose={() => setCreateOpen(false)}
          onSaved={async (name) => {
            setCreateOpen(false);
            setNotice(`Created group “${name}”.`);
            await load();
          }}
        />
      )}
      {editing && (
        <GroupFormDialog
          mode="edit"
          group={editing}
          members={members}
          onClose={() => setEditing(null)}
          onSaved={async (name) => {
            setEditing(null);
            setNotice(`Updated group “${name}”.`);
            await load();
          }}
        />
      )}
    </>
  );
}

// Shared create/edit dialog. Collects name/email/description and a member set
// (checkbox list of org members). Both modes send the full editable field set,
// matching the GroupsService Create/Update RPCs.
function GroupFormDialog(props: {
  mode: "create" | "edit";
  group?: Group;
  members: GroupMember[];
  onClose: () => void;
  onSaved: (name: string) => Promise<void> | void;
}) {
  const { mode, group, members } = props;
  const [name, setName] = useState(group?.name ?? "");
  const [email, setEmail] = useState(group?.email ?? "");
  const [description, setDescription] = useState(group?.description ?? "");
  const [memberIds, setMemberIds] = useState<string[]>(group?.member_ids ?? []);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function submit() {
    setSaving(true);
    setError(null);
    try {
      const fields = {
        name: name.trim(),
        email: email.trim(),
        description: description.trim(),
        member_ids: memberIds,
      };
      if (mode === "create") await createGroup(fields);
      else if (group) await updateGroup(group.id, fields);
      await props.onSaved(fields.name || fields.email);
    } catch (e) {
      setError((e as Error).message);
      setSaving(false);
    }
  }

  return (
    <Modal open onClose={props.onClose}>
      <ModalDialog
        sx={{
          width: { xs: "100vw", sm: 460 },
          maxWidth: "100vw",
          borderRadius: { xs: 0, sm: "md" },
        }}
      >
        <ModalClose />
        <Typography level="title-lg">
          {mode === "create" ? "Create group" : "Edit group"}
        </Typography>
        {error && (
          <Alert color="danger" variant="soft" sx={{ mt: 1 }}>
            {error}
          </Alert>
        )}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, mt: 1.5 }}>
          <FormControl>
            <FormLabel>Name</FormLabel>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Engineering"
              autoFocus
            />
          </FormControl>
          <FormControl>
            <FormLabel>Email</FormLabel>
            <Input
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="engineering@yourdomain.com"
            />
            <FormHelperText>The list address members post to.</FormHelperText>
          </FormControl>
          <FormControl>
            <FormLabel>Description</FormLabel>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What this group is for"
            />
          </FormControl>

          <FormControl>
            <FormLabel>
              Members{" "}
              <Typography component="span" level="body-xs" sx={{ opacity: 0.6 }}>
                ({memberIds.length} selected)
              </Typography>
            </FormLabel>
            <PeopleSelect
              value={memberIds}
              known={members}
              onChange={(ids) => setMemberIds(ids)}
              ariaLabel="Group members"
              placeholder="Add people by name, email or ID"
              maxChips={12}
              testId="admin-group-members"
            />
          </FormControl>
        </Box>

        <Box
          sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2.5 }}
        >
          <Button
            variant="plain"
            color="neutral"
            onClick={props.onClose}
            disabled={saving}
          >
            Cancel
          </Button>
          <Button
            onClick={submit}
            loading={saving}
            disabled={!name.trim() && !email.trim()}
            data-testid="admin-group-save"
          >
            {mode === "create" ? "Create" : "Save"}
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
