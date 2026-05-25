export type StepType = "question" | "task";
export type AnswerType = "text" | "single_choice" | "observe";

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
  /** Shown after verify succeeds; optional. */
  correct_message?: string;
  hints?: string[];
  poll_interval_seconds?: number;
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
  labId?: string;
  labsRoot?: string;
};

export type LabEntry = {
  id: string;
  name?: string;
  description?: string;
  stepCount?: number;
  valid: boolean;
  error?: string;
};

export type LabCatalog = {
  labsRoot: string;
  activeId: string;
  labs: LabEntry[];
};

export async function getLabs(): Promise<LabCatalog> {
  const res = await fetch("/api/labs");
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function selectLab(id: string): Promise<WorkshopState> {
  const res = await fetch("/api/labs/select", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id }),
    signal: AbortSignal.timeout(300_000),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((body as { error?: string }).error || res.statusText);
  return (body as { state: WorkshopState }).state;
}

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

export const LAB_RESTART_FAILED_MSG =
  "Lab restart failed and the cluster may be unavailable. Stop and recreate the container (docker run …) to recover.";

export type LabStatus = {
  cluster: "ready" | "resetting" | "unavailable";
};

export async function getLabStatus(): Promise<LabStatus> {
  const res = await fetch("/api/lab/status");
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export async function waitForLabReady(timeoutMs = 120_000): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const status = await getLabStatus();
    if (status.cluster === "ready") return;
    if (status.cluster === "unavailable") {
      throw new Error(LAB_RESTART_FAILED_MSG);
    }
    await new Promise((resolve) => window.setTimeout(resolve, 2000));
  }
  throw new Error(LAB_RESTART_FAILED_MSG);
}

export async function restartLab(): Promise<WorkshopState> {
  const res = await fetch("/api/lab/restart", {
    method: "POST",
    signal: AbortSignal.timeout(300_000),
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((body as { error?: string }).error || res.statusText);
  return (body as { state: WorkshopState }).state;
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

export async function checkQuestion(): Promise<{ ok: boolean; logs: string; state: WorkshopState }> {
  const res = await fetch("/api/question/check", { method: "POST" });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || res.statusText);
  return body;
}

export async function advanceQuestion(): Promise<{ state: WorkshopState }> {
  const res = await fetch("/api/question/next", { method: "POST" });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((body as { error?: string }).error || res.statusText);
  return body as { state: WorkshopState };
}

export function terminalWsUrl(): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${location.host}/api/ws/terminal`;
}

export type ExposedEndpoint = {
  id: string;
  kind: "nodeport" | "ingress";
  namespace: string;
  name: string;
  label: string;
  url: string;
  port?: number;
};

export type ExposedSnapshot = {
  endpoints: ExposedEndpoint[];
};

export async function getExposed(): Promise<ExposedSnapshot> {
  const res = await fetch("/api/exposed");
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}

export function exposedStreamUrl(): string {
  return "/api/stream/exposed";
}

export const LAST_LAB_STORAGE_KEY = "k3slab:lastLab";

export function needsLabSelection(catalog: LabCatalog, workshop: WorkshopState | null): boolean {
  const validCount = catalog.labs.filter((l) => l.valid).length;
  if (validCount === 0) return false;
  if (validCount === 1) return false;
  if (workshop?.error?.toLowerCase().includes("select a lab")) return true;
  return !catalog.activeId;
}
