import { useEffect, useRef, useState } from "react";
import {
  Box,
  Container,
  Typography,
  Input,
  Sheet,
  IconButton,
  CircularProgress,
  List,
  ListItem,
  ListItemButton,
  ListItemDecorator,
  Divider,
  Checkbox,
  Button,
  Tooltip,
  Modal,
  ModalDialog,
  Dropdown,
  Menu,
  MenuButton,
  MenuItem,
  ListDivider,
  Chip,
  ChipDelete,
} from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import EditOutlinedIcon from "@mui/icons-material/EditOutlined";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import MoreVertIcon from "@mui/icons-material/MoreVert";
import TaskAltIcon from "@mui/icons-material/TaskAlt";
import { Header } from "../../components/Header";
import type { User } from "../../api/types";
import {
  listTaskLists,
  createTaskList,
  updateTaskList,
  deleteTaskList,
  listTasks,
  createTask,
  updateTask,
  deleteTask,
  toggleTask,
} from "./api";
import type { TaskList, Task } from "./types";
import { TaskEditor, type EditorResult } from "./TaskEditor";
import { BoardView, TaskCalendar } from "./Views";
import { ShareDialog } from "./ShareDialog";
import { TasksAssistant } from "./ai/TasksAssistant";
import * as ext from "./extrasApi";
import { dueFromDay, emptyExtras, isOverdue, quickDue, recurrenceLabel, subtaskProgress, PRIORITIES, type Column, type QuickWhen, type TaskExtras } from "./logic";
import ShareOutlinedIcon from "@mui/icons-material/ShareOutlined";
import EventIcon from "@mui/icons-material/EventOutlined";
import { parseQuickAdd } from "./quickparse";
import RepeatIcon from "@mui/icons-material/Repeat";

interface TasksAppProps {
  user: User;
}

