import { useCallback, useMemo, useRef } from "react";
import { AssistantSlot } from "../../../components/AssistantSlot";
import { createNote, listNotes, trashNote, updateNote } from "../api";
import { createKeepTools, KEEP_SUGGESTIONS, keepContext, type KNote } from "./tools";

export interface KeepAssistantProps {
  count: number;
  filter: string;
  reload: () => void;
}

/** The Assistant button and right-hand panel of Keep. */
export function KeepAssistant(p: KeepAssistantProps) {
  const latest = useRef(p);
  latest.current = p;
  const tools = useMemo(() => {
    const done = <T,>(r: T) => {
      latest.current.reload();
      return r;
    };
    return createKeepTools({
      list: async () => (await listNotes()) as KNote[],
      create: async (i) => done((await createNote(i)) as KNote),
      update: async (id, i) => done((await updateNote(id, i)) as KNote),
      trash: async (id) => done(await trashNote(id)),
    });
  }, []);
  const getContext = useCallback(() => keepContext(latest.current.count, latest.current.filter), []);
  return <AssistantSlot app="keep" documentId="keep" documentTitle="Keep" tools={tools} getContext={getContext} suggestions={KEEP_SUGGESTIONS} />;
}
