import { describe, expect, it } from "vitest";
import { SURVEY_TEMPLATES } from "./surveyTemplates";

describe("survey templates", () => {
  it("have unique ids, titled questions and valid options/scales", () => {
    expect(new Set(SURVEY_TEMPLATES.map((t) => t.id)).size).toBe(SURVEY_TEMPLATES.length);
    for (const t of SURVEY_TEMPLATES) {
      expect((t.input.questions ?? []).length).toBeGreaterThan(1);
      for (const q of t.input.questions ?? []) {
        expect(q.title).not.toBe("");
        if (["multiple_choice", "checkboxes", "dropdown"].includes(q.type)) expect(q.options.length).toBeGreaterThan(1);
        if (q.type === "multiple_choice_grid") expect((q.rows ?? []).length).toBeGreaterThan(1);
        if (q.type === "linear_scale") expect(q.scale_max).toBeGreaterThan(q.scale_min);
      }
    }
  });
});
