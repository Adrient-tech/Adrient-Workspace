/* eslint-disable @typescript-eslint/no-explicit-any */
// Thin typed client for /api/v1/talent and the public career API.
const BASE = "/api/v1/talent";
const PUB = "/api/v1/public/talent";

export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.status = status;
  }
}

async function jf<T>(url: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(url, {
    credentials: "same-origin",
    headers: { Accept: "application/json", ...(init?.body && !(init.body instanceof FormData) ? { "Content-Type": "application/json" } : {}) },
    ...init,
  });
  if (!resp.ok) {
    const text = (await resp.text().catch(() => "")).trim();
    let msg = text;
    try {
      const j = JSON.parse(text);
      msg = j.message || j.error || text;
    } catch {
      /* plain text */
    }
    throw new ApiError(msg || `HTTP ${resp.status}`, resp.status);
  }
  return (await resp.json()) as T;
}
const send = (method: string, body?: unknown): RequestInit => ({ method, body: body === undefined ? undefined : JSON.stringify(body) });
const list = async <T,>(url: string, key: string): Promise<T[]> => ((await jf<Record<string, T[] | null>>(url))[key] ?? []) as T[];

export interface Me { user_id: string; is_hr: boolean }
export interface Person { id: string; name: string; email: string; job_title: string; department: string; manager_id: string }
export interface Stage { id: string; name: string }
export interface Settings {
  stages: Stage[]; hr_user_ids: string[]; career_enabled: boolean; career_title: string; career_blurb: string;
  privacy_text: string; retention_days: number; currency: string; offer_template: string;
}
export interface Job {
  id: string; title: string; department: string; location: string; employment_type: string; skills: string[]; description: string;
  salary_min: number | null; salary_max: number | null; currency: string; openings: number; status: "draft" | "open" | "closed";
  approval_status: "none" | "pending" | "approved" | "rejected"; recruiter_id: string; hiring_manager_id: string; team_ids: string[];
  active_applicants?: number; total_applicants?: number; hired?: number; created_by: string;
}
export interface Application {
  id: string; job_id: string; candidate_id: string; stage: string; status: "active" | "hired" | "rejected" | "withdrawn";
  reject_reason: string; cover_note: string; applied_at: string; stage_entered_at: string;
  candidate_name: string; candidate_email: string; candidate_phone: string; source: string; source_detail: string;
  resume_name: string; has_resume: boolean; talent_pool: boolean; job_title: string; avg_score: number | null; interviews: number;
}
export interface Interview { id: string; title: string; starts_at: string; ends_at: string; status: string; meet_code: string; mail_sent: boolean; calendar_event_id: string }
export interface Scorecard { id: string; reviewer_id: string; reviewer_name: string; ratings: Record<string, number>; overall: number; recommendation: string; comment: string; answers?: { question: string; answer: string }[] | null }
export interface GuideQuestion { text: string; dimension?: string }
export interface Guide { id: string; name: string; stage: string; format: string; questions: GuideQuestion[] }
export interface Offer { id: string; job_title: string; department: string; salary: number; currency: string; start_date: string | null; status: string; accept_token: string; responded_name: string }
export interface AppDetail extends Omit<Application, "interviews"> {
  history: { from_stage: string; to_stage: string; note: string; at: string; by_name: string }[];
  interviews: Interview[]; scorecards: Scorecard[]; offers: Offer[]; candidate_location: string; consent_at: string | null;
}
export interface Reports {
  time_to_hire: { job_id: string; title: string; hired: number; avg_days: number }[]; avg_days_to_hire: number;
  sources: { source: string; applications: number; interviewed: number; hired: number; hire_rate: number }[];
  funnel: { stage: string; name: string; count: number }[];
}

export const getMe = () => jf<Me>(`${BASE}/me`);
export const directory = () => list<Person>(`${BASE}/directory`, "people");
export const getSettings = () => jf<Settings>(`${BASE}/settings`);
export const saveSettings = (s: Settings) => jf<Settings>(`${BASE}/settings`, send("PUT", s));

export const listJobs = () => list<Job>(`${BASE}/jobs`, "jobs");
export const saveJob = (id: string, j: Partial<Job>) => jf<Job>(id ? `${BASE}/jobs/${id}` : `${BASE}/jobs`, send(id ? "PUT" : "POST", j));
export const deleteJob = (id: string) => jf<unknown>(`${BASE}/jobs/${id}`, send("DELETE"));
export const jobAction = (id: string, action: "request-approval" | "approve" | "reject") => jf<Job>(`${BASE}/jobs/${id}/${action}`, send("POST"));
export const setJobStatus = (id: string, status: string) => jf<Job>(`${BASE}/jobs/${id}/status`, send("POST", { status }));

