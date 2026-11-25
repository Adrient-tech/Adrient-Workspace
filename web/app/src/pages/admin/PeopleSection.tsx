// ---------------------------------------------------------------------------
// People - admin view of the org directory: edit each person's title,
// department, manager, phone, location, employee id and start date, and manage
// the department tree. Backed by /api/v1/people and /api/v1/departments
// (internal/people). Admin/Owner can edit; the server enforces it.
// ---------------------------------------------------------------------------
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Alert,
  Avatar,
  Box,
  Button,
  FormControl,
  FormLabel,
  IconButton,
  Input,
  Modal,
  ModalClose,
  ModalDialog,
  Option,
  Select,
  Sheet,
  Typography,
} from "@mui/joy";
import EditOutlinedIcon from "@mui/icons-material/EditOutlined";
import DeleteOutlineIcon from "@mui/icons-material/DeleteOutline";
import AccountTreeIcon from "@mui/icons-material/AccountTree";
import {
  peopleApi,
  type Department,
  type Person,
  type PersonPatch,
} from "../../api/people";
import { PersonCard } from "../../components/PersonCard";
import { DataTable, type Column, type TableQuery, type TablePage } from "../../components/DataTable";
import { PeoplePicker, usePeopleByIds, type DirectoryItem } from "../../components/PeoplePicker";

const NONE = "__none__";

/** Departments in tree order with their depth, for indented pickers/lists. */
export function flattenDepartments(
  deps: Department[],
): { dept: Department; depth: number }[] {
  const byParent = new Map<string, Department[]>();
  const ids = new Set(deps.map((d) => d.id));
  for (const d of deps) {
    const k = d.parent_id && ids.has(d.parent_id) ? d.parent_id : "";
    byParent.set(k, [...(byParent.get(k) ?? []), d]);
  }
  const out: { dept: Department; depth: number }[] = [];
  const seen = new Set<string>();
  const walk = (parent: string, depth: number) => {
    for (const d of byParent.get(parent) ?? []) {
      if (seen.has(d.id)) continue;
      seen.add(d.id);
      out.push({ dept: d, depth });
      walk(d.id, depth + 1);
    }
  };
  walk("", 0);
  return out;
}


function peopleParams(q: TableQuery): string {
  const p = new URLSearchParams({ limit: String(q.limit), page: String(q.page), sort: q.sort || "name", order: q.order });
  if (q.q) p.set("q", q.q);
  if (q.filters.department) p.set("department", q.filters.department);
  return p.toString();
}

async function loadPeople(q: TableQuery, signal: AbortSignal): Promise<TablePage<Person>> {
  const r = await fetch(`/api/v1/people/page?${peopleParams(q)}`, { credentials: "same-origin", signal });
  if (!r.ok) throw new Error(`Could not load people (HTTP ${r.status})`);
  return (await r.json()) as TablePage<Person>;
}

