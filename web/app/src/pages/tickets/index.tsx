import { useEffect, useMemo, useState } from "react";
import {
  Alert, Box, Button, DialogContent, DialogTitle, FormControl, FormLabel, Input, Modal, ModalDialog, Radio, RadioGroup, Textarea, Typography, CircularProgress,
} from "@mui/joy";
import { Header } from "../../components/Header";
import { EmptyState } from "../../components/ui/EmptyState";
import { onAiChanged } from "../../lib/ai/biz";
import { BizAssistant } from "../../components/BizAssistant";
import { createTicketTools, TICKETS_HINT, TICKETS_SUGGESTIONS } from "./ai/tools";
import { CustomersDialog } from "./Helpdesk";
import { Workspace } from "./Workspace";
import { DeskSettings } from "./DeskSettings";
import { DeskReports } from "./DeskReports";
import { DeskKb } from "./DeskKb";
import type { User } from "../../api/types";
import { listProjects, createProject, type Project, type IntakeMode } from "./api";

type Tool = "" | "settings" | "reports" | "kb" | "customers";

export default function TicketsApp({ user }: { user: User }) {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null);
  const [newProjectOpen, setNewProjectOpen] = useState(false);
  const [tool, setTool] = useState<Tool>("");
  const [deskKey, setDeskKey] = useState(0);

  const selectedProject = useMemo(() => projects.find((p) => p.id === selectedProjectId) ?? null, [projects, selectedProjectId]);
  const aiTools = useMemo(() => createTicketTools(() => selectedProjectId ?? ""), [selectedProjectId]);

  function reloadProjects() {
    return listProjects().then((l) => {
      setProjects(l);
      setSelectedProjectId((cur) => cur ?? l[0]?.id ?? null);
      return l;
    }).catch((e) => setError((e as Error).message));
  }
  useEffect(() => { reloadProjects().finally(() => setLoading(false)); }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const refresh = () => setDeskKey((k) => k + 1);
  useEffect(() => onAiChanged("tickets", refresh));

  return (
    <Box sx={{ minHeight: "100vh", bgcolor: "background.body" }}>
      <Header user={user} />
      {error && <Alert color="danger" sx={{ m: 2 }}>{error}</Alert>}
      {loading ? (
        <Box sx={{ display: "flex", justifyContent: "center", py: 10 }}><CircularProgress /></Box>
      ) : !selectedProject ? (
        <Box sx={{ maxWidth: 560, mx: "auto", py: 8 }}>
          <EmptyState illustration="inbox" title="Create your first help desk" hint="A project is a queue of tickets with its own statuses, teams and SLA. Add one to start." action={<Button onClick={() => setNewProjectOpen(true)} data-testid="first-project">New project</Button>} />
        </Box>
      ) : tool === "settings" ? (
        <DeskSettings pid={selectedProject.id} statuses={selectedProject.statuses} onClose={() => setTool("")} onChanged={() => { refresh(); void reloadProjects(); }} />
      ) : tool === "reports" ? (
        <DeskReports pid={selectedProject.id} onClose={() => setTool("")} />
      ) : tool === "kb" ? (
        <DeskKb pid={selectedProject.id} publicToken={selectedProject.public_token} onClose={() => setTool("")} />
      ) : (
        <>
          <Workspace user={user} project={selectedProject} projects={projects} onSelectProject={setSelectedProjectId} onNewProject={() => setNewProjectOpen(true)}
            onTool={(t) => setTool(t)} refreshKey={deskKey} assistant={<BizAssistant app="tickets" documentId={selectedProject.id} title={selectedProject.name} tools={aiTools} suggestions={TICKETS_SUGGESTIONS} systemHint={TICKETS_HINT} />} onChanged={() => { void reloadProjects(); }} />
        </>
      )}
      {tool === "customers" && selectedProject && <CustomersDialog projectId={selectedProject.id} onClose={() => setTool("")} />}
      {newProjectOpen && (
        <NewProjectDialog
          onClose={() => setNewProjectOpen(false)}
          onCreated={(p) => { setNewProjectOpen(false); void reloadProjects().then(() => setSelectedProjectId(p.id)); }}
        />
      )}
    </Box>
  );
}

// ----------------------------------------------------------------------------

function NewProjectDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (p: Project) => void;
}) {
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [description, setDescription] = useState("");
  const [intakeMode, setIntakeMode] = useState<IntakeMode>("team");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    setBusy(true);
    setError(null);
    try {
      const p = await createProject({
        name: name.trim(),
        key: key.trim().toUpperCase(),
        description: description.trim(),
        intake_mode: intakeMode,
      });
      onCreated(p);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const canSubmit = name.trim() !== "" && key.trim() !== "" && !busy;

  return (
    <Modal open onClose={onClose}>
      <ModalDialog sx={{ maxWidth: 460, width: "100%" }}>
        <DialogTitle>New project</DialogTitle>
        <DialogContent>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 1.5, mt: 1 }}>
            <FormControl size="sm">
              <FormLabel>Name</FormLabel>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Customer Support"
              />
            </FormControl>
            <FormControl size="sm">
              <FormLabel>Key</FormLabel>
              <Input
                value={key}
                onChange={(e) => setKey(e.target.value.toUpperCase())}
                placeholder="SUP"
                slotProps={{ input: { style: { textTransform: "uppercase" } } }}
              />
            </FormControl>
            <FormControl size="sm">
              <FormLabel>Description</FormLabel>
              <Textarea
                minRows={2}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="What is this project for?"
              />
            </FormControl>
            <FormControl size="sm">
              <FormLabel>Intake mode</FormLabel>
              <RadioGroup
                orientation="horizontal"
                value={intakeMode}
                onChange={(e) => setIntakeMode(e.target.value as IntakeMode)}
              >
                <Radio value="team" label="Team only" />
                <Radio value="public" label="Public link" />
              </RadioGroup>
            </FormControl>
            {error && (
              <Typography color="danger" level="body-sm">
                {error}
              </Typography>
            )}
            <Box
              sx={{
                display: "flex",
                justifyContent: "flex-end",
                gap: 1,
                mt: 1,
              }}
            >
              <Button variant="plain" color="neutral" onClick={onClose}>
                Cancel
              </Button>
              <Button
                onClick={submit}
                loading={busy}
                disabled={!canSubmit}
                
              >
                Create
              </Button>
            </Box>
          </Box>
        </DialogContent>
      </ModalDialog>
    </Modal>
  );
}

