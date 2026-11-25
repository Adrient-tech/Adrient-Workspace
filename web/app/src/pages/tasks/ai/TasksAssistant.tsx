import { useCallback, useMemo, useRef } from "react";
import { AssistantSlot } from "../../../components/AssistantSlot";
import type { Task } from "../types";
import { tasksAdapter } from "./adapter";
import { createTaskTools, TASKS_SUGGESTIONS, tasksContext } from "./tools";

export interface TasksAssistantProps {
  listId: string;
  listName: string;
  tasks: Task[];
  reload: () => void;
}

/** The Assistant button and right-hand panel of Tasks. */
export function TasksAssistant(p: TasksAssistantProps) {
  const latest = useRef(p);
  latest.current = p;
  const tools = useMemo(() => createTaskTools(tasksAdapter({ currentListId: () => latest.current.listId, changed: () => latest.current.reload() })), []);
  const getContext = useCallback(() => tasksContext(latest.current.listName, latest.current.tasks), []);
  return <AssistantSlot app="tasks" documentId={p.listId} documentTitle={p.listName} tools={tools} getContext={getContext} suggestions={TASKS_SUGGESTIONS} />;
}
