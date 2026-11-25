import { useEffect, useState } from "react";
import {
  Modal,
  ModalDialog,
  ModalClose,
  Typography,
  FormControl,
  FormLabel,
  Input,
  Textarea,
  Button,
  Box,
  Stack,
  IconButton,
  Chip,
} from "@mui/joy";
import AddIcon from "@mui/icons-material/Add";
import CloseIcon from "@mui/icons-material/Close";
import type { Contact, ContactInput } from "./types";
import { emptyExtras, getExtras, getLinks, type Extras, type Links } from "./extrasApi";

function emptyInput(): ContactInput {
  return {
    display_name: "",
    first_name: "",
    last_name: "",
    company: "",
    job_title: "",
    emails: [],
    phones: [],
    labels: [],
    notes: "",
    starred: false,
  };
}

function toInput(c: Contact): ContactInput {
  return {
    display_name: c.display_name,
    first_name: c.first_name,
    last_name: c.last_name,
    company: c.company,
    job_title: c.job_title,
    emails: [...c.emails],
    phones: [...c.phones],
    labels: [...c.labels],
    notes: c.notes,
    starred: c.starred,
  };
}

interface ContactDialogProps {
  contact: Contact | null; // null = create
  onClose: () => void;
  onSave: (input: ContactInput, extras: Extras) => Promise<void>;
}

