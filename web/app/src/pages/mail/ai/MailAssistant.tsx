import { useCallback, useMemo, useRef } from "react";
import { AssistantSlot } from "../../../components/AssistantSlot";
import type { MailMessage, MailThread } from "../types";
import { mailAdapter } from "./adapter";
import { createMailTools, mailContext, MAIL_SUGGESTIONS, type MailMsg } from "./tools";

export interface MailAssistantProps {
  openThread: { summary: MailThread; messages: MailMessage[] } | null;
  openCompose: (init: { to: string; cc?: string; subject: string; body: string }) => void;
  closeCompose: () => void;
  reload: () => void;
}

/** The Assistant button and right-hand panel of Mail. */
export function MailAssistant(p: MailAssistantProps) {
  const latest = useRef(p);
  latest.current = p;
  const tools = useMemo(
    () =>
      createMailTools(
        mailAdapter({
          openThreadId: () => latest.current.openThread?.summary.thread_id ?? null,
          openCompose: (init) => {
            latest.current.openCompose(init);
            return () => latest.current.closeCompose();
          },
          changed: () => latest.current.reload(),
        }),
      ),
    [],
  );
  const getContext = useCallback(() => {
    const t = latest.current.openThread;
    return mailContext(t ? { subject: t.summary.latest.subject, messages: t.messages as MailMsg[] } : null);
  }, []);
  const key = p.openThread?.summary.thread_id ?? "";
  return (
    <AssistantSlot
      app="mail"
      documentId="mail"
      documentTitle="Mail"
      tools={tools}
      getContext={getContext}
      // re-read the chip when another thread is opened
      subscribeContext={useCallback(
        (cb: () => void) => {
          const t = window.setTimeout(cb, 0);
          void key;
          return () => window.clearTimeout(t);
        },
        [key],
      )}
      suggestions={MAIL_SUGGESTIONS}
    />
  );
}