export const listApplications = (jobId = "") => list<Application>(`${BASE}/applications${jobId ? `?job_id=${jobId}` : ""}`, "applications");
export const getApplication = (id: string) => jf<AppDetail>(`${BASE}/applications/${id}`);
export const addCandidate = (b: Record<string, string>) => jf<AppDetail>(`${BASE}/applications`, send("POST", b));
export const moveApplication = (id: string, stage: string, note = "") => jf<AppDetail>(`${BASE}/applications/${id}/move`, send("POST", { stage, note }));
export const rejectApplication = (id: string, reason: string) => jf<AppDetail>(`${BASE}/applications/${id}/reject`, send("POST", { reason }));
export const hireApplication = (id: string) => jf<AppDetail>(`${BASE}/applications/${id}/hire`, send("POST"));
export const restoreApplication = (id: string) => jf<AppDetail>(`${BASE}/applications/${id}/restore`, send("POST"));
export const saveScorecard = (id: string, b: unknown) => jf<Scorecard[]>(`${BASE}/applications/${id}/scorecard`, send("POST", b));
export const listGuides = () => list<Guide>(`${BASE}/guides`, "guides");
export const saveGuide = (id: string, b: Partial<Guide>) => jf<Guide>(id ? `${BASE}/guides/${id}` : `${BASE}/guides`, send(id ? "PUT" : "POST", b));
export const deleteGuide = (id: string) => jf<unknown>(`${BASE}/guides/${id}`, send("DELETE"));
export const scheduleInterview = (id: string, b: unknown) => jf<Interview>(`${BASE}/applications/${id}/interviews`, send("POST", b));
export const cancelInterview = (id: string) => jf<unknown>(`${BASE}/interviews/${id}`, send("DELETE"));
export const createOffer = (id: string, b: unknown) => jf<Offer>(`${BASE}/applications/${id}/offers`, send("POST", b));
export const sendOffer = (id: string) => jf<{ mail_sent: boolean; link: string }>(`${BASE}/offers/${id}/send`, send("POST"));
export const withdrawOffer = (id: string) => jf<unknown>(`${BASE}/offers/${id}`, send("DELETE"));
export const newHire = (id: string) => jf<Record<string, string | number>>(`${BASE}/applications/${id}/new-hire`);
export interface HireToHR { created: boolean; employee: { id: string; emp_code: string; name: string }; checklist_id: string; project_id?: string; links: { app: string; label: string; href: string }[]; notes: string[] }
export const hireToHR = (applicationId: string, createProject: boolean) => jf<HireToHR>("/api/v1/bridge/talent/hire", send("POST", { application_id: applicationId, create_project: createProject }));
export const checkDuplicate = (email: string) => jf<{ duplicate: { name: string; applications: number } | null }>(`${BASE}/candidates?email=${encodeURIComponent(email)}`);
export const setPool = (cid: string, inPool: boolean, tags?: string[]) => jf<unknown>(`${BASE}/candidates/${cid}/pool`, send("POST", { in: inPool, tags }));
export const eraseCandidate = (cid: string) => jf<unknown>(`${BASE}/candidates/${cid}`, send("DELETE"));
export const listPool = () => list<any>(`${BASE}/pool`, "pool");
export const poolApply = (cid: string, jobId: string) => jf<unknown>(`${BASE}/pool/${cid}/apply`, send("POST", { job_id: jobId }));
export const getReports = (jobId = "") => jf<Reports>(`${BASE}/reports${jobId ? `?job_id=${jobId}` : ""}`);
export const resumeUrl = (cid: string) => `${BASE}/candidates/${cid}/resume`;
export const offerPdfUrl = (id: string) => `${BASE}/offers/${id}/pdf`;

// onboarding
export const listTemplates = () => list<any>(`${BASE}/templates`, "templates");
export const saveTemplate = (id: string, b: unknown) => jf<any>(id ? `${BASE}/templates/${id}` : `${BASE}/templates`, send(id ? "PUT" : "POST", b));
export const deleteTemplate = (id: string) => jf<unknown>(`${BASE}/templates/${id}`, send("DELETE"));
export const listChecklists = () => list<any>(`${BASE}/checklists`, "checklists");
export const getChecklist = (id: string) => jf<any>(`${BASE}/checklists/${id}`);
export const startChecklist = (b: unknown) => jf<any>(`${BASE}/checklists`, send("POST", b));
export const setTaskDone = (id: string, done: boolean) => jf<any>(`${BASE}/checklist-tasks/${id}/done`, send("POST", { done }));
export const myChecklistTasks = () => list<any>(`${BASE}/checklist-tasks`, "tasks");
export const remindOverdue = () => jf<{ sent: number }>(`${BASE}/checklist-tasks`, send("POST"));
export const addAsset = (cid: string, b: unknown) => jf<any>(`${BASE}/checklists/${cid}/assets`, send("POST", b));
export const setAssetStatus = (id: string, status: string) => jf<any>(`${BASE}/assets/${id}/status`, send("POST", { status }));
export const saveExit = (cid: string, b: unknown) => jf<any>(`${BASE}/checklists/${cid}/exit-interview`, send("POST", b));