/** ContactDialog is the create/edit form, mirroring Google Contacts' field set. */
export function ContactDialog({
  contact,
  onClose,
  onSave,
}: ContactDialogProps) {
  const [f, setF] = useState<ContactInput>(
    contact ? toInput(contact) : emptyInput(),
  );
  const [saving, setSaving] = useState(false);
  const [x, setX] = useState<Extras>(emptyExtras());
  const [links, setLinks] = useState<Links | null>(null);
  useEffect(() => {
    if (!contact) return;
    getExtras(contact.id).then(setX).catch(() => undefined);
    getLinks(contact.id).then(setLinks).catch(() => undefined);
  }, [contact]);
  const setx = (k: keyof Extras, v: string) => setX((p) => ({ ...p, [k]: v }));
  const set = <K extends keyof ContactInput>(k: K, v: ContactInput[K]) =>
    setF((p) => ({ ...p, [k]: v }));

  // Multi-value editors (emails / phones) keep a trailing blank for quick entry.
  function multi(
    field: "emails" | "phones",
    label: string,
    placeholder: string,
  ) {
    const vals = f[field];
    const rows = vals.length ? vals : [""];
    return (
      <FormControl>
        <FormLabel>{label}</FormLabel>
        <Stack spacing={0.75}>
          {rows.map((v, i) => (
            <Box key={i} sx={{ display: "flex", gap: 0.5 }}>
              <Input
                value={v}
                placeholder={placeholder}
                sx={{ flex: 1 }}
                onChange={(e) => {
                  const next = [...rows];
                  next[i] = e.target.value;
                  set(
                    field,
                    next.filter(
                      (x, idx) => x !== "" || idx === next.length - 1,
                    ),
                  );
                }}
              />
              {rows.length > 1 && (
                <IconButton
                  size="sm"
                  variant="plain"
                  onClick={() =>
                    set(
                      field,
                      rows.filter((_, idx) => idx !== i),
                    )
                  }
                >
                  <CloseIcon />
                </IconButton>
              )}
            </Box>
          ))}
          <Button
            size="sm"
            variant="plain"
            startDecorator={<AddIcon />}
            sx={{ alignSelf: "flex-start" }}
            onClick={() => set(field, [...vals, ""])}
          >
            Add {label.toLowerCase()}
          </Button>
        </Stack>
      </FormControl>
    );
  }

  async function save() {
    setSaving(true);
    const clean: ContactInput = {
      ...f,
      emails: f.emails.map((e) => e.trim()).filter(Boolean),
      phones: f.phones.map((p) => p.trim()).filter(Boolean),
      labels: f.labels.map((l) => l.trim()).filter(Boolean),
      display_name: (f.display_name || `${f.first_name} ${f.last_name}`).trim(),
    };
    try {
      await onSave(clean, x);
      onClose();
    } catch (e) {
      window.alert(`Save failed: ${(e as Error).message}`);
      setSaving(false);
    }
  }

  const [labelInput, setLabelInput] = useState("");

  return (
    <Modal open onClose={onClose}>
      <ModalDialog
        sx={{
          width: { xs: "100vw", sm: 560 },
          maxWidth: "100vw",
          maxHeight: { xs: "100dvh", sm: "90vh" },
          overflowY: "auto",
          borderRadius: { xs: 0, sm: "md" },
        }}
      >
        <ModalClose />
        <Typography level="h4">
          {contact ? "Edit contact" : "Create contact"}
        </Typography>
        <Stack spacing={1.5} sx={{ mt: 1 }}>
          <Box
            sx={{
              display: "flex",
              gap: 1,
              flexDirection: { xs: "column", sm: "row" },
            }}
          >
            <FormControl sx={{ flex: 1 }}>
              <FormLabel>First name</FormLabel>
              <Input
                value={f.first_name}
                onChange={(e) => set("first_name", e.target.value)}
                autoFocus
              />
            </FormControl>
            <FormControl sx={{ flex: 1 }}>
              <FormLabel>Last name</FormLabel>
              <Input
                value={f.last_name}
                onChange={(e) => set("last_name", e.target.value)}
              />
            </FormControl>
          </Box>
          <FormControl>
            <FormLabel>Display name</FormLabel>
            <Input
              value={f.display_name}
              placeholder="Defaults to first + last"
              onChange={(e) => set("display_name", e.target.value)}
            />
          </FormControl>
          <Box
            sx={{
              display: "flex",
              gap: 1,
              flexDirection: { xs: "column", sm: "row" },
            }}
          >
            <FormControl sx={{ flex: 1 }}>
              <FormLabel>Company</FormLabel>
              <Input
                value={f.company}
                onChange={(e) => set("company", e.target.value)}
              />
            </FormControl>
            <FormControl sx={{ flex: 1 }}>
              <FormLabel>Job title</FormLabel>
              <Input
                value={f.job_title}
                onChange={(e) => set("job_title", e.target.value)}
              />
            </FormControl>
          </Box>
          {multi("emails", "Email", "name@example.com")}
          {multi("phones", "Phone", "+1 555 123 4567")}
          <FormControl>
            <FormLabel>Labels</FormLabel>
            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mb: 0.5 }}>
              {f.labels.map((l) => (
                <Chip
                  key={l}
                  variant="soft"
                  endDecorator={<CloseIcon sx={{ fontSize: 14 }} />}
                  onClick={() =>
                    set(
                      "labels",
                      f.labels.filter((x) => x !== l),
                    )
                  }
                >
                  {l}
                </Chip>
              ))}
            </Box>
            <Input
              value={labelInput}
              placeholder="Add a label, press Enter"
              onChange={(e) => setLabelInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && labelInput.trim()) {
                  e.preventDefault();
                  if (!f.labels.includes(labelInput.trim()))
                    set("labels", [...f.labels, labelInput.trim()]);
                  setLabelInput("");
                }
              }}
            />
          </FormControl>
          <Box sx={{ display: "flex", gap: 1.5, flexWrap: "wrap" }}>
            <FormControl sx={{ flex: 1, minWidth: 150 }}>
              <FormLabel>Birthday</FormLabel>
              <Input type="date" value={x.birthday} onChange={(e) => setx("birthday", e.target.value)} slotProps={{ input: { "data-testid": "cx-birthday" } }} />
            </FormControl>
            <FormControl sx={{ flex: 1, minWidth: 150 }}>
              <FormLabel>Anniversary</FormLabel>
              <Input type="date" value={x.anniversary} onChange={(e) => setx("anniversary", e.target.value)} />
            </FormControl>
          </Box>
          <FormControl>
            <FormLabel>Website</FormLabel>
            <Input value={x.website} placeholder="https://" onChange={(e) => setx("website", e.target.value)} />
          </FormControl>
          <FormControl>
            <FormLabel>Address</FormLabel>
            <Input value={x.address} onChange={(e) => setx("address", e.target.value)} />
          </FormControl>
          <FormControl>
            <FormLabel>Notes</FormLabel>
            <Textarea
              minRows={2}
              value={f.notes}
              onChange={(e) => set("notes", e.target.value)}
            />
          </FormControl>
          {links && (links.events.length > 0 || links.crm_status || links.deals.length > 0) && (
            <Box data-testid="contact-links" sx={{ border: "1px solid", borderColor: "divider", borderRadius: "md", p: 1.25 }}>
              {links.events.length > 0 && (
                <>
                  <Typography level="title-sm">Upcoming meetings</Typography>
                  {links.events.slice(0, 4).map((ev) => (
                    <Typography key={ev.id} level="body-sm" component="a" href="/calendar" sx={{ display: "block" }}>
                      {new Date(ev.start).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })} · {ev.title}
                    </Typography>
                  ))}
                </>
              )}
              {(links.crm_status || links.deals.length > 0) && (
                <>
                  <Typography level="title-sm" sx={{ mt: 1 }}>CRM{links.crm_status ? ` · ${links.crm_status}` : ""}</Typography>
                  {links.deals.map((d) => (
                    <Typography key={d.id} level="body-sm" component="a" href="/crm" sx={{ display: "block" }}>
                      {d.title} · {d.stage} · {d.currency} {d.value.toLocaleString()}
                    </Typography>
                  ))}
                </>
              )}
            </Box>
          )}
          <Box
            sx={{ display: "flex", justifyContent: "flex-end", gap: 1, mt: 1 }}
          >
            <Button variant="plain" color="neutral" onClick={onClose}>
              Cancel
            </Button>
            <Button loading={saving} onClick={save}>
              Save
            </Button>
          </Box>
        </Stack>
      </ModalDialog>
    </Modal>
  );
}
