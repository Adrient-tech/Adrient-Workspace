import { useCallback, useMemo, useRef } from "react";
import { AssistantSlot } from "../../../components/AssistantSlot";
import type { TChannel, TMessage } from "../types";
import { chatAdapter } from "./adapter";
import { CHAT_SUGGESTIONS, chatContext, createChatTools } from "./tools";

export interface ChatAssistantProps {
  channelId: string;
  channelName: string;
  channels: TChannel[];
  messages: TMessage[];
  reload: () => void;
}

/** The Assistant button and right-hand panel of Chat. */
export function ChatAssistant(p: ChatAssistantProps) {
  const latest = useRef(p);
  latest.current = p;
  const tools = useMemo(() => createChatTools(chatAdapter({ channels: () => latest.current.channels, currentChannelId: () => latest.current.channelId, changed: () => latest.current.reload() })), []);
  const getContext = useCallback(() => chatContext(latest.current.channelName, latest.current.messages), []);
  return <AssistantSlot app="chat" documentId={p.channelId} documentTitle={p.channelName} tools={tools} getContext={getContext} suggestions={CHAT_SUGGESTIONS} />;
}
