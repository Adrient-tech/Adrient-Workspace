import { newElement, type SlideElement } from "./model";

/** A text box with `text` at the given box. */
export function newTextElement(text: string, box: { x: number; y: number; w: number; h: number }): SlideElement {
  return { ...newElement("text"), ...box, text };
}
