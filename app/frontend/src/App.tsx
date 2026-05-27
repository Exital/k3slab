import { useCallback, useEffect, useMemo, useRef, useState, type Dispatch, type SetStateAction } from "react";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import {
  advanceQuestion,
  checkQuestion,
  getLabStatus,
  getLabs,
  getWorkshop,
  LAST_LAB_STORAGE_KEY,
  LAB_RESTART_FAILED_MSG,
  needsLabSelection,
  restartLab,
  runQuestionSetup,
  runTask,
  selectLab,
  submitAnswer,
  waitForLabReady,
  type LabCatalog,
  type LabStatus,
  type WorkshopState,
} from "./api";
import { LabPicker, LabSwitcher } from "./LabPicker";
import { MarkdownContent } from "./components/MarkdownContent";
import { MIcon } from "./components/MIcon";
import { useExposedEndpoints } from "./hooks/useExposedEndpoints";
import { useTerminalDetach } from "./hooks/useTerminalDetach";
import { TerminalView } from "./terminal/TerminalView";
import type { ThemeMode } from "./theme";
import { playVerifyTone } from "./playTone";

export type { ThemeMode };

/** Default copy when verify fails and the question has no `incorrect_message` in YAML. */
const DEFAULT_WRONG_ANSWER_MSG =
  "That doesn't match what we expected. Double-check your answer or the cluster state, then try again.";

type VerifyOutcome = "idle" | "success" | "failure";
const CLUSTER_STATUS_STORAGE_KEY = "k3slab:lastClusterStatus";
const CLUSTER_STATUS_VALUES: readonly LabStatus["cluster"][] = ["ready", "resetting", "unavailable"];

function readStoredClusterStatus(): LabStatus["cluster"] | null {
  try {
    const raw = localStorage.getItem(CLUSTER_STATUS_STORAGE_KEY);
    if (!raw) return null;
    if ((CLUSTER_STATUS_VALUES as readonly string[]).includes(raw)) {
      return raw as LabStatus["cluster"];
    }
    return null;
  } catch {
    return null;
  }
}

function DismissibleMessagePanel({
  variant,
  markdown,
  text,
  onDismiss,
}: {
  variant: "success" | "error";
  markdown: boolean;
  text: string;
  onDismiss: () => void;
}) {
  const isSuccess = variant === "success";
  return (
    <div
      className={
        isSuccess
          ? "rounded-xl border border-emerald-200 bg-emerald-50/90 p-4 text-sm dark:border-k3-secondary/35 dark:bg-k3-secondary-container/15"
          : "rounded-xl border border-rose-200 bg-rose-50/90 p-4 text-sm dark:border-k3-error-container/50 dark:bg-k3-surface-container"
      }
    >
      <div className="flex items-start gap-3">
        <div className="min-w-0 flex-1">
      {markdown ? (
        <div
          className={
            isSuccess
              ? "text-emerald-900 dark:text-k3-secondary [&_code]:rounded [&_code]:bg-emerald-100 [&_code]:px-1.5 [&_code]:text-emerald-950 dark:[&_code]:bg-k3-surface-container-highest dark:[&_code]:text-k3-on-surface [&_p]:mb-2 [&_p:last-child]:mb-0"
              : "text-rose-900 dark:text-k3-on-error-container [&_code]:rounded [&_code]:bg-rose-100 [&_code]:px-1.5 dark:[&_code]:bg-k3-surface-lowest [&_p]:mb-2 [&_p:last-child]:mb-0"
          }
        >
          <MarkdownContent>{text}</MarkdownContent>
        </div>
      ) : (
        <p className={isSuccess ? "text-emerald-900 dark:text-k3-secondary" : "text-rose-900 dark:text-k3-on-error-container"}>
          {text}
        </p>
      )}
        </div>
        <button
          type="button"
          className={
            isSuccess
              ? "flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-200/80 hover:text-slate-800 dark:text-k3-on-surface-variant dark:hover:bg-k3-surface-container-high dark:hover:text-k3-on-surface"
              : "flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-slate-500 transition hover:bg-slate-200/80 hover:text-slate-800 dark:text-k3-on-surface-variant dark:hover:bg-k3-surface-variant dark:hover:text-k3-on-surface"
          }
          aria-label="Dismiss message"
          onClick={onDismiss}
        >
          <MIcon name="close" className="!text-lg" />
        </button>
      </div>
    </div>
  );
}

function ClusterStatusBadge({ status }: { status: LabStatus["cluster"] | null }) {
  if (status === null) {
    return (
      <span
        className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-slate-300/70 bg-slate-100 px-2.5 py-1 font-mono text-[10px] font-medium uppercase tracking-wider text-slate-600 dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface-variant"
        role="status"
        aria-live="polite"
      >
        <span className="h-2 w-2 shrink-0 animate-pulse rounded-full bg-slate-400 dark:bg-k3-outline" />
        Checking cluster
      </span>
    );
  }
  if (status === "ready") {
    return (
      <span
        className="inline-flex shrink-0 items-center rounded-full border border-emerald-300/70 bg-emerald-100 px-2.5 py-1 font-mono text-[10px] font-medium uppercase tracking-wider text-emerald-800 dark:border-emerald-500/30 dark:bg-emerald-500/15 dark:text-emerald-200"
        role="status"
        aria-live="polite"
      >
        Cluster ready
      </span>
    );
  }
  if (status === "resetting") {
    return (
      <span
        className="inline-flex shrink-0 items-center rounded-full border border-amber-300/70 bg-amber-100 px-2.5 py-1 font-mono text-[10px] font-medium uppercase tracking-wider text-amber-900 dark:border-amber-400/35 dark:bg-amber-500/15 dark:text-amber-100"
        role="status"
        aria-live="polite"
      >
        Cluster restarting
      </span>
    );
  }
  return (
    <span
      className="inline-flex shrink-0 items-center rounded-full border border-rose-300/70 bg-rose-100 px-2.5 py-1 font-mono text-[10px] font-medium uppercase tracking-wider text-rose-800 dark:border-rose-500/35 dark:bg-rose-500/15 dark:text-rose-200"
      role="status"
      aria-live="polite"
    >
      Cluster not ready
    </span>
  );
}