export function PeopleSection() {
  const navigate = useNavigate();
  const [tick, setTick] = useState(0);
  const [depts, setDepts] = useState<Department[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState<Person | null>(null);
  const [deptDlg, setDeptDlg] = useState<Department | "new" | null>(null);

  const reload = useCallback(async () => {
    try {
      setDepts(await peopleApi.departments());
      setTick((n) => n + 1);
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    }
  }, []);
  useEffect(() => {
    void reload();
  }, [reload]);

  const columns: Column<Person>[] = useMemo(
    () => [
      {
        key: "name",
        header: "Person",
        sortable: true,
        width: "34%",
        render: (p) => (
          <Box sx={{ display: "flex", alignItems: "center", gap: 1 }} data-testid="people-row">
            <PersonCard userId={p.id} name={p.name}>
              <Avatar size="sm" src={`/api/v1/users/${p.id}/avatar`}>
                {p.name.charAt(0).toUpperCase()}
              </Avatar>
            </PersonCard>
            <Box sx={{ minWidth: 0 }}>
              <Typography level="body-sm" noWrap sx={{ fontWeight: 600 }}>
                {p.name}
              </Typography>
              <Typography level="body-xs" noWrap sx={{ opacity: 0.7 }}>
                {p.email}
              </Typography>
            </Box>
          </Box>
        ),
      },
      { key: "title", header: "Title", sortable: true, render: (p) => p.job_title || "-" },
      { key: "department", header: "Department", sortable: true, render: (p) => p.department_name || "-" },
      { key: "employee_id", header: "Employee ID", sortable: true, width: 120, render: (p) => p.employee_id || "-" },
      { key: "manager", header: "Manager", render: (p) => p.manager_name || "-" },
      {
        key: "edit",
        header: "",
        width: 56,
        render: (p) => (
          <IconButton size="sm" variant="plain" aria-label={`Edit ${p.name}`} onClick={() => setEditing(p)}>
            <EditOutlinedIcon />
          </IconButton>
        ),
      },
    ],
    [],
  );
  // Departments: filter on the client (the tree is small next to people) and
  // render the first 25 until asked for all.
  const [deptQ, setDeptQ] = useState("");
  const [deptAll, setDeptAll] = useState(false);
  const flat = useMemo(() => flattenDepartments(depts), [depts]);
  const deptMatches = useMemo(() => {
    const n = deptQ.trim().toLowerCase();
    return n ? flat.filter(({ dept }) => dept.name.toLowerCase().includes(n)) : flat;
  }, [flat, deptQ]);
  const deptShown = deptAll || deptQ ? deptMatches : deptMatches.slice(0, 25);

  async function removeDept(d: Department) {
    if (!window.confirm(`Delete department "${d.name}"? Its people keep their account but lose this department.`)) return;
    try {
      await peopleApi.deleteDepartment(d.id);
      await reload();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }} data-testid="people-section">
      <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
        <Typography level="h3" sx={{ flex: 1 }}>
          People
        </Typography>
        <Button
          variant="outlined"
          startDecorator={<AccountTreeIcon />}
          onClick={() => navigate("/org-chart")}
          data-testid="open-org-chart"
        >
          Org chart
        </Button>
      </Box>
      {error && (
        <Alert color="danger" variant="soft">
          {error}
        </Alert>
      )}

      <Sheet variant="outlined" sx={{ borderRadius: "md", p: 2 }}>
        <Box sx={{ display: "flex", alignItems: "center", mb: 1 }}>
          <Typography level="title-md" sx={{ flex: 1 }}>
            Departments
          </Typography>
          <Input size="sm" placeholder="Filter departments" value={deptQ} onChange={(e) => setDeptQ(e.target.value)} sx={{ mr: 1, width: 200 }} slotProps={{ input: { "aria-label": "Filter departments" } }} />
          <Button size="sm" onClick={() => setDeptDlg("new")} data-testid="add-department">
            Add department
          </Button>
        </Box>
        {flat.length === 0 ? (
          <Typography level="body-sm" sx={{ opacity: 0.7 }}>
            No departments yet.
          </Typography>
        ) : (
          deptShown.map(({ dept, depth }) => (
            <Box
              key={dept.id}
              data-testid="department-row"
              sx={{ display: "flex", alignItems: "center", gap: 1, py: 0.5, pl: depth * 3 }}
            >
              <Typography level="body-sm" sx={{ fontWeight: 600 }}>
                {dept.name}
              </Typography>
              <Typography level="body-xs" sx={{ opacity: 0.7, flex: 1 }}>
                {dept.member_count} {dept.member_count === 1 ? "person" : "people"}
                {dept.head_name ? ` · head: ${dept.head_name}` : ""}
              </Typography>
              <IconButton size="sm" variant="plain" aria-label={`Edit ${dept.name}`} onClick={() => setDeptDlg(dept)}>
                <EditOutlinedIcon />
              </IconButton>
              <IconButton size="sm" variant="plain" color="danger" aria-label={`Delete ${dept.name}`} onClick={() => void removeDept(dept)}>
                <DeleteOutlineIcon />
              </IconButton>
            </Box>
          ))
        )}
        {deptMatches.length > deptShown.length && (
          <Button size="sm" variant="plain" onClick={() => setDeptAll(true)} data-testid="departments-all">
            Show all {deptMatches.length} departments
          </Button>
        )}
      </Sheet>

      <DataTable<Person>
        testId="people-table"
        columns={columns}
        rowKey={(p) => p.id}
        load={loadPeople}
        mode="page"
        defaultSort="name"
        defaultOrder="asc"
        searchPlaceholder="Search name, email, title, employee ID, department"
        filters={[
          {
            key: "department",
            label: "Department",
            type: "select",
            options: flat.map(({ dept, depth }) => ({ value: dept.id, label: "\u2003".repeat(depth) + dept.name })),
          },
        ]}
        exportUrl={(q) => `/api/v1/people/page?format=csv&${peopleParams(q)}`}
        reloadKey={tick}
        emptyText="No people yet"
      />

      {editing && (
        <EditPersonDialog
          person={editing}
          departments={flat}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            void reload();
          }}
        />
      )}
      {deptDlg && (
        <DepartmentDialog
          dept={deptDlg === "new" ? null : deptDlg}
          departments={flat}
          onClose={() => setDeptDlg(null)}
          onSaved={() => {
            setDeptDlg(null);
            void reload();
          }}
        />
      )}
    </Box>
  );
}

