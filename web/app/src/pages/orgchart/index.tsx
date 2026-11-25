import { useEffect, useMemo, useState } from "react";
import { Alert, Avatar, Box, Button, CircularProgress, Container, Input, Sheet, Tab, TabList, Tabs, Typography } from "@mui/joy";
import { Header } from "../../components/Header";
import { PersonCard } from "../../components/PersonCard";
import type { User } from "../../api/types";
import { peopleApi, type Department, type Person } from "../../api/people";

export interface HrNode {
  id: string;
  user_id: string;
  name: string;
  email: string;
  designation: string;
  department: string;
  department_id: string;
  manager_id: string;
}

/** hrPeople maps HR employees onto the directory Person shape. People with an account keep their user id so avatars and PersonCards work. */
export function hrPeople(nodes: HrNode[], dir: Person[]): Person[] {
  const key = (n: HrNode) => n.user_id || n.id;
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const phones = new Map(dir.map((p) => [p.id, p]));
  return nodes.map((n) => {
    const mgr = n.manager_id ? byId.get(n.manager_id) : undefined;
    const d = phones.get(n.user_id);
    return {
      id: key(n),
      name: n.name,
      email: n.email,
      job_title: n.designation,
      phone: d?.phone ?? "",
      location: d?.location ?? "",
      employee_id: d?.employee_id ?? "",
      start_date: d?.start_date ?? "",
      department_id: n.department_id,
      department_name: n.department,
      manager_id: mgr ? key(mgr) : "",
      manager_name: mgr?.name ?? "",
    };
  });
}

export interface OrgNode {
  person: Person;
  reports: OrgNode[];
}

/** Builds the manager forest. Anyone whose manager is unknown (or who sits in a loop) becomes a root. */
export function buildOrgTree(people: Person[]): OrgNode[] {
  const byId = new Map(people.map((p) => [p.id, p]));
  const kids = new Map<string, Person[]>();
  const roots: Person[] = [];
  for (const p of people) {
    if (p.manager_id && byId.has(p.manager_id) && p.manager_id !== p.id) {
      kids.set(p.manager_id, [...(kids.get(p.manager_id) ?? []), p]);
    } else {
      roots.push(p);
    }
  }
  const seen = new Set<string>();
  const make = (p: Person): OrgNode => {
    seen.add(p.id);
    return {
      person: p,
      reports: (kids.get(p.id) ?? []).filter((c) => !seen.has(c.id)).map(make),
    };
  };
  const out = roots.map(make);
  // Members of a manager loop are unreachable from any root: surface them too.
  for (const p of people) if (!seen.has(p.id)) out.push(make(p));
  return out;
}

function PersonLine({ p }: { p: Person }) {
  return (
    <Sheet variant="outlined" sx={{ display: "inline-flex", alignItems: "center", gap: 1, p: 1, pr: 1.5, borderRadius: "md" }} data-testid="org-node">
      <PersonCard userId={p.id} name={p.name}>
        <Avatar size="sm" src={`/api/v1/users/${p.id}/avatar`}>
          {p.name.charAt(0).toUpperCase()}
        </Avatar>
      </PersonCard>
      <Box sx={{ minWidth: 0 }}>
        <PersonCard userId={p.id} name={p.name}>
          <Typography level="title-sm">{p.name}</Typography>
        </PersonCard>
        <Typography level="body-xs" sx={{ opacity: 0.75 }}>
          {[p.job_title, p.department_name].filter(Boolean).join(" · ") || p.email}
        </Typography>
      </Box>
    </Sheet>
  );
}

function Node({ n, depth }: { n: OrgNode; depth: number }) {
  const [open, setOpen] = useState(depth < 3);
  return (
    <Box sx={{ ml: depth ? 3 : 0, borderLeft: depth ? "1px solid" : "none", borderColor: "divider", pl: depth ? 1.5 : 0, mt: 1 }}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        {n.reports.length > 0 && (
          <Button size="sm" variant="plain" color="neutral" onClick={() => setOpen((o) => !o)} aria-label={open ? "Collapse" : "Expand"} sx={{ minWidth: 0, px: 1 }}>
            {open ? "-" : `+${n.reports.length}`}
          </Button>
        )}
        <PersonLine p={n.person} />
      </Box>
      {open && n.reports.map((c) => <Node key={c.person.id} n={c} depth={depth + 1} />)}
    </Box>
  );
}