type AppProps = {
  theme: ThemeMode;
  setTheme: Dispatch<SetStateAction<ThemeMode>>;
};

export default function App({ theme, setTheme }: AppProps) {
  const [state, setState] = useState<WorkshopState | null>(null);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionErr, setActionErr] = useState<string | null>(null);
  const [answer, setAnswer] = useState("");
  const [verifyOutcome, setVerifyOutcome] = useState<VerifyOutcome>("idle");
  const [hadFailure, setHadFailure] = useState(false);
  const [incorrectPanelDismissed, setIncorrectPanelDismissed] = useState(false);
  const [correctPanelDismissed, setCorrectPanelDismissed] = useState(false);
  const [hintCount, setHintCount] = useState(0);
  const [labRestarting, setLabRestarting] = useState(false);
  const [labRestartFailed, setLabRestartFailed] = useState<string | null>(null);
  const [sidebarView, setSidebarView] = useState<"workshop" | string>("workshop");
  const [catalog, setCatalog] = useState<LabCatalog | null>(null);
  const [labSwitcherOpen, setLabSwitcherOpen] = useState(false);
  const [labSwitchMessage, setLabSwitchMessage] = useState("Restarting lab…");
  const [clusterStatus, setClusterStatus] = useState<LabStatus["cluster"] | null>(() => readStoredClusterStatus());
  const clusterReady = clusterStatus === "ready";
  const exposedEndpoints = useExposedEndpoints();
  const { detached, detach, dock, popupBlocked } = useTerminalDetach();
  const autoKey = useRef<string>("");
  const checkInFlight = useRef(false);
  const observePollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const chromeBtn =
    "flex h-7 w-7 shrink-0 items-center justify-center rounded-md border border-slate-400/30 bg-slate-200/80 text-slate-700 transition hover:border-k3-secondary/50 hover:bg-white dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface-variant dark:hover:border-k3-secondary/40 dark:hover:bg-k3-surface-container-high";

  const updateClusterStatus = useCallback((status: LabStatus["cluster"]) => {
    setClusterStatus(status);
    try {
      localStorage.setItem(CLUSTER_STATUS_STORAGE_KEY, status);
    } catch {
      // ignore storage failures
    }
  }, []);

  const refresh = useCallback(async () => {
    setVerifyOutcome("idle");
    try {
      const [cat, s, labStatus] = await Promise.all([
        getLabs(),
        getWorkshop(),
        getLabStatus().catch(() => null),
      ]);
      setCatalog(cat);
      setState(s);
      setLoadErr(null);
      if (labStatus) updateClusterStatus(labStatus.cluster);
      return { cat, s };
    } catch (e) {
      setLoadErr(String(e));
      return null;
    }
  }, [updateClusterStatus]);

  const resetWorkshopUI = useCallback(() => {
    setVerifyOutcome("idle");
    setHadFailure(false);
    setIncorrectPanelDismissed(false);
    setCorrectPanelDismissed(false);
    setHintCount(0);
    setAnswer("");
    autoKey.current = "";
    setSidebarView("workshop");
  }, []);

  const onSelectLab = useCallback(
    async (id: string, skipConfirm = false) => {
      const prev = catalog?.activeId;
      if (prev === id && state && !state.error) return;

      if (
        !skipConfirm &&
        prev &&
        prev !== id &&
        !window.confirm(
          "Switch to a different lab? The cluster will be fully reset and your progress in this lab will be lost. This may take up to a minute.",
        )
      ) {
        return;
      }

      setLabSwitchMessage(prev ? "Switching lab…" : "Loading lab…");
      setLabRestarting(true);
      setLabRestartFailed(null);
      setActionErr(null);
      try {
        const s = await selectLab(id);
        setState(s);
        resetWorkshopUI();
        try {
          localStorage.setItem(LAST_LAB_STORAGE_KEY, id);
        } catch {
          // ignore
        }
        const cat = await getLabs();
        setCatalog(cat);
        setLabRestarting(false);
      } catch (e) {
        const msg = String(e).includes("Lab restart failed")
          ? String(e)
          : String(e) || LAB_RESTART_FAILED_MSG;
        setLabRestartFailed(msg);
        setActionErr(msg);
      }
    },
    [catalog?.activeId, resetWorkshopUI, state],
  );

  const initialLoadDone = useRef(false);
  useEffect(() => {
    if (initialLoadDone.current) return;
    initialLoadDone.current = true;
    void (async () => {
      const r = await refresh();
      if (!r) return;
      const qLab = new URLSearchParams(window.location.search).get("lab")?.trim();
      if (qLab && r.cat.labs.some((l) => l.valid && l.id === qLab) && r.cat.activeId !== qLab) {
        await onSelectLab(qLab, true);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once on mount
  }, []);

  useEffect(() => {
    let closed = false;
    const pollStatus = async () => {
      try {
        const status = await getLabStatus();
        if (closed) return;
        updateClusterStatus(status.cluster);
        if (status.cluster === "resetting") {
          setLabRestarting(true);
          return;
        }
        if (status.cluster === "ready" && labRestarting && !labRestartFailed) {
          setLabRestarting(false);
          await refresh();
        }
      } catch {
        // Keep last known status on transient errors (avoid flashing "not ready" on refresh).
      }
    };
    void pollStatus();
    const id = window.setInterval(() => {
      void pollStatus();
    }, 2000);
    return () => {
      closed = true;
      window.clearInterval(id);
    };
  }, [labRestartFailed, labRestarting, refresh, updateClusterStatus]);

  useEffect(() => {
    if (labRestarting || !clusterReady || !state || state.error || state.done || !state.current) return;

    const key = `${state.currentStepIndex}:${state.current.id}:${state.current.type}`;
    const run = async () => {
      if (state.current!.type === "task") {
        if (autoKey.current === key) return;
        autoKey.current = key;
        setBusy(true);
        setActionErr(null);
        try {
          const r = await runTask();
          setState(r.state);
        } catch (e) {
          setActionErr(String(e));
          autoKey.current = "";
        } finally {
          setBusy(false);
        }
        return;
      }

      if (state.current!.type === "question" && !state.current!.setupDone) {
        if (autoKey.current === key + ":setup") return;
        autoKey.current = key + ":setup";
        setBusy(true);
        setActionErr(null);
        setHintCount(0);
        setAnswer("");
        try {
          const r = await runQuestionSetup();
          setState(r.state);
        } catch (e) {
          setActionErr(String(e));
          autoKey.current = "";
        } finally {
          setBusy(false);
        }
      }
    };

    void run();
  }, [clusterReady, labRestarting, state]);

  useEffect(() => {
    if (state?.current?.type === "question") {
      setHintCount(0);
      setAnswer("");
    }
    setHadFailure(false);
    setIncorrectPanelDismissed(false);
    setCorrectPanelDismissed(false);
  }, [state?.currentStepIndex, state?.current?.id]);

  useEffect(() => {
    if (verifyOutcome !== "success" && verifyOutcome !== "failure") return;
    const ms = verifyOutcome === "success" ? 4200 : 4500;
    const t = window.setTimeout(() => setVerifyOutcome("idle"), ms);
    return () => window.clearTimeout(t);
  }, [verifyOutcome]);

  const current = state?.current;
  const awaitingNext = current?.type === "question" && current.completed;

  const canSubmit = useMemo(() => {
    if (!current || current.type !== "question") return false;
    if (!clusterReady) return false;
    if (current.answer_type === "observe") return false;
    if (!current.setupDone || busy || awaitingNext) return false;
    if (current.answer_type === "text") return answer.trim().length > 0;
    return answer.length > 0;
  }, [answer, awaitingNext, busy, clusterReady, current]);

  const runObserveCheck = useCallback(async () => {
    if (!current || current.type !== "question" || current.answer_type !== "observe") return;
    if (!clusterReady || !current.setupDone || current.completed || checkInFlight.current || busy) return;
    checkInFlight.current = true;
    try {
      const r = await checkQuestion();
      setState(r.state);
      if (r.ok) {
        setVerifyOutcome("success");
        setHadFailure(false);
        setIncorrectPanelDismissed(false);
        setCorrectPanelDismissed(false);
        playVerifyTone(true);
      }
    } catch (e) {
      setActionErr(String(e));
    } finally {
      checkInFlight.current = false;
    }
  }, [busy, clusterReady, current]);

  useEffect(() => {
    if (observePollRef.current) {
      window.clearInterval(observePollRef.current);
      observePollRef.current = null;
    }
    if (
      labRestarting ||
      !clusterReady ||
      !current ||
      current.type !== "question" ||
      current.answer_type !== "observe" ||
      !current.setupDone ||
      current.completed
    ) {
      return;
    }
    const intervalSec = current.poll_interval_seconds ?? 5;
    void runObserveCheck();
    observePollRef.current = window.setInterval(() => {
      void runObserveCheck();
    }, intervalSec * 1000);
    return () => {
      if (observePollRef.current) {
        window.clearInterval(observePollRef.current);
        observePollRef.current = null;
      }
    };
  }, [
    current,
    clusterReady,
    labRestarting,
    runObserveCheck,
    state?.currentStepIndex,
  ]);

  const onSubmit = async () => {
    if (!current || current.type !== "question") return;
    if (!clusterReady) {
      setActionErr("Cluster is not ready yet. Wait until readiness turns green, then try again.");
      return;
    }
    setBusy(true);
    setActionErr(null);
    try {
      const r = await submitAnswer(
        current.answer_type === "text" ? answer.trim() : answer,
      );
      setState(r.state);
      if (r.ok) {
        setVerifyOutcome("success");
        setHadFailure(false);
        setIncorrectPanelDismissed(false);
        setCorrectPanelDismissed(false);
        playVerifyTone(true);
        setAnswer("");
      } else {
        setVerifyOutcome("failure");
        setHadFailure(true);
        setIncorrectPanelDismissed(false);
        playVerifyTone(false);
      }
    } catch (e) {
      setActionErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  const onNextQuestion = async () => {
    if (!clusterReady) {
      setActionErr("Cluster is not ready yet. Wait until readiness turns green, then continue.");
      return;
    }
    setBusy(true);
    setActionErr(null);
    try {
      const r = await advanceQuestion();
      setState(r.state);
      setVerifyOutcome("idle");
      setHadFailure(false);
      setIncorrectPanelDismissed(false);
      setCorrectPanelDismissed(false);
      setAnswer("");
      autoKey.current = "";
    } catch (e) {
      setActionErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  const onRestartLab = async () => {
    if (
      !window.confirm(
        "Restart the lab from the beginning? Workshop progress and all cluster state will be reset. This usually takes about a minute; kubectl may be unavailable briefly.",
      )
    ) {
      return;
    }
    setLabRestarting(true);
    setLabRestartFailed(null);
    setActionErr(null);
    try {
      const s = await restartLab();
      setState(s);
      setVerifyOutcome("idle");
      setHadFailure(false);
      setIncorrectPanelDismissed(false);
      setCorrectPanelDismissed(false);
      setHintCount(0);
      setAnswer("");
      autoKey.current = "";
      setSidebarView("workshop");
      const cat = await getLabs();
      setCatalog(cat);
      await waitForLabReady();
      setLabRestarting(false);
    } catch (e) {
      const msg = String(e).includes("Lab restart failed")
        ? String(e)
        : LAB_RESTART_FAILED_MSG;
      setLabRestartFailed(msg);
      setActionErr(msg);
    }
  };

  const dismissLabRestartOverlay = () => {
    setLabRestarting(false);
    setLabRestartFailed(null);
  };

  const showHints = current?.hints?.slice(0, hintCount) ?? [];
  const hasMoreHints = current?.hints && hintCount < current.hints.length;

  const wrongAnswerCopy = useMemo(() => {
    const custom = current?.incorrect_message?.trim();
    if (custom) return { markdown: true as const, text: custom };
    return { markdown: false as const, text: DEFAULT_WRONG_ANSWER_MSG };
  }, [current?.incorrect_message]);

  const correctMessageText = current?.correct_message?.trim() ?? "";
  const showIncorrectPanel = hadFailure && !incorrectPanelDismissed;
  const showCorrectPanel = awaitingNext && correctMessageText.length > 0 && !correctPanelDismissed;
  const answerDisabled = busy || awaitingNext || !clusterReady;

  const progressMeta = useMemo(() => {
    if (!state || state.error) {
      return { line1: "—", line2: "", pct: 0 };
    }
    if (state.done) {
      return { line1: "Complete", line2: "Workshop finished", pct: 100 };
    }
    const tq = state.totalQuestions ?? 0;
    const cq = state.currentQuestionNumber ?? 0;
    if (state.current?.type === "task") {
      return { line1: "Starting", line2: "Preparing workshop", pct: 5 };
    }
    if (tq <= 0) return { line1: "—", line2: state.name, pct: 0 };
    const pct = Math.min(100, Math.round((cq / tq) * 100));
    return { line1: `Question ${cq} / ${tq}`, line2: state.name, pct };
  }, [state]);

  const markdownTab = useMemo(() => {
    if (sidebarView === "workshop") return undefined;
    return state?.sidebarTabs?.find((t) => t.id === sidebarView);
  }, [state?.sidebarTabs, sidebarView]);

  useEffect(() => {
    if (sidebarView === "workshop") return;
    const tabs = state?.sidebarTabs ?? [];
    if (!tabs.some((t) => t.id === sidebarView)) {
      setSidebarView("workshop");
    }
  }, [state?.sidebarTabs, sidebarView]);

  const overlayLabel = useMemo(() => {
    if (!busy || !current) return null;
    if (current.type === "task") return "Preparing environment…";
    if (current.type === "question" && !current.setupDone) return "Setting up…";
    return null;
  }, [busy, current]);

  const typeBadge = current?.type === "task" ? "Setup" : current?.type === "question" ? "Task overview" : "";

  const shell = theme === "dark" ? "dark" : "";

  const pickerRequired = catalog != null && needsLabSelection(catalog, state);
  const lastLabHint =
    typeof localStorage !== "undefined"
      ? localStorage.getItem(LAST_LAB_STORAGE_KEY) ?? undefined
      : undefined;

  const navInactive =
    "rounded-lg px-3 py-2 text-slate-600 transition-colors hover:bg-slate-300/50 dark:text-k3-on-surface-variant dark:hover:bg-k3-surface-variant";

  const navActive =
    "flex w-full items-center gap-3 rounded-lg border border-teal-400/50 bg-teal-100/90 px-3 py-2 text-left font-medium text-teal-950 shadow-sm dark:border-transparent dark:bg-k3-secondary-container dark:font-normal dark:text-k3-on-secondary-container dark:shadow-none";

  return (
    <div
      className={`${shell} relative flex h-full min-h-0 flex-col bg-[#dfe3ea] text-slate-800 dark:bg-k3-background dark:text-k3-on-background`}
    >
      {labRestarting && (
        <div className="absolute inset-0 z-[100] flex flex-col items-center justify-center gap-4 bg-[#dfe3ea]/95 px-6 backdrop-blur-md dark:bg-k3-background/95">
          {labRestartFailed ? (
            <>
              <MIcon name="error" className="text-5xl text-rose-600 dark:text-k3-error" filled />
              <p className="max-w-md text-center text-sm font-medium text-slate-800 dark:text-k3-on-surface">
                {labRestartFailed}
              </p>
              <button
                type="button"
                className="rounded-lg border border-slate-400/35 bg-white px-4 py-2 font-sans text-sm font-semibold text-slate-800 shadow-sm hover:bg-slate-50 dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface dark:hover:bg-k3-surface-container-high"
                onClick={dismissLabRestartOverlay}
              >
                Dismiss
              </button>
            </>
          ) : (
            <>
              <span className="h-12 w-12 animate-spin rounded-full border-2 border-slate-200 border-t-k3-primary-container dark:border-k3-outline-variant dark:border-t-k3-secondary" />
              <p className="max-w-sm text-center text-sm font-medium text-slate-700 dark:text-k3-on-surface">
                {labSwitchMessage} This may take up to a minute. kubectl may be unavailable briefly.
              </p>
            </>
          )}
        </div>
      )}
      {/* Top app bar — Stitch / DESIGN.md */}
      <header className="z-50 flex h-16 shrink-0 items-center justify-between border-b border-slate-400/25 bg-[#eceef3]/95 px-6 backdrop-blur-sm dark:border-k3-outline-variant dark:bg-k3-surface dark:backdrop-blur-none">
        <div className="flex min-w-0 flex-1 items-center gap-8">
          <div className="flex min-w-0 flex-col gap-1">
            <span className="block truncate font-display text-xl font-extrabold tracking-tight text-slate-900 dark:text-k3-primary">
              {pickerRequired ? "K3sLab" : (state?.name ?? "K3sLab")}
            </span>
            {state?.labId && !pickerRequired ? (
              <span className="font-mono text-xs text-slate-500 dark:text-k3-on-surface-variant">
                lab: {state.labId}
              </span>
            ) : null}
          </div>
          {catalog ? (
            <LabSwitcher
              catalog={catalog}
              activeId={catalog.activeId || state?.labId}
              open={labSwitcherOpen}
              busy={busy || labRestarting}
              onToggle={() => setLabSwitcherOpen((o) => !o)}
              onClose={() => setLabSwitcherOpen(false)}
              onSelect={(id) => void onSelectLab(id)}
            />
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-2 sm:gap-3">
          {detached && (
            <button
              type="button"
              className="flex items-center gap-1.5 rounded-lg border border-slate-400/35 bg-white/80 px-2.5 py-1.5 font-mono text-xs font-medium text-slate-800 shadow-sm transition hover:border-k3-secondary/50 hover:bg-white dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface dark:hover:border-k3-secondary/40 dark:hover:bg-k3-surface-container-high"
              aria-label="Dock terminal back into lab"
              title="Dock terminal back into lab"
              onClick={dock}
            >
              <MIcon name="close_fullscreen" className="!text-base" />
              <span className="hidden sm:inline">Terminal</span>
            </button>
          )}
          {popupBlocked && !detached && (
            <span className="max-w-[12rem] text-xs text-rose-700 dark:text-k3-error" role="status">
              Pop-up blocked
            </span>
          )}
          <button
            type="button"
            className="rounded-lg p-2 text-slate-600 transition-colors hover:bg-slate-300/40 hover:text-slate-900 dark:text-k3-on-surface-variant dark:hover:bg-k3-surface-variant dark:hover:text-k3-primary"
            title={theme === "dark" ? "Light mode" : "Dark mode"}
            onClick={() => setTheme((t) => (t === "dark" ? "light" : "dark"))}
          >
            <MIcon name={theme === "dark" ? "light_mode" : "dark_mode"} className="text-[22px]" />
          </button>
          <button
            type="button"
            className="hidden rounded-lg border border-rose-800/25 bg-rose-600 px-3 py-1.5 font-sans text-xs font-semibold text-white shadow-sm hover:bg-rose-700 disabled:cursor-not-allowed disabled:opacity-60 sm:inline dark:border-rose-400/30 dark:bg-rose-700 dark:hover:bg-rose-600"
            disabled={labRestarting}
            onClick={() => void onRestartLab()}
          >
            Restart lab
          </button>
          <div className="flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-full border border-slate-400/40 bg-slate-300/50 text-xs font-bold text-slate-800 dark:border-k3-outline-variant dark:bg-k3-surface-container-highest dark:text-k3-on-background">
            {(state?.name ?? "K").slice(0, 1).toUpperCase()}
          </div>
        </div>
      </header>

      {pickerRequired && catalog ? (
        <LabPicker
          catalog={catalog}
          highlightId={lastLabHint}
          busy={labRestarting}
          onSelect={(id) => void onSelectLab(id, true)}
        />
      ) : (
      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* Side nav — contextual lab (Stitch) */}
        <aside className="hidden min-h-0 w-[280px] shrink-0 flex-col border-r border-slate-400/20 bg-[#d4d9e3]/90 py-4 pl-2 pr-2 dark:border-k3-outline-variant dark:bg-k3-surface-low md:flex">
          <div className="mb-6 px-3">
            <div className="mb-1 flex items-center gap-2">
              <div className="h-2 w-full overflow-hidden rounded-full bg-slate-400/35 dark:bg-k3-surface-variant">
                <div
                  className="h-full rounded-full bg-teal-600 dark:bg-k3-secondary transition-[width] duration-300"
                  style={{ width: `${progressMeta.pct}%` }}
                />
              </div>
            </div>
            <h2 className="font-display text-2xl font-bold tracking-tight text-blue-950 dark:text-k3-primary">
              {progressMeta.line1}
            </h2>
            {progressMeta.line2 && (
              <p className="mt-0.5 text-sm font-medium text-slate-700 dark:text-k3-on-surface-variant">{progressMeta.line2}</p>
            )}
          </div>
          <nav className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
            <button
              type="button"
              className={sidebarView === "workshop" ? navActive : `flex w-full items-center gap-3 text-left ${navInactive}`}
              onClick={() => setSidebarView("workshop")}
            >
              <MIcon name="quiz" />
              <span className="font-mono text-xs font-medium uppercase tracking-wider">Questions</span>
            </button>
            {(state?.sidebarTabs ?? []).map((t) => (
              <button
                key={t.id}
                type="button"
                className={sidebarView === t.id ? navActive : `flex w-full items-center gap-3 text-left ${navInactive}`}
                onClick={() => setSidebarView(t.id)}
              >
                <MIcon name={t.icon?.trim() || "article"} />
                <span className="font-mono text-xs font-medium tracking-wider">{t.title}</span>
              </button>
            ))}
          </nav>
          <div className="mt-auto flex shrink-0 flex-col gap-1 border-t border-slate-400/25 pt-4 dark:border-k3-outline-variant">
            <button
              type="button"
              className={`flex w-full items-center gap-3 ${navInactive}`}
              onClick={() => window.open("https://kubernetes.io/docs/", "_blank", "noopener,noreferrer")}
            >
              <MIcon name="menu_book" />
              <span className="font-mono text-xs font-medium tracking-wider">K8s docs</span>
            </button>
            <button
              type="button"
              className={`flex w-full items-center gap-3 ${navInactive}`}
              onClick={() => window.open("https://github.com/Exital/k3slab", "_blank", "noopener,noreferrer")}
            >
              <MIcon name="code" />
              <span className="font-mono text-xs font-medium tracking-wider">GitHub</span>
            </button>
          </div>
        </aside>

        {/* Instruction + terminal */}
        <PanelGroup
          direction="horizontal"
          className="min-h-0 min-w-0 flex-1"
          autoSaveId={detached ? undefined : "k3slab-main-split"}
        >
          <Panel
            id="instruction"
            order={1}
            defaultSize={detached ? 100 : 34}
            minSize={22}
            className="min-h-0 min-w-[240px]"
          >
            <section className="relative flex h-full min-h-0 flex-col overflow-hidden border-r border-slate-400/20 bg-[#eef1f6] dark:border-k3-outline-variant dark:bg-k3-surface">
              {sidebarView === "workshop" && overlayLabel && (
                <div className="absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 bg-[#eef1f6]/95 backdrop-blur-md dark:bg-k3-surface-lowest/90">
                  <span className="h-10 w-10 animate-spin rounded-full border-2 border-slate-200 border-t-k3-primary-container dark:border-k3-outline-variant dark:border-t-k3-secondary" />
                  <p className="text-sm font-medium text-slate-700 dark:text-k3-on-surface">{overlayLabel}</p>
                </div>
              )}

              {sidebarView === "workshop" && verifyOutcome === "success" && (
                <div className="k3-verify-enter flex shrink-0 items-center gap-3 border-b border-k3-secondary/20 bg-emerald-600/15 px-6 py-4 dark:bg-k3-secondary-container dark:text-k3-on-secondary-container">
                  <MIcon name="check_circle" className="text-2xl text-emerald-700 dark:text-k3-on-secondary-container" filled />
                  <span className="font-display text-lg font-semibold text-emerald-900 dark:text-k3-on-secondary-container">
                    Correct! Great job.
                  </span>
                </div>
              )}
              {sidebarView === "workshop" && verifyOutcome === "failure" && (
                <div className="k3-verify-enter flex shrink-0 items-center gap-3 border-b border-rose-500/30 bg-rose-50 px-6 py-4 dark:border-k3-error-container dark:bg-k3-error-container/40 dark:text-k3-on-error-container">
                  <MIcon name="cancel" className="text-2xl text-rose-700 dark:text-k3-error" filled />
                  <span className="font-display text-lg font-semibold text-rose-900 dark:text-k3-on-error-container">
                    Not quite — try again.
                  </span>
                </div>
              )}

              <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-8">
                {loadErr && (
                  <div className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-900 dark:border-k3-error-container dark:bg-k3-error-container/30 dark:text-k3-on-error-container">
                    Failed to load: {loadErr}
                  </div>
                )}
                {state?.error && (
                  <div className="rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-950 dark:border-amber-700/50 dark:bg-amber-950/40 dark:text-amber-100">
                    Workshop: {state.error}
                  </div>
                )}
                {actionErr && (
                  <div className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-900 dark:border-k3-error-container dark:bg-k3-error-container/30 dark:text-k3-on-error-container">
                    {actionErr}
                  </div>
                )}

                {sidebarView !== "workshop" && markdownTab ? (
                  <div className="space-y-4">
                    <p className="text-xs text-slate-500 dark:text-k3-on-surface-variant">
                      Reference — choose <span className="font-medium text-k3-secondary">Questions</span> in the left bar to return to the lab step.
                    </p>
                    <h1 className="font-display text-2xl font-semibold tracking-tight text-slate-900 dark:text-k3-on-background md:text-3xl">
                      {markdownTab.title}
                    </h1>
                    <MarkdownContent className="text-base leading-relaxed text-slate-700 dark:text-k3-on-surface-variant [&_code]:rounded [&_code]:bg-slate-100 [&_code]:px-1 [&_code]:font-mono [&_code]:text-sm dark:[&_code]:bg-k3-surface-container-highest dark:[&_code]:text-k3-primary [&_p]:mb-2 [&_ul]:mb-2 [&_li]:mb-0.5 [&_h2]:mt-4 [&_h2]:font-display [&_h2]:text-lg [&_h2]:font-semibold">
                      {markdownTab.content}
                    </MarkdownContent>
                  </div>
                ) : (
                  <>
                {state?.done && !state.error && (
                  <div className="text-base font-medium text-emerald-700 dark:text-k3-secondary">
                    You finished every question. Well done.
                  </div>
                )}

                {current && !state?.done && (
                  <>
                    <div className="space-y-3">
                      {typeBadge && (
                        <span className="font-mono text-xs font-medium uppercase tracking-widest text-teal-800 dark:text-k3-secondary">
                          {typeBadge}
                        </span>
                      )}
                      <h1 className="font-display text-2xl font-semibold tracking-tight text-slate-900 dark:text-k3-on-background md:text-3xl">
                        {current.title}
                      </h1>
                    </div>

                    {current.type === "question" && current.description && (
                      <MarkdownContent className="text-base leading-relaxed text-slate-600 dark:text-k3-on-surface-variant [&_code]:rounded [&_code]:bg-slate-100 [&_code]:px-1 [&_code]:font-mono [&_code]:text-sm dark:[&_code]:bg-k3-surface-container-highest dark:[&_code]:text-k3-primary">
                        {current.description}
                      </MarkdownContent>
                    )}

                    {showIncorrectPanel && (
                      <DismissibleMessagePanel
                        variant="error"
                        markdown={wrongAnswerCopy.markdown}
                        text={wrongAnswerCopy.text}
                        onDismiss={() => setIncorrectPanelDismissed(true)}
                      />
                    )}

                    {showCorrectPanel && (
                      <DismissibleMessagePanel
                        variant="success"
                        markdown
                        text={correctMessageText}
                        onDismiss={() => setCorrectPanelDismissed(true)}
                      />
                    )}

                    {current.type === "question" && current.setupDone && (
                      <div className="space-y-4">
                        {current.answer_type === "observe" && !awaitingNext && (
                          <div className="flex items-center gap-3 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700 dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface-variant">
                            <span className="h-4 w-4 shrink-0 animate-spin rounded-full border-2 border-slate-300 border-t-teal-600 dark:border-k3-outline-variant dark:border-t-k3-secondary" />
                            <span>Checking cluster state…</span>
                          </div>
                        )}

                        {current.answer_type === "text" && (
                          <div className="rounded-xl border-2 border-teal-500/45 bg-white p-4 shadow-sm dark:border-k3-secondary dark:bg-k3-surface-container dark:shadow-none">
                            <label className="mb-2 block font-sans text-sm font-semibold tracking-normal text-teal-900 subpixel-antialiased dark:text-k3-secondary">
                              Your answer
                            </label>
                            <input
                              className="w-full rounded-lg border-2 border-slate-300 bg-white px-3 py-2.5 font-mono text-sm text-slate-900 outline-none ring-0 placeholder:text-slate-400 focus:border-blue-600 focus:ring-2 focus:ring-blue-500/20 dark:border-k3-secondary dark:bg-k3-surface-lowest dark:text-k3-secondary dark:placeholder:text-k3-on-surface-variant dark:focus:border-k3-primary-container dark:focus:ring-cyan-500/20"
                              value={answer}
                              onChange={(e) => setAnswer(e.target.value)}
                              disabled={answerDisabled}
                              placeholder="Type your answer…"
                            />
                          </div>
                        )}

                        {current.answer_type === "single_choice" && (
                          <div className="space-y-3">
                            <div className="font-mono text-xs font-medium uppercase tracking-wider text-slate-600 dark:text-k3-on-surface-variant">
                              Choose one
                            </div>
                            {(current.options ?? []).map((opt) => (
                              <label
                                key={opt}
                                className="flex cursor-pointer items-center gap-3 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2.5 hover:border-k3-primary-container dark:border-k3-outline-variant dark:bg-k3-surface-lowest dark:hover:border-k3-secondary"
                              >
                                <input
                                  type="radio"
                                  name="choice"
                                  checked={answer === opt}
                                  onChange={() => setAnswer(opt)}
                                  disabled={answerDisabled}
                                />
                                <span className="font-mono text-sm text-slate-800 dark:text-k3-on-surface">{opt}</span>
                              </label>
                            ))}
                          </div>
                        )}

                        <div className="flex flex-wrap gap-2">
                          {awaitingNext ? (
                            <button
                              type="button"
                              disabled={busy}
                              className="flex w-full items-center justify-center gap-2 rounded-lg bg-k3-primary-container py-3.5 font-sans text-base font-semibold tracking-normal text-k3-on-primary-container subpixel-antialiased transition-all hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-40 dark:text-k3-on-primary-container"
                              onClick={() => void onNextQuestion()}
                            >
                              Next question
                              <MIcon name="arrow_forward" className="text-xl" />
                            </button>
                          ) : current.answer_type !== "observe" ? (
                            <button
                              type="button"
                              disabled={!canSubmit || busy}
                              className="flex w-full items-center justify-center gap-2 rounded-lg bg-k3-primary-container py-3.5 font-sans text-base font-semibold tracking-normal text-k3-on-primary-container subpixel-antialiased transition-all hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-40 dark:text-k3-on-primary-container"
                              onClick={() => void onSubmit()}
                            >
                              Submit answer
                              <MIcon name="arrow_forward" className="text-xl" />
                            </button>
                          ) : null}
                          {hasMoreHints && (
                            <button
                              type="button"
                              className="w-full rounded-lg border border-slate-400/35 bg-[#f4f6fa] py-2.5 font-sans text-sm font-medium tracking-normal text-slate-800 subpixel-antialiased hover:bg-slate-200/60 dark:border-k3-outline-variant dark:bg-k3-surface-container-high dark:text-k3-on-surface dark:hover:bg-k3-surface-variant"
                              onClick={() => setHintCount((c) => c + 1)}
                              disabled={answerDisabled}
                            >
                              Show next hint
                            </button>
                          )}
                        </div>

                        {showHints.length > 0 && (
                          <div className="rounded-xl border border-slate-200 bg-slate-50 p-3 text-sm dark:border-k3-outline-variant dark:bg-k3-surface-container">
                            <div className="mb-2 font-mono text-xs font-semibold uppercase tracking-wide text-slate-500 dark:text-k3-on-surface-variant">
                              Hints
                            </div>
                            <ul className="list-decimal space-y-1 pl-5 text-slate-800 dark:text-k3-on-surface-variant">
                              {showHints.map((h, i) => (
                                <li key={i}>{h}</li>
                              ))}
                            </ul>
                          </div>
                        )}
                      </div>
                    )}
                  </>
                )}
                  </>
                )}
              </div>
            </section>
          </Panel>

          {!detached && (
            <>
          <PanelResizeHandle className="group relative w-2 shrink-0 cursor-col-resize bg-slate-300/50 dark:bg-k3-surface-low">
            <span className="absolute inset-y-8 left-1/2 w-0.5 -translate-x-1/2 rounded-full bg-slate-500/50 opacity-0 transition-opacity group-hover:opacity-100 dark:bg-k3-outline" />
          </PanelResizeHandle>

          <Panel id="terminal" order={2} defaultSize={66} minSize={38} className="min-h-0 min-w-0">
            <section className="flex h-full min-h-0 flex-col overflow-hidden border-slate-400/15 bg-[#e4e7ee] dark:border-k3-outline-variant dark:bg-k3-surface-lowest k3-terminal-glow">
              <div className="flex h-12 shrink-0 items-center gap-2 overflow-hidden border-b border-slate-400/20 bg-[#d8dce5] px-3 dark:border-k3-outline-variant dark:bg-k3-surface-container-high">
                <div className="flex shrink-0 items-center gap-3">
                  <div className="flex gap-1">
                    <div className="h-3 w-3 rounded-full bg-red-500/50" />
                    <div className="h-3 w-3 rounded-full bg-amber-400/50" />
                    <div className="h-3 w-3 rounded-full bg-emerald-500/50" />
                  </div>
                  <div className="hidden h-6 w-px bg-slate-300 sm:block dark:bg-k3-outline-variant" />
                  <ClusterStatusBadge status={clusterStatus} />
                </div>
                <div className="flex min-w-0 flex-1 items-center justify-end gap-2 overflow-x-auto">
                  <button
                    type="button"
                    className={chromeBtn}
                    aria-label="Open terminal in new window"
                    title="Open terminal in new window"
                    onClick={detach}
                  >
                    <MIcon name="open_in_new" className="!text-lg" />
                  </button>
                  <span className="shrink-0 rounded-md border border-slate-400/30 bg-slate-200/80 px-2 py-1 font-mono text-xs font-medium text-slate-800 dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface">
                    Terminal
                  </span>
                  {exposedEndpoints.map((ep) => (
                    <button
                      key={ep.id}
                      type="button"
                      title={ep.url}
                      className="flex shrink-0 items-center gap-1 rounded-md border border-slate-400/25 bg-white/60 px-2 py-1 font-mono text-xs text-slate-700 transition hover:border-k3-secondary/50 hover:bg-white dark:border-k3-outline-variant dark:bg-k3-surface-low dark:text-k3-on-surface-variant dark:hover:border-k3-secondary/40 dark:hover:bg-k3-surface-container"
                      onClick={() => window.open(ep.url, "_blank", "noopener,noreferrer")}
                    >
                      <MIcon name="open_in_new" className="!text-sm" />
                      <span className="max-w-[10rem] truncate">{ep.label}</span>
                    </button>
                  ))}
                </div>
              </div>
              <div className="min-h-0 flex-1 overflow-hidden p-2 sm:p-3">
                <TerminalView theme={theme} mode="session" active />
              </div>
              <div className="flex h-8 shrink-0 items-center justify-between border-t border-slate-400/20 bg-[#dce0e8] px-3 font-mono text-xs text-slate-600 dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface-variant">
                <div className="flex items-center gap-3">
                  <span className="text-k3-secondary">UTF-8</span>
                  <span>Terminal</span>
                </div>
                <span className="text-slate-500 dark:text-k3-on-surface-variant">WebSocket</span>
              </div>
            </section>
          </Panel>
            </>
          )}
        </PanelGroup>
      </div>
      )}
    </div>
  );
}
