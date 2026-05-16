import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import {
  getWorkshop,
  restartWorkshop,
  runQuestionSetup,
  runTask,
  submitAnswer,
  terminalWsUrl,
  type WorkshopState,
} from "./api";
import { playVerifyTone } from "./playTone";

const THEME_KEY = "k3slab-theme";
export type ThemeMode = "light" | "dark";

/** Default copy when verify fails and the question has no `incorrect_message` in YAML. */
const DEFAULT_WRONG_ANSWER_MSG =
  "That doesn't match what we expected. Double-check your answer or the cluster state, then try again.";

function readStoredTheme(): ThemeMode {
  try {
    const v = localStorage.getItem(THEME_KEY);
    if (v === "light" || v === "dark") return v;
  } catch {
    /* ignore */
  }
  return "dark";
}

function MIcon({ name, className = "", filled }: { name: string; className?: string; filled?: boolean }) {
  return (
    <span className={`material-symbols-outlined ${filled ? "filled" : ""} ${className}`.trim()} aria-hidden>
      {name}
    </span>
  );
}

type TerminalPaneProps = { theme: ThemeMode };

function TerminalPane({ theme }: TerminalPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const dark = theme === "dark";
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "'JetBrains Mono', ui-monospace, monospace",
      fontSize: 16,
      lineHeight: 1.25,
      theme: dark
        ? {
            background: "#010f1f",
            foreground: "#d4e4fa",
            cursor: "#4edea3",
            selectionBackground: "#326ce540",
          }
        : {
            background: "#f8fafc",
            foreground: "#0f172a",
            cursor: "#0d9488",
            selectionBackground: "#94a3b840",
          },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(el);

    const ws = new WebSocket(terminalWsUrl());
    ws.binaryType = "arraybuffer";

    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") return;
      const buf = new Uint8Array(ev.data as ArrayBuffer);
      term.write(buf);
    };

    const sendResize = () => {
      if (!term.element) return;
      const dims = fit.proposeDimensions();
      if (!dims || ws.readyState !== WebSocket.OPEN) return;
      ws.send(JSON.stringify({ type: "resize", cols: dims.cols, rows: dims.rows }));
    };

    const disposable = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
    });

    ws.onopen = () => {
      fit.fit();
      sendResize();
    };

    const ro = new ResizeObserver(() => {
      fit.fit();
      sendResize();
    });
    ro.observe(el);

    const onWin = () => {
      fit.fit();
      sendResize();
    };
    window.addEventListener("resize", onWin);

    return () => {
      window.removeEventListener("resize", onWin);
      ro.disconnect();
      disposable.dispose();
      ws.close();
      term.dispose();
    };
  }, [theme]);

  const border =
    theme === "dark"
      ? "border-k3-outline-variant bg-k3-surface-lowest"
      : "border-slate-200 bg-slate-50";

  return <div ref={containerRef} className={`h-full min-h-[280px] w-full overflow-hidden p-0.5 ${border}`} />;
}

type VerifyOutcome = "idle" | "success" | "failure";