function DeptTree({ deps, people }: { deps: Department[]; people: Person[] }) {
  const kids = (parent: string) => deps.filter((d) => (d.parent_id || "") === parent);
  const render = (d: Department, depth: number): React.ReactNode => (
    <Box key={d.id} sx={{ ml: depth ? 3 : 0, mt: 1.5 }} data-testid="org-dept">
      <Typography level="title-md">
        {d.name} <Typography level="body-xs">({d.member_count})</Typography>
      </Typography>
      <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1, mt: 0.5 }}>
        {people
          .filter((p) => p.department_id === d.id)
          .map((p) => (
            <PersonLine key={p.id} p={p} />
          ))}
      </Box>
      {kids(d.id).map((c) => render(c, depth + 1))}
    </Box>
  );
  const ids = new Set(deps.map((d) => d.id));
  const roots = deps.filter((d) => !d.parent_id || !ids.has(d.parent_id));
  return <>{roots.map((d) => render(d, 0))}</>;
}

function MyDetails({ user }: { user: User }) {
  const [phone, setPhone] = useState("");
  const [location, setLocation] = useState("");
  const [msg, setMsg] = useState<string | null>(null);
  useEffect(() => {
    peopleApi
      .profile(user.id)
      .then((r) => {
        setPhone(r.person.phone);
        setLocation(r.person.location);
      })
      .catch(() => undefined);
  }, [user.id]);
  async function save() {
    setMsg(null);
    try {
      await peopleApi.update(user.id, { phone, location });
      setMsg("Saved");
    } catch (e) {
      setMsg((e as Error).message);
    }
  }
  return (
    <Sheet variant="soft" sx={{ p: 1.5, borderRadius: "md", mb: 2, display: "flex", gap: 1, alignItems: "center", flexWrap: "wrap" }} data-testid="my-details">
      <Typography level="title-sm">Your details</Typography>
      <Input size="sm" placeholder="Phone" value={phone} onChange={(e) => setPhone(e.target.value)} slotProps={{ input: { "aria-label": "My phone" } }} />
      <Input size="sm" placeholder="Location" value={location} onChange={(e) => setLocation(e.target.value)} slotProps={{ input: { "aria-label": "My location" } }} />
      <Button size="sm" onClick={() => void save()}>
        Save
      </Button>
      {msg && <Typography level="body-xs">{msg}</Typography>}
    </Sheet>
  );
}

export default function OrgChartPage({ user }: { user: User }) {
  const [data, setData] = useState<{ people: Person[]; departments: Department[] } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState<"manager" | "department">("manager");
  useEffect(() => {
    // Prefer HR employee records (they carry the reporting lines HR maintains);
    // fall back to the directory when HR has no employees yet.
    (async () => {
      const dir = await peopleApi.orgChart();
      try {
        const r = await fetch("/api/v1/hr/orgchart", { credentials: "same-origin", headers: { Accept: "application/json" } });
        if (r.ok) {
          const hrNodes = ((await r.json()) as { people: HrNode[] }).people ?? [];
          if (hrNodes.length > 0) {
            setData({ people: hrPeople(hrNodes, dir.people), departments: dir.departments });
            return;
          }
        }
      } catch {
        /* fall through to the directory */
      }
      setData(dir);
    })().catch((e: Error) => setError(e.message));
  }, []);
  const tree = useMemo(() => (data ? buildOrgTree(data.people) : []), [data]);
  return (
    <>
      <Header user={user} />
      <Container maxWidth="lg" sx={{ py: 3 }}>
        <Typography level="h2" sx={{ mb: 1 }}>
          Org chart
        </Typography>
        <MyDetails user={user} />
        <Tabs value={tab} onChange={(_, v) => setTab(v as "manager" | "department")} sx={{ bgcolor: "transparent" }}>
          <TabList>
            <Tab value="manager">By manager</Tab>
            <Tab value="department">By department</Tab>
          </TabList>
        </Tabs>
        {error && <Alert color="danger">{error}</Alert>}
        {!data && !error && <CircularProgress />}
        {data && tab === "manager" && (
          <Box data-testid="org-tree">
            {tree.map((n) => (
              <Node key={n.person.id} n={n} depth={0} />
            ))}
          </Box>
        )}
        {data && tab === "department" && <DeptTree deps={data.departments} people={data.people} />}
      </Container>
    </>
  );
}
