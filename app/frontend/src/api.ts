export type StepType = "question" | "task";
export type AnswerType = "text" | "single_choice";

/** Sidebar reference tab from workshop `tabs.markdowns` (Material Symbols icon name in `icon`). */
export type SidebarTab = {
  id: string;
  title: string;
  content: string;
  /** Material Symbols icon id, e.g. `description`, `info`. */
  icon?: string;
};

export type CurrentStep = {
  id: string;
  type: StepType;
  title: string;
  description?: string;
  answer_type?: AnswerType;
  options?: string[];
  /** Shown when verify fails; omit to use UI default. */
  incorrect_message?: string;
  hints?: string[];
  setupDone: boolean;
  completed: boolean;
};

export type WorkshopState = {
  name: string;
  error?: string;
  totalSteps: number;
  currentStepIndex: number;
  totalQuestions: number;
  currentQuestionNumber: number;
  done: boolean;
  current?: CurrentStep;
  lastSetupLogs?: string;
  lastVerifyLogs?: string;
  lastTaskLogs?: string;
  sidebarTabs?: SidebarTab[];
};

export async function getWorkshop(): Promise<WorkshopState> {
  const res = await fetch("/api/workshop");
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function restartWorkshop(): Promise<WorkshopState> {
  const res = await fetch("/api/workshop/restart", { method: "POST" });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((body as { error?: string }).error || res.statusText);
  return body as WorkshopState;
}

export async function runTask(): Promise<{ ok: boolean; logs: string; state: WorkshopState }> {
  const res = await fetch("/api/task/run", { method: "POST" });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

export async function runQuestionSetup(): Promise<{ ok: boolean; logs: string; state: WorkshopState }> {
  const res = await fetch("/api/question/setup", { method: "POST" });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

export async function submitAnswer(answer: string): Promise<{ ok: boolean; logs: string; state: WorkshopState }> {
  const res = await fetch("/api/question/submit", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ answer }),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

export function terminalWsUrl(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/api/ws/terminal`;
}
