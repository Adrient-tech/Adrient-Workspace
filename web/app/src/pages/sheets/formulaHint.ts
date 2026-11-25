// Formula help while typing: which function call is the caret inside, and
// which argument is it on? Pure text scan (strings and quoted sheet names are
// skipped); the tooltip itself is FormulaHint.tsx.

export interface ActiveCall {
  /** Upper-case function name. */
  name: string;
  /** Zero-based index of the argument the caret is in. */
  argIndex: number;
}

/**
 * activeCallAt returns the innermost unclosed function call around `caret`
 * (default: end of text), or null when the text is not a formula or the caret
 * is not inside a call's parentheses.
 */
export function activeCallAt(text: string, caret = text.length): ActiveCall | null {
  if (!text.startsWith("=")) return null;
  const s = text.slice(0, Math.max(0, Math.min(caret, text.length)));
  const stack: { name: string; arg: number }[] = [];
  for (let i = 1; i < s.length; i++) {
    const ch = s[i];
    if (ch === '"' || ch === "'") {
      let j = i + 1;
      while (j < s.length) {
        if (s[j] === ch) {
          if (s[j + 1] === ch) j += 2;
          else break;
        } else j++;
      }
      i = j;
      continue;
    }
    if (ch === "(") {
      let k = i - 1;
      while (k >= 0 && /\s/.test(s[k])) k--;
      let e = k;
      while (k >= 0 && /[A-Za-z0-9_.]/.test(s[k])) k--;
      const name = s.slice(k + 1, e + 1);
      stack.push({ name: /^[A-Za-z_]/.test(name) ? name.toUpperCase() : "", arg: 0 });
    } else if (ch === ")") {
      stack.pop();
    } else if ((ch === "," || ch === ";") && stack.length) {
      stack[stack.length - 1].arg++;
    }
  }
  for (let i = stack.length - 1; i >= 0; i--) {
    if (stack[i].name) return { name: stack[i].name, argIndex: stack[i].arg };
  }
  return null;
}

/** Index into a signature's args that `argIndex` maps to (repeating tails clamp to the last). */
export function hintArgIndex(args: { repeat?: boolean }[], argIndex: number): number {
  if (args.length === 0) return -1;
  if (argIndex < args.length) return argIndex;
  return args[args.length - 1].repeat ? args.length - 1 : -1;
}
