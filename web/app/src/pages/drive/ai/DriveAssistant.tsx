import { useCallback, useMemo, useRef } from "react";
import { AssistantSlot } from "../../../components/AssistantSlot";
import type { DriveFile } from "../types";
import { driveAdapter } from "./adapter";
import { createDriveTools, DRIVE_SUGGESTIONS, driveContext } from "./tools";

export interface DriveAssistantProps {
  folderId: string;
  folderName: string;
  files: DriveFile[];
  reload: () => void;
}

/** The Assistant button and right-hand panel of Drive. */
export function DriveAssistant(p: DriveAssistantProps) {
  const latest = useRef(p);
  latest.current = p;
  const tools = useMemo(() => createDriveTools(driveAdapter({ currentFolder: () => latest.current.folderId, changed: () => latest.current.reload() })), []);
  const getContext = useCallback(() => driveContext(latest.current.folderName, latest.current.files), []);
  return <AssistantSlot app="drive" documentId="drive" documentTitle="Drive" tools={tools} getContext={getContext} suggestions={DRIVE_SUGGESTIONS} />;
}