export default function TasksApp({ user }: TasksAppProps) {
  const [lists, setLists] = useState<TaskList[] | null>(null);
  const [selectedListId, setSelectedListId] = useState<string | null>(null);
  const [tasks, setTasks] = useState<Task[]>([]);
  const [loadingTasks, setLoadingTasks] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [completedExpanded, setCompletedExpanded] = useState(false);

  // New list input
  const [newListName, setNewListName] = useState("");
  const [addingList, setAddingList] = useState(false);

  // Rename list modal
  const [renaming, setRenaming] = useState<TaskList | null>(null);
  const [renameValue, setRenameValue] = useState("");

  // New task input
  const [newTaskTitle, setNewTaskTitle] = useState("");
  const [quickIgnore, setQuickIgnore] = useState<Set<string>>(new Set());
  const [tagFilter, setTagFilter] = useState<string | null>(null);

  // Edit task modal
  const [editingTask, setEditingTask] = useState<Task | null>(null);

  const newTaskRef = useRef<HTMLInputElement>(null);
  const [extras, setExtras] = useState<Record<string, TaskExtras>>({});
  const [view, setView] = useState<"list" | "board" | "calendar">("list");
  const [shared, setShared] = useState<ext.SharedList[]>([]);
  const [assignedTasks, setAssignedTasks] = useState<ext.AssignedTask[]>([]);
  const [shareOpen, setShareOpen] = useState(false);
  const ASSIGNED = "__assigned__";

  async function reloadLists() {
    try {
      const ls = await listTaskLists();
      setLists(ls);
      if (ls.length > 0 && !selectedListId) {
        setSelectedListId(ls[0].id);
      }
    } catch (e) {
      setError((e as Error).message);
    }
  }

  async function reloadShared() {
    ext.sharedLists().then(setShared).catch(() => setShared([]));
    ext.assigned().then(setAssignedTasks).catch(() => setAssignedTasks([]));
  }

  async function reloadTasks(listId: string) {
    if (listId === ASSIGNED) {
      setTasks([]);
      setExtras({});
      return;
    }
    setLoadingTasks(true);
    try {
      const [ts, ex] = await Promise.all([listTasks(listId), ext.listExtras(listId).catch(() => ({}) as Record<string, TaskExtras>)]);
      setExtras(ex);
      setTasks(ts);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoadingTasks(false);
    }
  }

  useEffect(() => {
    reloadLists();
    void reloadShared();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (selectedListId) reloadTasks(selectedListId);
    else setTasks([]);
  }, [selectedListId]); // eslint-disable-line react-hooks/exhaustive-deps

  const sharedSel = shared.find((l) => l.id === selectedListId) ?? null;
  const selectedList =
    (lists ?? []).find((l) => l.id === selectedListId) ??
    (sharedSel ? ({ id: sharedSel.id, name: sharedSel.name, org_id: "", owner_user_id: "", position: 0, created_at: "" } as TaskList) : null);
  const isOwner = !!selectedList && !sharedSel;
  const readOnly = sharedSel?.role === "viewer";
  const overdue = tasks.filter((t) => !t.parent_task_id && isOverdue(t));

  // Partition tasks
  const listTags = Array.from(new Set(Object.values(extras).flatMap((x) => x.tags ?? []))).sort();
  const activeTasks = tasks.filter((t) => !t.completed && !t.parent_task_id && (!tagFilter || (extras[t.id]?.tags ?? []).includes(tagFilter)));
  const subtasksFor = (parentId: string) =>
    tasks.filter((t) => t.parent_task_id === parentId);
  const completedTasks = tasks.filter((t) => t.completed);

  // ---- Optimistic mutations ----
  function patchTask(id: string, patch: Partial<Task>) {
    setTasks((cur) => cur.map((t) => (t.id === id ? { ...t, ...patch } : t)));
  }

  async function handleToggle(task: Task) {
    patchTask(task.id, {
      completed: !task.completed,
      completed_at: !task.completed ? new Date().toISOString() : "",
    });
    try {
      const updated = await toggleTask(task.list_id, task.id);
      patchTask(task.id, updated);
    } catch {
      if (selectedListId) reloadTasks(selectedListId);
    }
  }

  async function handleDeleteTask(task: Task) {
    setTasks((cur) => cur.filter((t) => t.id !== task.id));
    try {
      await deleteTask(task.list_id, task.id);
    } catch {
      if (selectedListId) reloadTasks(selectedListId);
    }
  }

  const quickDraft = parseQuickAdd(newTaskTitle, {
    lists: (lists ?? []).map((l) => ({ id: l.id, name: l.name })),
    ignore: quickIgnore,
  });

  async function handleAddTask() {
    const d = quickDraft;
    const title = d.title || newTaskTitle.trim();
    if (!title) return;
    const listId = d.listId ?? selectedListId;
    if (!listId || listId === ASSIGNED) return;
    setNewTaskTitle("");
    setQuickIgnore(new Set());
    try {
      const due_at = d.due ? dueFromDay(d.due, d.dueTime) : "";
      const t = await createTask(listId, { title, due_at });
      if (d.priority || d.repeat || d.tags.length) {
        const x = { ...emptyExtras(t.id), priority: d.priority, recurrence: d.repeat ?? "", tags: d.tags };
        const saved = await ext.saveExtras(t.id, x);
        if (listId === selectedListId) setExtras((cur) => ({ ...cur, [t.id]: { ...x, ...saved } }));
      }
      if (listId === selectedListId) setTasks((cur) => [...cur, t]);
      else setSelectedListId(listId);
    } catch (e) {
      setError((e as Error).message);
    }
  }

  async function handleSaveEditor(r: EditorResult) {
    if (!editingTask) return;
    const t = editingTask;
    const updated = await updateTask(t.list_id, t.id, { title: r.title, notes: r.notes, due_at: r.due_at });
    patchTask(t.id, updated);
    const before = extras[t.id];
    const changed =
      !before ||
      before.assignee_id !== r.extras.assignee_id ||
      before.priority !== r.extras.priority ||
      before.recurrence !== r.extras.recurrence ||
      (before.tags ?? []).join(",") !== (r.extras.tags ?? []).join(",") ||
      before.remind_minutes !== r.extras.remind_minutes;
    if (changed) {
      const saved = await ext.saveExtras(t.id, r.extras);
      setExtras((cur) => ({ ...cur, [t.id]: { ...r.extras, ...saved, board_status: r.extras.board_status } }));
      if (saved.assignee_id !== (before?.assignee_id ?? "")) void reloadTasks(t.list_id);
    }
    void reloadShared();
  }

  async function handleAddSubtask(title: string) {
    if (!editingTask) return;
    const st = await createTask(editingTask.list_id, { title, parent_task_id: editingTask.id });
    setTasks((cur) => [...cur, st]);
  }

  function openEdit(task: Task) {
    setEditingTask(task);
  }

  async function handleReschedule(task: Task, when: QuickWhen) {
    const due = quickDue(when);
    patchTask(task.id, { due_at: due });
    try {
      await updateTask(task.list_id, task.id, { due_at: due });
    } catch {
      if (selectedListId) reloadTasks(selectedListId);
    }
  }

  async function rescheduleOverdue() {
    const due = quickDue("today");
    setTasks((cur) => cur.map((t) => (!t.parent_task_id && isOverdue(t) ? { ...t, due_at: due } : t)));
    await Promise.all(overdue.map((t) => updateTask(t.list_id, t.id, { due_at: due }).catch(() => undefined)));
  }

  async function handleMove(task: Task, to: Column) {
    const was = task.completed ? "done" : extras[task.id]?.board_status === "doing" ? "doing" : "todo";
    if (was === to) return;
    if (to === "done") {
      await handleToggle(task);
      return;
    }
    if (task.completed) await handleToggle(task);
    const x = { ...(extras[task.id] ?? { task_id: task.id, assignee_id: "", priority: 0, recurrence: "", remind_minutes: null, link_url: "" }), board_status: to } as TaskExtras;
    setExtras((cur) => ({ ...cur, [task.id]: x }));
    try {
      await ext.saveExtras(task.id, x);
    } catch {
      if (selectedListId) reloadTasks(selectedListId);
    }
  }

  // ---- List management ----
  async function handleAddList() {
    const name = newListName.trim();
    if (!name) return;
    setNewListName("");
    setAddingList(false);
    try {
      const l = await createTaskList({ name });
      setLists((cur) => [...(cur ?? []), l]);
      setSelectedListId(l.id);
    } catch (e) {
      setError((e as Error).message);
    }
  }

  async function handleRenameList() {
    if (!renaming) return;
    const name = renameValue.trim();
    if (!name) return;
    try {
      const updated = await updateTaskList(renaming.id, { name });
      setLists((cur) =>
        (cur ?? []).map((l) => (l.id === updated.id ? updated : l)),
      );
      setRenaming(null);
    } catch (e) {
      setError((e as Error).message);
    }
  }

  async function handleDeleteList(list: TaskList) {
    if (!window.confirm(`Delete list "${list.name}" and all its tasks?`))
      return;
    try {
      await deleteTaskList(list.id);
      setLists((cur) => (cur ?? []).filter((l) => l.id !== list.id));
      if (selectedListId === list.id) {
        const remaining = (lists ?? []).filter((l) => l.id !== list.id);
        setSelectedListId(remaining.length > 0 ? remaining[0].id : null);
      }
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <>
      <Header user={user} />
      <Container
        maxWidth="lg"
        sx={{ py: { xs: 2, sm: 4 }, px: { xs: 1.5, sm: 3 } }}
      >
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, mb: 3 }}>
          <TaskAltIcon sx={{ fontSize: 28, color: "primary.main" }} />
          <Typography
            level="h2"
            sx={{ flex: 1, fontSize: { xs: "xl", sm: "xl3" } }}
          >
            Tasks
          </Typography>
          <Button
            component="a"
            href="/api/v1/tx/export.ics"
            download="adrient-tasks.ics"
            size="sm"
            variant="outlined"
            color="neutral"
            startDecorator={<EventIcon />}
            data-testid="tasks-export-ics"
          >
            Export .ics
          </Button>
        </Box>

        {error && (
          <Sheet
            color="danger"
            variant="soft"
            sx={{ p: 2, mb: 2, borderRadius: "md" }}
          >
            <Typography color="danger">{error}</Typography>
          </Sheet>
        )}

        {/* Phones: the list sidebar is hidden, so lists are switched here. */}
        <Box sx={{ display: { xs: "block", sm: "none" }, mb: 1.5 }}>
          <select
            aria-label="Choose a list"
            data-testid="mobile-list-select"
            value={selectedListId ?? ""}
            onChange={(e) => setSelectedListId(e.target.value || null)}
            style={{ width: "100%", padding: 10, borderRadius: 8, fontSize: 16 }}
          >
            {(lists ?? []).map((l) => (
              <option key={l.id} value={l.id}>{l.name}</option>
            ))}
            {shared.map((l) => (
              <option key={l.id} value={l.id}>{l.name} (shared by {l.owner_name})</option>
            ))}
            <option value={ASSIGNED}>Assigned to me</option>
          </select>
        </Box>
        <Box sx={{ display: "flex", gap: 2, alignItems: "flex-start" }}>
          {/* Left panel: task lists */}
          <Sheet
            variant="outlined"
            sx={{
              width: 220,
              flexShrink: 0,
              borderRadius: "md",
              display: { xs: "none", sm: "flex" },
              flexDirection: "column",
            }}
          >
            <Typography
              level="body-xs"
              sx={{
                px: 2,
                pt: 1.5,
                pb: 0.5,
                opacity: 0.6,
                textTransform: "uppercase",
                letterSpacing: 1,
              }}
            >
              My lists
            </Typography>
            <List
              size="sm"
              sx={{ "--ListItem-radius": "6px", px: 0.5, pb: 0.5 }}
            >
              {(lists ?? []).map((l) => (
                <ListItem
                  key={l.id}
                  endAction={
                    <Dropdown>
                      <MenuButton
                        slots={{ root: IconButton }}
                        slotProps={{
                          root: {
                            size: "sm",
                            variant: "plain",
                            "aria-label": "List options",
                          },
                        }}
                      >
                        <MoreVertIcon sx={{ fontSize: 16 }} />
                      </MenuButton>
                      <Menu size="sm" placement="bottom-end">
                        <MenuItem
                          onClick={() => {
                            setRenaming(l);
                            setRenameValue(l.name);
                          }}
                        >
                          <ListItemDecorator>
                            <EditOutlinedIcon />
                          </ListItemDecorator>
                          Rename
                        </MenuItem>
                        <ListDivider />
                        <MenuItem
                          color="danger"
                          onClick={() => handleDeleteList(l)}
                        >
                          <ListItemDecorator>
                            <DeleteOutlineIcon />
                          </ListItemDecorator>
                          Delete list
                        </MenuItem>
                      </Menu>
                    </Dropdown>
                  }
                >
                  <ListItemButton
                    selected={l.id === selectedListId}
                    onClick={() => setSelectedListId(l.id)}
                    sx={{ borderRadius: "6px" }}
                  >
                    <Typography noWrap sx={{ fontSize: "sm" }}>
                      {l.name}
                    </Typography>
                  </ListItemButton>
                </ListItem>
              ))}
            </List>

            <Box sx={{ px: 0.5 }}>
              <List size="sm" sx={{ "--ListItem-radius": "6px" }}>
                <ListItem>
                  <ListItemButton
                    selected={selectedListId === ASSIGNED}
                    data-testid="assigned-tab"
                    onClick={() => {
                      void reloadShared();
                      setSelectedListId(ASSIGNED);
                    }}
                  >
                    <Typography noWrap sx={{ fontSize: "sm", flex: 1 }}>
                      Assigned to me
                    </Typography>
                    {assignedTasks.length > 0 && <Chip size="sm" variant="soft">{assignedTasks.length}</Chip>}
                  </ListItemButton>
                </ListItem>
              </List>
              {shared.length > 0 && (
                <>
                  <Typography level="body-xs" sx={{ px: 1.5, pb: 0.5, opacity: 0.6, textTransform: "uppercase", letterSpacing: 1 }}>
                    Shared with me
                  </Typography>
                  <List size="sm" sx={{ "--ListItem-radius": "6px" }}>
                    {shared.map((l) => (
                      <ListItem key={l.id}>
                        <ListItemButton selected={l.id === selectedListId} onClick={() => setSelectedListId(l.id)} data-testid="shared-list">
                          <Box sx={{ minWidth: 0 }}>
                            <Typography noWrap sx={{ fontSize: "sm" }}>{l.name}</Typography>
                            <Typography noWrap level="body-xs">{l.owner_name}{l.role === "viewer" ? " · view only" : ""}</Typography>
                          </Box>
                        </ListItemButton>
                      </ListItem>
                    ))}
                  </List>
                </>
              )}
            </Box>

            {addingList ? (
              <Box sx={{ px: 1, pb: 1, display: "flex", gap: 0.5 }}>
                <Input
                  size="sm"
                  autoFocus
                  placeholder="List name"
                  value={newListName}
                  onChange={(e) => setNewListName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") handleAddList();
                    if (e.key === "Escape") setAddingList(false);
                  }}
                  sx={{ flex: 1 }}
                />
                <Button size="sm" onClick={handleAddList}>
                  Add
                </Button>
              </Box>
            ) : (
              <Box sx={{ px: 1, pb: 1 }}>
                <Button
                  size="sm"
                  variant="plain"
                  color="neutral"
                  startDecorator={<AddIcon />}
                  onClick={() => setAddingList(true)}
                  sx={{ width: "100%", justifyContent: "flex-start" }}
                >
                  New list
                </Button>
              </Box>
            )}
          </Sheet>

          {/* Main: task list panel */}
          <Box sx={{ flex: 1, minWidth: 0 }}>
            {lists === null && !error && (
              <Box sx={{ display: "flex", justifyContent: "center", py: 6 }}>
                <CircularProgress />
              </Box>
            )}

            {selectedListId === ASSIGNED && (
              <Box data-testid="assigned-view">
                <Typography level="h3" sx={{ mb: 2 }}>Assigned to me</Typography>
                {assignedTasks.length === 0 && (
                  <Sheet variant="soft" sx={{ p: 3, borderRadius: "md", textAlign: "center" }}>
                    <Typography level="body-md" sx={{ opacity: 0.6 }}>Nothing is assigned to you.</Typography>
                  </Sheet>
                )}
                <List size="sm" sx={{ "--List-gap": "2px" }}>
                  {assignedTasks.map((a) => (
                    <ListItem key={a.id} endAction={<Button size="sm" variant="plain" onClick={() => setSelectedListId(a.list_id)}>Open list</Button>}>
                      <Box>
                        <Typography level="body-sm">{a.title}</Typography>
                        <Typography level="body-xs" color={a.due_at && new Date(a.due_at) < new Date() ? "danger" : "neutral"}>
                          {a.list_name}{a.due_at ? ` · ${formatDue(a.due_at)}` : ""}
                        </Typography>
                      </Box>
                    </ListItem>
                  ))}
                </List>
              </Box>
            )}

            {lists !== null && selectedList === null && selectedListId !== ASSIGNED && (
              <Sheet
                variant="soft"
                sx={{ p: 4, borderRadius: "md", textAlign: "center" }}
              >
                <Typography level="body-lg" sx={{ opacity: 0.7 }}>
                  Create a list to get started.
                </Typography>
              </Sheet>
            )}

            {selectedList && (
              <>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 2, flexWrap: "wrap" }}>
                  <Typography level="h3" sx={{ flex: 1, minWidth: 120 }}>
                    {selectedList.name}
                  </Typography>
                  <Box sx={{ display: "flex", gap: 0.5 }} role="group" aria-label="View">
                    {(["list", "board", "calendar"] as const).map((v) => (
                      <Button key={v} size="sm" variant={view === v ? "solid" : "outlined"} color={view === v ? "primary" : "neutral"} onClick={() => setView(v)} data-testid={`view-${v}`} sx={{ textTransform: "capitalize" }}>
                        {v}
                      </Button>
                    ))}
                  </Box>
                  {isOwner && (
                    <Button size="sm" variant="outlined" color="neutral" startDecorator={<ShareOutlinedIcon />} onClick={() => setShareOpen(true)} data-testid="share-list">
                      Share
                    </Button>
                  )}
                  <TasksAssistant listId={selectedList.id} listName={selectedList.name} tasks={tasks} reload={() => void reloadTasks(selectedList.id)} />
                </Box>
                {readOnly && (
                  <Typography level="body-sm" sx={{ mb: 1 }}>You can view this list but not change it.</Typography>
                )}
                {overdue.length > 0 && !readOnly && view === "list" && (
                  <Sheet color="warning" variant="soft" sx={{ p: 1, mb: 1.5, borderRadius: "md", display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }} data-testid="overdue-banner">
                    <Typography level="body-sm" sx={{ flex: 1 }}>{overdue.length} overdue</Typography>
                    <Button size="sm" variant="outlined" color="warning" onClick={rescheduleOverdue} data-testid="overdue-today">Move to today</Button>
                  </Sheet>
                )}

                {/* Add task input */}
                {!readOnly && (
                <Sheet
                  variant="outlined"
                  sx={{
                    borderRadius: "md",
                    display: "flex",
                    alignItems: "center",
                    px: 1.5,
                    py: 0.75,
                    mb: 2,
                    gap: 1,
                  }}
                >
                  <AddIcon sx={{ opacity: 0.4 }} />
                  <Input
                    ref={newTaskRef}
                    variant="plain"
                    placeholder='Add a task, e.g. "pay rent friday 9am every month !high #home"'
                    value={newTaskTitle}
                    onChange={(e) => setNewTaskTitle(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") handleAddTask();
                    }}
                    sx={{ flex: 1, "--Input-focusedThickness": "0px" }}
                  />
                  {newTaskTitle && (
                    <Button size="sm" onClick={handleAddTask}>
                      Add
                    </Button>
                  )}
                </Sheet>
                )}
                {!readOnly && quickDraft.tokens.length > 0 && (
                  <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mb: 1.5, mt: -1 }} data-testid="quick-chips">
                    {quickDraft.tokens.map((tk) => (
                      <Chip
                        key={tk.key}
                        size="sm"
                        variant="soft"
                        color="primary"
                        endDecorator={
                          <ChipDelete
                            aria-label={`Keep "${tk.text}" as text`}
                            onDelete={() => setQuickIgnore((cur) => new Set(cur).add(tk.key))}
                          />
                        }
                      >
                        {tk.type === "due" ? `Due ${tk.label}` : tk.type === "time" ? `At ${tk.label}` : tk.type === "list" ? `List ${tk.label}` : tk.label}
                      </Chip>
                    ))}
                  </Box>
                )}
                {listTags.length > 0 && (
                  <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", mb: 1.5 }} data-testid="tag-filter">
                    {listTags.map((tg) => (
                      <Chip
                        key={tg}
                        size="sm"
                        variant={tagFilter === tg ? "solid" : "outlined"}
                        onClick={() => setTagFilter(tagFilter === tg ? null : tg)}
                      >
                        #{tg}
                      </Chip>
                    ))}
                  </Box>
                )}

                {view === "board" && (
                  <BoardView tasks={tasks} extras={extras} readOnly={readOnly} onMove={handleMove} onOpen={openEdit} />
                )}
                {view === "calendar" && <TaskCalendar tasks={tasks} onOpen={openEdit} />}

                {loadingTasks && (
                  <Box
                    sx={{ display: "flex", justifyContent: "center", py: 3 }}
                  >
                    <CircularProgress size="sm" />
                  </Box>
                )}

                {view === "list" && !loadingTasks && tasks.length === 0 && (
                  <Sheet
                    variant="soft"
                    sx={{ p: 3, borderRadius: "md", textAlign: "center" }}
                  >
                    <Typography level="body-md" sx={{ opacity: 0.6 }}>
                      No tasks yet.
                    </Typography>
                  </Sheet>
                )}

                {/* Active tasks */}
                {view === "list" && !loadingTasks && activeTasks.length > 0 && (
                  <List size="sm" sx={{ "--List-gap": "2px", mb: 1 }}>
                    {activeTasks.map((task) => (
                      <TaskRow
                        key={task.id}
                        task={task}
                        subtasks={subtasksFor(task.id)}
                        extras={extras[task.id]}
                        readOnly={readOnly}
                        onReschedule={(w) => handleReschedule(task, w)}
                        onToggle={() => handleToggle(task)}
                        onEdit={() => openEdit(task)}
                        onDelete={() => handleDeleteTask(task)}
                        onToggleSubtask={(st) => handleToggle(st)}
                        onDeleteSubtask={(st) => handleDeleteTask(st)}
                        onEditSubtask={(st) => openEdit(st)}
                      />
                    ))}
                  </List>
                )}

                {/* Completed section */}
                {view === "list" && !loadingTasks && completedTasks.length > 0 && (
                  <Box>
                    <Divider sx={{ my: 1 }} />
                    <Button
                      variant="plain"
                      color="neutral"
                      size="sm"
                      startDecorator={
                        completedExpanded ? (
                          <ExpandLessIcon />
                        ) : (
                          <ExpandMoreIcon />
                        )
                      }
                      onClick={() => setCompletedExpanded((v) => !v)}
                      sx={{ mb: 0.5 }}
                    >
                      Completed ({completedTasks.length})
                    </Button>
                    {completedExpanded && (
                      <List
                        size="sm"
                        sx={{ "--List-gap": "2px", opacity: 0.6 }}
                      >
                        {completedTasks.map((task) => (
                          <TaskRow
                            key={task.id}
                            task={task}
                            subtasks={[]}
                            onToggle={() => handleToggle(task)}
                            onEdit={() => openEdit(task)}
                            onDelete={() => handleDeleteTask(task)}
                            onToggleSubtask={() => {}}
                            onDeleteSubtask={() => {}}
                            onEditSubtask={() => {}}
                          />
                        ))}
                      </List>
                    )}
                  </Box>
                )}
              </>
            )}
          </Box>
        </Box>
      </Container>

      {/* Rename list modal */}
      {renaming && (
        <Modal open onClose={() => setRenaming(null)}>
          <ModalDialog sx={{ width: 360 }}>
            <Typography level="title-md" sx={{ mb: 1.5 }}>
              Rename list
            </Typography>
            <Input
              autoFocus
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") handleRenameList();
              }}
              sx={{ mb: 2 }}
            />
            <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1 }}>
              <Button
                variant="plain"
                color="neutral"
                onClick={() => setRenaming(null)}
              >
                Cancel
              </Button>
              <Button onClick={handleRenameList}>Rename</Button>
            </Box>
          </ModalDialog>
        </Modal>
      )}

      {/* Edit task */}
      {editingTask && (
        <TaskEditor
          key={editingTask.id}
          task={editingTask}
          extras={extras[editingTask.id]}
          subtasks={subtasksFor(editingTask.id)}
          readOnly={readOnly}
          onSave={handleSaveEditor}
          onAddSubtask={handleAddSubtask}
          onClose={() => setEditingTask(null)}
        />
      )}
      {shareOpen && selectedList && <ShareDialog listId={selectedList.id} listName={selectedList.name} onClose={() => setShareOpen(false)} />}
    </>
  );
}

