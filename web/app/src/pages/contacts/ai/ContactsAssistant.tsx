import { useCallback, useMemo, useRef } from "react";
import { AssistantSlot } from "../../../components/AssistantSlot";
import { listContacts, updateContact } from "../api";
import type { Contact, ContactInput } from "../types";
import { CONTACTS_SUGGESTIONS, contactsContext, createContactTools, type ContactRow } from "./tools";

export interface ContactsAssistantProps {
  selected: Contact | null;
  total: number;
  reload: () => void;
}

const input = (c: Contact, notes: string): ContactInput => ({
  display_name: c.display_name, first_name: c.first_name, last_name: c.last_name, company: c.company, job_title: c.job_title,
  emails: c.emails, phones: c.phones, labels: c.labels, notes, starred: c.starred,
});

/** The Assistant button and right-hand panel of Contacts. */
export function ContactsAssistant(p: ContactsAssistantProps) {
  const latest = useRef(p);
  latest.current = p;
  const tools = useMemo(
    () =>
      createContactTools({
        all: async () => (await listContacts()) as ContactRow[],
        updateNotes: async (c, notes) => {
          await updateContact(c.id, input(c as Contact, notes));
          latest.current.reload();
        },
        selected: () => latest.current.selected as ContactRow | null,
      }),
    [],
  );
  const getContext = useCallback(() => contactsContext(latest.current.selected as ContactRow | null, latest.current.total), []);
  return <AssistantSlot app="contacts" documentId="contacts" documentTitle="Contacts" tools={tools} getContext={getContext} suggestions={CONTACTS_SUGGESTIONS} />;
}
