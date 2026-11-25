import { useCallback, useMemo, useRef } from "react";
import { useNavigate } from "react-router-dom";
import { AssistantSlot } from "../../../components/AssistantSlot";
import { createForm, getSummary, trashForm } from "../api";
import type { Form } from "../types";
import { createFormTools, formContext, FORMS_LIST_SUGGESTIONS, FORMS_SUGGESTIONS } from "./tools";

export interface FormsAssistantProps {
  /** The open form (editor) or null (list). */
  form: Form | null;
  /** Editor only: apply a change; the editor autosaves. */
  mutate?: (fn: (f: Form) => Form) => void;
  /** Editor only: latest form incl. changes not rendered yet. */
  getForm?: () => Form | null;
  /** List only: refresh after create/undo. */
  reload?: () => void;
}

/** The Assistant button and right-hand panel of Forms (editor and list). */
export function FormsAssistant(p: FormsAssistantProps) {
  const navigate = useNavigate();
  const latest = useRef({ ...p, navigate });
  latest.current = { ...p, navigate };
  const tools = useMemo(
    () =>
      createFormTools({
        current: () => latest.current.getForm?.() ?? latest.current.form,
        mutate: p.mutate ? (fn) => latest.current.mutate?.(fn) : undefined,
        create: p.form ? undefined : async (i) => {
          const f = await createForm({ title: i.title, description: i.description, questions: i.questions });
          latest.current.navigate(`/forms/d/${f.id}`);
          return { id: f.id, title: f.title };
        },
        trash: async (id) => {
          await trashForm(id);
          latest.current.reload?.();
          if (latest.current.form?.id === id) latest.current.navigate("/forms");
        },
        summary: (id) => getSummary(id),
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [!!p.mutate],
  );
  const getContext = useCallback(() => formContext(latest.current.getForm?.() ?? latest.current.form), []);
  return (
    <AssistantSlot
      app="forms"
      documentId={p.form?.id ?? "forms"}
      documentTitle={p.form?.title ?? "Forms"}
      tools={tools}
      getContext={getContext}
      suggestions={p.form ? FORMS_SUGGESTIONS : FORMS_LIST_SUGGESTIONS}
    />
  );
}
