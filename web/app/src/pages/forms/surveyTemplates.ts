// Survey starters (customer feedback, NPS, employee, event, course, market research), adapted to Forms question types.
import { blankQuestion, newQuestionId, type FormTemplate } from "./helpers";
import type { FormQuestion, QuestionType } from "./types";

function q(p: Partial<FormQuestion> & { type: QuestionType; title: string }): FormQuestion {
  return { ...blankQuestion(p.type), ...p, id: newQuestionId() };
}
const AGREE = ["Strongly agree", "Agree", "Neutral", "Disagree", "Strongly disagree"];
const nps = (title: string) => q({ type: "linear_scale", title, scale_min: 0, scale_max: 10, scale_min_label: "Not at all likely", scale_max_label: "Extremely likely", required: true });

export const SURVEY_TEMPLATES: FormTemplate[] = [
  {
    id: "survey-customer-feedback",
    name: "Customer feedback",
    category: "Survey",
    input: {
      title: "Customer feedback",
      description: "Tell us how we did. It takes about two minutes.",
      questions: [
        q({ type: "rating", title: "Overall, how satisfied are you with us?", scale_max: 5, required: true }),
        q({ type: "multiple_choice", title: "How easy was it to get what you needed?", options: ["Very easy", "Easy", "Neither easy nor difficult", "Difficult", "Very difficult"] }),
        q({ type: "checkboxes", title: "What did you like most?", options: ["Quality", "Price", "Support", "Delivery", "Ease of use"] }),
        q({ type: "paragraph", title: "What could we do better?" }),
        q({ type: "short_answer", title: "Email (only if you want a reply)", validation: { kind: "text", op: "email" } }),
      ],
    },
  },
  {
    id: "survey-nps",
    name: "Recommendation score (NPS)",
    category: "Survey",
    input: {
      title: "Would you recommend us?",
      description: "One question and an optional comment.",
      questions: [nps("How likely are you to recommend us to a friend or colleague?"), q({ type: "paragraph", title: "What is the main reason for your score?" })],
    },
  },
  {
    id: "survey-employee-engagement",
    name: "Employee engagement",
    category: "Survey",
    input: {
      title: "Employee engagement",
      description: "Anonymous. Your honest answers help us improve.",
      questions: [
        q({ type: "multiple_choice_grid", title: "How much do you agree?", rows: ["I understand what is expected of me", "I have the tools I need", "My manager supports me", "I see a future for myself here"], options: AGREE, required: true }),
        nps("How likely are you to recommend working here to a friend?"),
        q({ type: "paragraph", title: "What one thing would make work better?" }),
      ],
    },
  },
  {
    id: "survey-event-feedback",
    name: "Event feedback",
    category: "Survey",
    input: {
      title: "Event feedback",
      description: "Thanks for coming. How was it?",
      questions: [
        q({ type: "rating", title: "How would you rate the event overall?", scale_max: 5, required: true }),
        q({ type: "multiple_choice", title: "Was the event worth your time?", options: ["Definitely", "Mostly", "Not really"] }),
        q({ type: "checkboxes", title: "Which parts did you enjoy?", options: ["Speakers", "Networking", "Venue", "Food and drink", "Activities"] }),
        q({ type: "paragraph", title: "Suggestions for next time" }),
      ],
    },
  },
  {
    id: "survey-course-evaluation",
    name: "Course evaluation",
    category: "Survey",
    input: {
      title: "Course evaluation",
      description: "Help us improve this course.",
      questions: [
        q({ type: "multiple_choice_grid", title: "Rate the following", rows: ["The content was clear", "The pace was right", "The instructor was engaging", "The materials were useful"], options: AGREE, required: true }),
        q({ type: "rating", title: "Overall rating of the course", scale_max: 5, required: true }),
        q({ type: "paragraph", title: "What should we change?" }),
      ],
    },
  },
  {
    id: "survey-market-research",
    name: "Market research",
    category: "Survey",
    input: {
      title: "Product research",
      description: "A few questions about how you work today.",
      questions: [
        q({ type: "dropdown", title: "Which best describes your role?", options: ["Owner or founder", "Manager", "Individual contributor", "Student", "Other"], required: true }),
        q({ type: "multiple_choice", title: "How often do you use tools like ours?", options: ["Daily", "Weekly", "Monthly", "Rarely", "Never"] }),
        q({ type: "checkboxes", title: "Which problems do you run into?", options: ["Too expensive", "Too complicated", "Missing features", "Poor support", "Data is hard to move"] }),
        q({ type: "paragraph", title: "Describe your ideal solution" }),
      ],
    },
  },
];
