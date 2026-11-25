// Voice typing through the browser's SpeechRecognition (where available;
// Chrome/Edge/Safari). Final phrases are inserted at the caret. The ribbon
// hides the button when the browser has no speech recognition.
import { useCallback, useEffect, useRef, useState } from "react";
import type { Editor } from "@tiptap/react";

interface Rec {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  onresult: ((e: { resultIndex: number; results: ArrayLike<{ isFinal: boolean; 0: { transcript: string } }> }) => void) | null;
  onend: (() => void) | null;
  onerror: (() => void) | null;
  start(): void;
  stop(): void;
}
type RecCtor = new () => Rec;

export function speechCtor(w: unknown = typeof window === "undefined" ? undefined : window): RecCtor | null {
  const x = w as { SpeechRecognition?: RecCtor; webkitSpeechRecognition?: RecCtor } | undefined;
  return x?.SpeechRecognition ?? x?.webkitSpeechRecognition ?? null;
}

export function useVoiceTyping(editor: Editor | null) {
  const Ctor = speechCtor();
  const [active, setActive] = useState(false);
  const rec = useRef<Rec | null>(null);
  const stop = useCallback(() => {
    rec.current?.stop();
    rec.current = null;
    setActive(false);
  }, []);
  useEffect(() => stop, [stop]);
  const toggle = useCallback(() => {
    if (!Ctor || !editor) return;
    if (rec.current) return stop();
    const r = new Ctor();
    r.continuous = true;
    r.interimResults = false;
    r.lang = navigator.language || "en-US";
    r.onresult = (e) => {
      for (let i = e.resultIndex; i < e.results.length; i++)
        if (e.results[i].isFinal) editor.chain().focus().insertContent(e.results[i][0].transcript.trim() + " ").run();
    };
    r.onend = () => {
      rec.current = null;
      setActive(false);
    };
    r.onerror = () => stop();
    rec.current = r;
    r.start();
    setActive(true);
  }, [Ctor, editor, stop]);
  return { supported: !!Ctor, active, toggle };
}
