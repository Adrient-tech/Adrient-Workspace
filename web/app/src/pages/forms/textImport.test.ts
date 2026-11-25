import { describe, expect, it } from "vitest";
import { parseQuestions, scaleOf } from "./textImport";

describe("text import", () => {
  it("makes choices, checkboxes, scales and long answers", () => {
    const { questions: q, count } = parseQuestions(`1. How satisfied are you? *
a) Very satisfied
b) Satisfied
c) Not satisfied

2. Which features do you use? Select all that apply.
[ ] Reports
[ ] Invoices
[ ] Other (please specify)

3. On a scale of 0 to 10, how likely are you to recommend us?

4. Rate our support from 1 to 5 stars

5. Please describe your experience

6. What is your email?`);
    expect(count).toBe(6);
    expect(q[0]).toMatchObject({ type: "multiple_choice", title: "How satisfied are you?", required: true, options: ["Very satisfied", "Satisfied", "Not satisfied"] });
    expect(q[1]).toMatchObject({ type: "checkboxes", options: ["Reports", "Invoices", "Other (please specify)"] });
    expect(q[2]).toMatchObject({ type: "linear_scale", scale_min: 0, scale_max: 10 });
    expect(q[3]).toMatchObject({ type: "rating", scale_max: 5 });
    expect(q[4].type).toBe("paragraph");
    expect(q[5]).toMatchObject({ type: "short_answer", validation: { kind: "text", op: "email" } });
  });
  it("splits sections and starts questions at question marks without blank lines", () => {
    const { questions: q } = parseQuestions(`Section 1: About you
Your name?
Your city?
---
Page: Feedback
Anything else we should know? (required)`);
    expect(q.map((x) => [x.is_section, x.title])).toEqual([
      [true, "About you"], [false, "Your name?"], [false, "Your city?"], [true, "Feedback"], [false, "Anything else we should know?"],
    ]);
    expect(q[4].required).toBe(true);
    expect(q[4].type).toBe("paragraph");
  });
  it("treats unmarked short lines as choices, numbers like 1.5 as answers, and a dropdown past 12", () => {
    const { questions: q } = parseQuestions("Pick a colour\nRed\nGreen\nBlue\n\nHow long?\n- 1.5 hours\n- 2 hours");
    expect(q[0]).toMatchObject({ type: "multiple_choice", options: ["Red", "Green", "Blue"] });
    expect(q[1].options).toEqual(["1.5 hours", "2 hours"]);
    const many = "Pick one\n" + Array.from({ length: 13 }, (_, i) => `- Item ${i}`).join("\n");
    expect(parseQuestions(many).questions[0].type).toBe("dropdown");
  });
  it("is safe on junk and finds scales only with scale words", () => {
    expect(parseQuestions("").count).toBe(0);
    expect(parseQuestions("\n\n---\n\n").count).toBe(0);
    expect(scaleOf("children aged 1 to 5")).toBeNull();
    expect(scaleOf("a 1 to 10 scale")).toEqual([1, 10]);
  });
});