function EditPersonDialog({
  person,
  departments,
  onClose,
  onSaved,
}: {
  person: Person;
  departments: { dept: Department; depth: number }[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [f, setF] = useState({
    job_title: person.job_title,
    phone: person.phone,
    location: person.location,
    employee_id: person.employee_id,
    start_date: person.start_date,
    department_id: person.department_id,
    manager_id: person.manager_id,
  });
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const known = usePeopleByIds(person.manager_id ? [person.manager_id] : []);
  const [picked, setPicked] = useState<DirectoryItem[] | null>(null);
  const manager = picked ?? known;
  const setManager = (n: DirectoryItem[]) => setPicked(n);
  const set = (k: keyof typeof f, v: string) => setF((c) => ({ ...c, [k]: v }));

  async function save() {
    setBusy(true);
    setErr(null);
    try {
      const patch: PersonPatch = { ...f };
      await peopleApi.update(person.id, patch);
      onSaved();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 480, maxWidth: "95vw", overflow: "auto" }} data-testid="edit-person-dialog">
        <ModalClose />
        <Typography level="title-lg">{person.name}</Typography>
        <Typography level="body-sm" sx={{ opacity: 0.7, mb: 1 }}>
          {person.email}
        </Typography>
        {err && (
          <Alert color="danger" variant="soft">
            {err}
          </Alert>
        )}
        <Box sx={{ display: "flex", flexDirection: "column", gap: 1.25 }}>
          <FormControl>
            <FormLabel>Job title</FormLabel>
            <Input value={f.job_title} onChange={(e) => set("job_title", e.target.value)} slotProps={{ input: { "aria-label": "Job title" } }} />
          </FormControl>
          <FormControl>
            <FormLabel>Department</FormLabel>
            <Select
              value={f.department_id || NONE}
              onChange={(_, v) => set("department_id", !v || v === NONE ? "" : v)}
              slotProps={{ button: { "aria-label": "Department" } }}
            >
              <Option value={NONE}>None</Option>
              {departments.map(({ dept, depth }) => (
                <Option key={dept.id} value={dept.id}>
                  {"\u2003".repeat(depth)}
                  {dept.name}
                </Option>
              ))}
            </Select>
          </FormControl>
          <FormControl>
            <FormLabel>Manager</FormLabel>
            <PeoplePicker
              ariaLabel="Manager"
              placeholder="Search for a manager"
              value={manager}
              onChange={(n) => {
                setManager(n);
                set("manager_id", n[0]?.id ?? "");
              }}
              hideMe
              testId="manager-picker"
            />
          </FormControl>
          <Box sx={{ display: "flex", gap: 1 }}>
            <FormControl sx={{ flex: 1 }}>
              <FormLabel>Phone</FormLabel>
              <Input value={f.phone} onChange={(e) => set("phone", e.target.value)} />
            </FormControl>
            <FormControl sx={{ flex: 1 }}>
              <FormLabel>Location</FormLabel>
              <Input value={f.location} onChange={(e) => set("location", e.target.value)} />
            </FormControl>
          </Box>
          <Box sx={{ display: "flex", gap: 1 }}>
            <FormControl sx={{ flex: 1 }}>
              <FormLabel>Employee ID</FormLabel>
              <Input value={f.employee_id} onChange={(e) => set("employee_id", e.target.value)} />
            </FormControl>
            <FormControl sx={{ flex: 1 }}>
              <FormLabel>Start date</FormLabel>
              <Input type="date" value={f.start_date} onChange={(e) => set("start_date", e.target.value)} />
            </FormControl>
          </Box>
        </Box>
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 2 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={busy} onClick={() => void save()} data-testid="save-person">
            Save
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}

function DepartmentDialog({
  dept,
  departments,
  onClose,
  onSaved,
}: {
  dept: Department | null;
  departments: { dept: Department; depth: number }[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(dept?.name ?? "");
  const [parent, setParent] = useState(dept?.parent_id ?? "");
  const [head, setHead] = useState(dept?.head_user_id ?? "");
  const knownHead = usePeopleByIds(dept?.head_user_id ? [dept.head_user_id] : []);
  const [headPicked, setHeadPicked] = useState<DirectoryItem[] | null>(null);
  const headItems = headPicked ?? knownHead;
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  async function save() {
    setBusy(true);
    setErr(null);
    try {
      if (dept) {
        await peopleApi.updateDepartment(dept.id, { name, parent_id: parent, head_user_id: head });
      } else {
        await peopleApi.createDepartment({ name, parent_id: parent, head_user_id: head });
      }
      onSaved();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ width: 440, maxWidth: "95vw" }} data-testid="department-dialog">
        <ModalClose />
        <Typography level="title-lg">{dept ? "Edit department" : "New department"}</Typography>
        {err && (
          <Alert color="danger" variant="soft">
            {err}
          </Alert>
        )}
        <FormControl>
          <FormLabel>Name</FormLabel>
          <Input autoFocus value={name} onChange={(e) => setName(e.target.value)} slotProps={{ input: { "aria-label": "Department name" } }} />
        </FormControl>
        <FormControl>
          <FormLabel>Parent department</FormLabel>
          <Select value={parent || NONE} onChange={(_, v) => setParent(!v || v === NONE ? "" : v)}>
            <Option value={NONE}>None (top level)</Option>
            {departments
              .filter(({ dept: d }) => d.id !== dept?.id)
              .map(({ dept: d, depth }) => (
                <Option key={d.id} value={d.id}>
                  {"\u2003".repeat(depth)}
                  {d.name}
                </Option>
              ))}
          </Select>
        </FormControl>
        <FormControl>
          <FormLabel>Head</FormLabel>
          <PeoplePicker
            ariaLabel="Department head"
            placeholder="Search for the head"
            value={headItems}
            onChange={(n) => {
              setHeadPicked(n);
              setHead(n[0]?.id ?? "");
            }}
            hideMe
            testId="head-picker"
          />
        </FormControl>
        <Box sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1 }}>
          <Button variant="plain" color="neutral" onClick={onClose}>
            Cancel
          </Button>
          <Button loading={busy} disabled={!name.trim()} onClick={() => void save()} data-testid="save-department">
            Save
          </Button>
        </Box>
      </ModalDialog>
    </Modal>
  );
}