export default function App() {
  const [theme, setTheme] = useState<ThemeMode>(() => readStoredTheme());
  const [state, setState] = useState<WorkshopState | null>(null);
  const [loadErr, setLoadErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [actionErr, setActionErr] = useState<string | null>(null);
  const [answer, setAnswer] = useState("");
  const [verifyOutcome, setVerifyOutcome] = useState<VerifyOutcome>("idle");
  const [hintCount, setHintCount] = useState(0);

  useEffect(() => {
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch {
      /* ignore */
    }
  }, [theme]);

  const refresh = useCallback(async () => {
    setVerifyOutcome("idle");
    try {
      const s = await getWorkshop();
      setState(s);
      setLoadErr(null);
      return s;
    } catch (e) {
      setLoadErr(String(e));
      return null;
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const autoKey = useRef<string>("");
  useEffect(() => {
    if (!state || state.error || state.done || !state.current) return;

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
  }, [state]);

  useEffect(() => {
    if (state?.current?.type === "question") {
      setHintCount(0);
      setAnswer("");
    }
  }, [state?.currentStepIndex, state?.current?.id]);

  useEffect(() => {
    if (verifyOutcome !== "success" && verifyOutcome !== "failure") return;
    const ms = verifyOutcome === "success" ? 4200 : 4500;
    const t = window.setTimeout(() => setVerifyOutcome("idle"), ms);
    return () => window.clearTimeout(t);
  }, [verifyOutcome]);

  const current = state?.current;

  const canSubmit = useMemo(() => {
    if (!current || current.type !== "question") return false;
    if (!current.setupDone || busy) return false;
    if (current.answer_type === "text") return answer.trim().length > 0;
    return answer.length > 0;
  }, [answer, busy, current]);

  const onSubmit = async () => {
    if (!current || current.type !== "question") return;
    setBusy(true);
    setActionErr(null);
    try {
      const r = await submitAnswer(answer);
      setState(r.state);
      if (r.ok) {
        setVerifyOutcome("success");
        playVerifyTone(true);
        setAnswer("");
      } else {
        setVerifyOutcome("failure");
        playVerifyTone(false);
      }
    } catch (e) {
      setActionErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  const onRestart = async () => {
    if (!window.confirm("Restart the workshop from the beginning? Your progress will be reset.")) return;
    try {
      const s = await restartWorkshop();
      setState(s);
      setActionErr(null);
      setVerifyOutcome("idle");
      setHintCount(0);
      setAnswer("");
      autoKey.current = "";
    } catch (e) {
      setActionErr(String(e));
    }
  };

  const showHints = current?.hints?.slice(0, hintCount) ?? [];
  const hasMoreHints = current?.hints && hintCount < current.hints.length;

  const wrongAnswerCopy = useMemo(() => {
    const custom = current?.incorrect_message?.trim();
    if (custom) return { markdown: true as const, text: custom };
    return { markdown: false as const, text: DEFAULT_WRONG_ANSWER_MSG };
  }, [current?.incorrect_message]);

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

  const overlayLabel = useMemo(() => {
    if (!busy || !current) return null;
    if (current.type === "task") return "Preparing environment…";
    if (current.type === "question" && !current.setupDone) return "Setting up…";
    return null;
  }, [busy, current]);

  const typeBadge = current?.type === "task" ? "Setup" : current?.type === "question" ? "Task overview" : "";

  const shell = theme === "dark" ? "dark" : "";

  const navInactive =
    "rounded-lg px-3 py-2 text-slate-600 transition-colors hover:bg-slate-200/80 dark:text-k3-on-surface-variant dark:hover:bg-k3-surface-variant";

  return (
    <div
      className={`${shell} flex h-full min-h-0 flex-col bg-slate-50 text-slate-900 dark:bg-k3-background dark:text-k3-on-background`}
    >
      {/* Top app bar — Stitch / DESIGN.md */}
      <header className="z-50 flex h-16 shrink-0 items-center justify-between border-b border-slate-200 bg-white px-5 dark:border-k3-outline-variant dark:bg-k3-surface">
        <div className="flex min-w-0 items-center gap-6">
          <span className="truncate font-display text-xl font-extrabold tracking-tight text-k3-primary dark:text-k3-primary">
            {state?.name ?? "K3sLab"}
          </span>
          <nav className="hidden items-center md:flex">
            <span className="border-b-2 border-k3-primary pb-1 font-mono text-xs font-medium uppercase tracking-wider text-k3-primary dark:text-k3-primary">
              Labs
            </span>
          </nav>
        </div>
        <div className="flex shrink-0 items-center gap-2 sm:gap-3">
          <button
            type="button"
            className="rounded-lg p-2 text-slate-500 transition-colors hover:bg-slate-100 hover:text-k3-primary-container dark:text-k3-on-surface-variant dark:hover:bg-k3-surface-variant dark:hover:text-k3-primary"
            title={theme === "dark" ? "Light mode" : "Dark mode"}
            onClick={() => setTheme((t) => (t === "dark" ? "light" : "dark"))}
          >
            <MIcon name={theme === "dark" ? "light_mode" : "dark_mode"} className="text-[22px]" />
          </button>
          <button
            type="button"
            className="hidden rounded-lg border border-rose-800/25 bg-rose-600 px-3 py-1.5 font-sans text-xs font-semibold text-white shadow-sm hover:bg-rose-700 sm:inline dark:border-rose-400/30 dark:bg-rose-700 dark:hover:bg-rose-600"
            onClick={() => void onRestart()}
          >
            Restart
          </button>
          <div className="flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-full border border-slate-200 bg-slate-200 text-xs font-bold text-slate-700 dark:border-k3-outline-variant dark:bg-k3-surface-container-highest dark:text-k3-on-background">
            {(state?.name ?? "K").slice(0, 1).toUpperCase()}
          </div>
        </div>
      </header>

      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* Side nav — contextual lab (Stitch) */}
        <aside className="hidden w-[280px] shrink-0 flex-col border-r border-slate-200 bg-slate-100 py-4 pl-2 pr-2 dark:border-k3-outline-variant dark:bg-k3-surface-low md:flex">
          <div className="mb-6 px-3">
            <div className="mb-1 flex items-center gap-2">
              <div className="h-2 w-full overflow-hidden rounded-full bg-slate-300 dark:bg-k3-surface-variant">
                <div
                  className="h-full rounded-full bg-k3-secondary transition-[width] duration-300"
                  style={{ width: `${progressMeta.pct}%` }}
                />
              </div>
            </div>
            <h2 className="font-display text-2xl font-bold tracking-tight text-k3-primary dark:text-k3-primary">
              {progressMeta.line1}
            </h2>
            {progressMeta.line2 && (
              <p className="mt-0.5 text-sm text-slate-600 dark:text-k3-on-surface-variant">{progressMeta.line2}</p>
            )}
          </div>
          <nav className="flex flex-1 flex-col gap-1">
            <div className="flex items-center gap-3 rounded-lg bg-k3-secondary-container px-3 py-2 text-k3-on-secondary-container dark:bg-k3-secondary-container dark:text-k3-on-secondary-container">
              <MIcon name="quiz" />
              <span className="font-mono text-xs font-medium uppercase tracking-wider">Questions</span>
            </div>
            <button type="button" className={`flex w-full items-center gap-3 ${navInactive}`}>
              <MIcon name="description" />
              <span className="font-mono text-xs font-medium uppercase tracking-wider">Cheat sheet</span>
            </button>
            <button type="button" className={`flex w-full items-center gap-3 ${navInactive}`}>
              <MIcon name="info" />
              <span className="font-mono text-xs font-medium uppercase tracking-wider">Lab info</span>
            </button>
          </nav>
          <div className="mt-auto flex flex-col gap-1 border-t border-slate-200 pt-4 dark:border-k3-outline-variant">
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
        <PanelGroup direction="horizontal" className="min-h-0 min-w-0 flex-1">
          <Panel defaultSize={34} minSize={22} className="min-h-0 min-w-[240px]">
            <section className="relative flex h-full min-h-0 flex-col overflow-hidden border-r border-slate-200 bg-white dark:border-k3-outline-variant dark:bg-k3-surface">
              {overlayLabel && (
                <div className="absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 bg-white/90 backdrop-blur-md dark:bg-k3-surface-lowest/90">
                  <span className="h-10 w-10 animate-spin rounded-full border-2 border-slate-200 border-t-k3-primary-container dark:border-k3-outline-variant dark:border-t-k3-secondary" />
                  <p className="text-sm font-medium text-slate-700 dark:text-k3-on-surface">{overlayLabel}</p>
                </div>
              )}

              {verifyOutcome === "success" && (
                <div className="k3-verify-enter flex shrink-0 items-center gap-3 border-b border-k3-secondary/20 bg-emerald-600/15 px-6 py-4 dark:bg-k3-secondary-container dark:text-k3-on-secondary-container">
                  <MIcon name="check_circle" className="text-2xl text-emerald-700 dark:text-k3-on-secondary-container" filled />
                  <span className="font-display text-lg font-semibold text-emerald-900 dark:text-k3-on-secondary-container">
                    Correct! Great job.
                  </span>
                </div>
              )}
              {verifyOutcome === "failure" && (
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

                {state?.done && !state.error && (
                  <div className="text-base font-medium text-emerald-700 dark:text-k3-secondary">
                    You finished every question. Well done.
                  </div>
                )}

                {current && !state?.done && (
                  <>
                    <div className="space-y-3">
                      {typeBadge && (
                        <span className="font-mono text-xs font-medium uppercase tracking-widest text-k3-secondary">
                          {typeBadge}
                        </span>
                      )}
                      <h1 className="font-display text-2xl font-semibold tracking-tight text-slate-900 dark:text-k3-on-background md:text-3xl">
                        {current.title}
                      </h1>
                    </div>

                    {current.type === "question" && current.description && (
                      <div className="text-base leading-relaxed text-slate-600 dark:text-k3-on-surface-variant [&_code]:rounded [&_code]:bg-slate-100 [&_code]:px-1 [&_code]:font-mono [&_code]:text-sm dark:[&_code]:bg-k3-surface-container-highest dark:[&_code]:text-k3-primary">
                        <ReactMarkdown>{current.description}</ReactMarkdown>
                      </div>
                    )}

                    {verifyOutcome === "failure" && (
                      <div className="rounded-xl border border-rose-200 bg-rose-50/90 p-4 text-sm dark:border-k3-error-container/50 dark:bg-k3-surface-container">
                        {wrongAnswerCopy.markdown ? (
                          <div className="text-rose-900 dark:text-k3-on-error-container [&_code]:rounded [&_code]:bg-rose-100 [&_code]:px-1.5 dark:[&_code]:bg-k3-surface-lowest [&_p]:mb-2 [&_p:last-child]:mb-0">
                            <ReactMarkdown>{wrongAnswerCopy.text}</ReactMarkdown>
                          </div>
                        ) : (
                          <p className="text-rose-900 dark:text-k3-on-error-container">{wrongAnswerCopy.text}</p>
                        )}
                      </div>
                    )}

                    {current.type === "question" && current.setupDone && (
                      <div className="space-y-4">
                        {current.answer_type === "text" && (
                          <div className="rounded-xl border-2 border-k3-secondary bg-k3-surface-container p-4 dark:border-k3-secondary dark:bg-k3-surface-container">
                            <label className="mb-2 block font-sans text-sm font-semibold tracking-normal text-k3-secondary subpixel-antialiased dark:text-k3-secondary">
                              Your answer
                            </label>
                            <input
                              className="w-full rounded-lg border-2 border-k3-secondary bg-k3-surface-lowest px-3 py-2.5 font-mono text-sm text-k3-secondary outline-none ring-0 placeholder:text-k3-on-surface-variant focus:border-k3-primary-container dark:bg-k3-surface-lowest dark:text-k3-secondary"
                              value={answer}
                              onChange={(e) => setAnswer(e.target.value)}
                              disabled={busy}
                              placeholder="Type your answer…"
                            />
                          </div>
                        )}

                        {current.answer_type === "single_choice" && (
                          <div className="space-y-3">
                            <div className="font-mono text-xs font-medium uppercase tracking-wider text-k3-on-surface-variant">
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
                                  disabled={busy}
                                />
                                <span className="font-mono text-sm text-slate-800 dark:text-k3-on-surface">{opt}</span>
                              </label>
                            ))}
                          </div>
                        )}

                        <div className="flex flex-wrap gap-2">
                          <button
                            type="button"
                            disabled={!canSubmit || busy}
                            className="flex w-full items-center justify-center gap-2 rounded-lg bg-k3-primary-container py-3.5 font-sans text-base font-semibold tracking-normal text-k3-on-primary-container subpixel-antialiased transition-all hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-40 dark:text-k3-on-primary-container"
                            onClick={() => void onSubmit()}
                          >
                            Submit answer
                            <MIcon name="arrow_forward" className="text-xl" />
                          </button>
                          {hasMoreHints && (
                            <button
                              type="button"
                              className="w-full rounded-lg border border-slate-300 bg-white py-2.5 font-sans text-sm font-medium tracking-normal text-slate-800 subpixel-antialiased hover:bg-slate-50 dark:border-k3-outline-variant dark:bg-k3-surface-container-high dark:text-k3-on-surface dark:hover:bg-k3-surface-variant"
                              onClick={() => setHintCount((c) => c + 1)}
                              disabled={busy}
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
              </div>
            </section>
          </Panel>

          <PanelResizeHandle className="group relative w-2 shrink-0 cursor-col-resize bg-slate-200 dark:bg-k3-surface-low">
            <span className="absolute inset-y-8 left-1/2 w-0.5 -translate-x-1/2 rounded-full bg-slate-400 opacity-0 transition-opacity group-hover:opacity-100 dark:bg-k3-outline" />
          </PanelResizeHandle>

          <Panel defaultSize={66} minSize={38} className="min-h-0 min-w-0">
            <section className="flex h-full min-h-0 flex-col overflow-hidden border-slate-200 bg-slate-100 dark:border-k3-outline-variant dark:bg-k3-surface-lowest k3-terminal-glow">
              <div className="flex h-12 shrink-0 items-center border-b border-slate-200 bg-slate-200/80 px-3 dark:border-k3-outline-variant dark:bg-k3-surface-container-high">
                <div className="flex min-w-0 items-center gap-3">
                  <div className="flex gap-1">
                    <div className="h-3 w-3 rounded-full bg-red-500/50" />
                    <div className="h-3 w-3 rounded-full bg-amber-400/50" />
                    <div className="h-3 w-3 rounded-full bg-emerald-500/50" />
                  </div>
                  <div className="hidden h-6 w-px bg-slate-300 sm:block dark:bg-k3-outline-variant" />
                  <span className="hidden items-center gap-2 truncate font-mono text-xs text-slate-600 sm:flex dark:text-k3-on-surface-variant">
                    <span className="h-2 w-2 shrink-0 animate-pulse rounded-full bg-k3-secondary" />
                    connected: lab shell
                  </span>
                </div>
              </div>
              <div className="min-h-0 flex-1 overflow-hidden p-2 sm:p-3">
                <TerminalPane theme={theme} />
              </div>
              <div className="flex h-8 shrink-0 items-center justify-between border-t border-slate-200 bg-slate-100 px-3 font-mono text-xs text-slate-500 dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface-variant">
                <div className="flex items-center gap-3">
                  <span className="text-k3-secondary">UTF-8</span>
                  <span>Terminal</span>
                </div>
                <span className="text-slate-500 dark:text-k3-on-surface-variant">WebSocket</span>
              </div>
            </section>
          </Panel>
        </PanelGroup>
      </div>
    </div>
  );
}