interface TaskRowProps {
  task: Task;
  subtasks: Task[];
  extras?: TaskExtras;
  readOnly?: boolean;
  onReschedule?: (w: QuickWhen) => void;
  onToggle: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onToggleSubtask: (t: Task) => void;
  onDeleteSubtask: (t: Task) => void;
  onEditSubtask: (t: Task) => void;
}

function TaskRow({
  task,
  subtasks,
  extras,
  readOnly,
  onReschedule,
  onToggle,
  onEdit,
  onDelete,
  onToggleSubtask,
  onDeleteSubtask,
  onEditSubtask,
}: TaskRowProps) {
  const hasDue = task.due_at && !task.completed;
  const dueLabel = hasDue ? formatDue(task.due_at) : null;
  const isOverdue = hasDue && new Date(task.due_at) < new Date();

  return (
    <>
      <ListItem
        sx={{ borderRadius: "md" }}
        endAction={
          <Box sx={{ display: "flex", gap: 0.25 }}>
            {!readOnly && !task.completed && onReschedule && (
              <Dropdown>
                <MenuButton slots={{ root: IconButton }} slotProps={{ root: { size: "sm", variant: "plain", "aria-label": "Reschedule", "data-testid": "reschedule" } as never }}>
                  <EventIcon sx={{ fontSize: 16 }} />
                </MenuButton>
                <Menu size="sm" placement="bottom-end">
                  <MenuItem onClick={() => onReschedule("today")}>Today</MenuItem>
                  <MenuItem onClick={() => onReschedule("tomorrow")}>Tomorrow</MenuItem>
                  <MenuItem onClick={() => onReschedule("nextmonday")}>Next Monday</MenuItem>
                  <MenuItem onClick={() => onReschedule("nextweek")}>In a week</MenuItem>
                </Menu>
              </Dropdown>
            )}
            <Tooltip title="Edit" size="sm">
              <IconButton
                size="sm"
                variant="plain"
                onClick={onEdit}
                aria-label="Edit task"
              >
                <EditOutlinedIcon sx={{ fontSize: 16 }} />
              </IconButton>
            </Tooltip>
            <Tooltip title="Delete" size="sm">
              <IconButton
                size="sm"
                variant="plain"
                color="danger"
                onClick={onDelete}
                aria-label="Delete task"
              >
                <DeleteOutlineIcon sx={{ fontSize: 16 }} />
              </IconButton>
            </Tooltip>
          </Box>
        }
      >
        <Checkbox
          size="sm"
          checked={task.completed}
          onChange={onToggle}
          label={
            <Box>
              <Typography
                level="body-sm"
                sx={{
                  textDecoration: task.completed ? "line-through" : "none",
                  opacity: task.completed ? 0.5 : 1,
                }}
              >
                {task.title}
              </Typography>
              {dueLabel && (
                <Typography
                  level="body-xs"
                  color={isOverdue ? "danger" : "neutral"}
                  sx={{ opacity: 0.8 }}
                >
                  {dueLabel}
                </Typography>
              )}
              {(extras?.priority || extras?.recurrence || extras?.assignee_name || (extras?.tags?.length ?? 0) > 0 || subtasks.length > 0) && (
                <Box sx={{ display: "flex", gap: 0.75, flexWrap: "wrap", alignItems: "center" }}>
                  {!!extras?.priority && (
                    <Chip size="sm" variant="soft" color={extras.priority === 3 ? "danger" : extras.priority === 2 ? "warning" : "neutral"}>
                      {PRIORITIES[extras.priority]}
                    </Chip>
                  )}
                  {extras?.recurrence && (
                    <Typography level="body-xs" startDecorator={<RepeatIcon sx={{ fontSize: 12 }} />}>
                      {recurrenceLabel(extras.recurrence)}
                    </Typography>
                  )}
                  {extras?.assignee_name && <Typography level="body-xs">{extras.assignee_name}</Typography>}
                  {(extras?.tags ?? []).map((tg) => (
                    <Chip key={tg} size="sm" variant="outlined">#{tg}</Chip>
                  ))}
                  {subtasks.length > 0 && <Typography level="body-xs">{subtaskProgress(subtasks)} subtasks</Typography>}
                </Box>
              )}
              {task.notes && (
                <Typography
                  level="body-xs"
                  sx={{ opacity: 0.6, whiteSpace: "pre-wrap" }}
                >
                  {task.notes.slice(0, 80)}
                  {task.notes.length > 80 ? "…" : ""}
                </Typography>
              )}
            </Box>
          }
        />
      </ListItem>
      {subtasks.map((st) => (
        <ListItem
          key={st.id}
          sx={{ pl: 4, borderRadius: "md" }}
          endAction={
            <Box sx={{ display: "flex", gap: 0.25 }}>
              <IconButton
                size="sm"
                variant="plain"
                onClick={() => onEditSubtask(st)}
                aria-label="Edit subtask"
              >
                <EditOutlinedIcon sx={{ fontSize: 14 }} />
              </IconButton>
              <IconButton
                size="sm"
                variant="plain"
                color="danger"
                onClick={() => onDeleteSubtask(st)}
                aria-label="Delete subtask"
              >
                <DeleteOutlineIcon sx={{ fontSize: 14 }} />
              </IconButton>
            </Box>
          }
        >
          <Checkbox
            size="sm"
            checked={st.completed}
            onChange={() => onToggleSubtask(st)}
            label={
              <Typography
                level="body-xs"
                sx={{
                  textDecoration: st.completed ? "line-through" : "none",
                  opacity: st.completed ? 0.5 : 1,
                }}
              >
                {st.title}
              </Typography>
            }
          />
        </ListItem>
      ))}
    </>
  );
}

function formatDue(isoStr: string): string {
  try {
    const d = new Date(isoStr);
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    const diff = Math.round((d.getTime() - today.getTime()) / 86400000);
    if (diff === 0) return "Today";
    if (diff === 1) return "Tomorrow";
    if (diff === -1) return "Yesterday";
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  } catch {
    return isoStr;
  }
}