// performance
export const listGoals = (owner = "", scope = "") => list<any>(`${BASE}/goals?${owner ? `owner=${owner}&` : ""}${scope ? `scope=${scope}` : ""}`, "goals");
export const saveGoal = (id: string, b: unknown) => jf<any>(id ? `${BASE}/goals/${id}` : `${BASE}/goals`, send(id ? "PUT" : "POST", b));
export const deleteGoal = (id: string) => jf<unknown>(`${BASE}/goals/${id}`, send("DELETE"));
export const listCycles = () => list<any>(`${BASE}/cycles`, "cycles");
export const saveCycle = (b: unknown) => jf<any>(`${BASE}/cycles`, send("POST", b));
export const setCycleStatus = (id: string, status: string) => jf<any>(`${BASE}/cycles/${id}/status`, send("POST", { status }));
export const myReviews = () => list<any>(`${BASE}/reviews`, "reviews");
export const submitReview = (id: string, b: unknown) => jf<unknown>(`${BASE}/reviews/${id}/submit`, send("POST", b));
export const getResult = (cycle: string, subject: string) => jf<any>(`${BASE}/cycles/${cycle}/result/${subject}`);
export const calibration = (cycle: string) => list<any>(`${BASE}/cycles/${cycle}/calibration`, "rows");
export const calibrate = (cycle: string, subject: string, rating: number) => jf<unknown>(`${BASE}/cycles/${cycle}/calibrate/${subject}`, send("POST", { rating }));
export const assignPeers = (cycle: string, subject: string, peers: string[]) => jf<unknown>(`${BASE}/cycles/${cycle}/peers/${subject}`, send("POST", { peer_ids: peers }));
export const listOneOnOnes = (withId = "") => list<any>(`${BASE}/one-on-ones${withId ? `?with=${withId}` : ""}`, "one_on_ones");
export const saveOneOnOne = (id: string, b: unknown) => jf<any>(id ? `${BASE}/one-on-ones/${id}` : `${BASE}/one-on-ones`, send(id ? "PUT" : "POST", b));
export const feedbackToGive = () => list<any>(`${BASE}/feedback`, "feedback");
export const feedbackReceived = () => list<any>(`${BASE}/feedback?box=received`, "feedback");
export const requestFeedback = (b: unknown) => jf<unknown>(`${BASE}/feedback`, send("POST", b));
export const answerFeedback = (id: string, response: string, decline = false) => jf<unknown>(`${BASE}/feedback/${id}/answer`, send("POST", { response, decline }));

// learning, policies, feed
export const listCourses = () => list<any>(`${BASE}/courses`, "courses");
export const saveCourse = (id: string, b: unknown) => jf<any>(id ? `${BASE}/courses/${id}` : `${BASE}/courses`, send(id ? "PUT" : "POST", b));
export const enroll = (id: string, b: unknown) => jf<{ enrolled: number }>(`${BASE}/courses/${id}/enroll`, send("POST", b));
export const courseProgress = (id: string, status: string) => jf<unknown>(`${BASE}/courses/${id}/progress`, send("POST", { status }));
export const courseTracking = (id: string) => list<any>(`${BASE}/courses/${id}/tracking`, "rows");
export const listPolicies = () => list<any>(`${BASE}/policies`, "policies");
export const savePolicy = (id: string, b: unknown) => jf<any>(id ? `${BASE}/policies/${id}` : `${BASE}/policies`, send(id ? "PUT" : "POST", b));
export const acknowledge = (id: string) => jf<unknown>(`${BASE}/policies/${id}/acknowledge`, send("POST"));
export const policyTracking = (id: string) => list<any>(`${BASE}/policies/${id}/tracking`, "rows");
export const feed = () => list<any>(`${BASE}/feed`, "feed");
export const postAnnouncement = (b: unknown) => jf<any>(`${BASE}/announcements`, send("POST", b));
export const deleteAnnouncement = (id: string) => jf<unknown>(`${BASE}/announcements/${id}`, send("DELETE"));
export const giveKudos = (b: unknown) => jf<unknown>(`${BASE}/kudos`, send("POST", b));
export const celebrations = () => list<any>(`${BASE}/celebrations`, "celebrations");
export const setBirthday = (b: unknown) => jf<unknown>(`${BASE}/celebrations`, send("POST", b));

// public career page and offer e-accept (no sign-in)
export interface PublicJob { id: string; title: string; department: string; location: string; employment_type: string; skills: string[]; description: string; salary_min: number | null; salary_max: number | null; currency: string }
export const publicJobs = (org: string) => jf<{ title: string; blurb: string; privacy_text: string; jobs: PublicJob[] }>(`${PUB}/${encodeURIComponent(org)}/jobs`);
export const publicApply = (org: string, jobId: string, form: FormData) =>
  jf<{ ok: boolean }>(`${PUB}/${encodeURIComponent(org)}/jobs/${jobId}/apply`, { method: "POST", body: form });
export const publicOffer = (token: string) => jf<{ candidate: string; company: string; job_title: string; body: string; status: string; expired: boolean }>(`${PUB}/offers/${token}`);
export const publicOfferRespond = (token: string, accept: boolean, name: string) => jf<unknown>(`${PUB}/offers/${token}/respond`, send("POST", { accept, name }));
export const publicOfferPdfUrl = (token: string) => `${PUB}/offers/${token}/pdf`;
