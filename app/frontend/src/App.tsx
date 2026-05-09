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

type TerminalPaneProps = { theme: ThemeMode };

function TerminalPane({ theme }: TerminalPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const dark = theme === "dark";
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
      fontSize: 14,
      theme: dark
        ? {
            background: "#0c1222",
            foreground: "#cbd5e1",
            cursor: "#38bdf8",
            selectionBackground: "#33415580",
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
      ? "border-slate-700/80 bg-[#0c1222] shadow-inner shadow-black/20"
      : "border-slate-200 bg-slate-50 shadow-sm";

  return <div ref={containerRef} className={`h-full min-h-[240px] w-full overflow-hidden rounded-lg border p-1 ${border}`} />;
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
        // Keep verifyOutcome (e.g. success) until its own timer clears — do not hide celebration when setup runs.
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
    return () => clearTimeout(t);
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

  const progressLine = useMemo(() => {
    if (!state || state.error) return null;
    if (state.done) {
      return <span className="font-medium text-emerald-600 dark:text-emerald-400">Workshop complete</span>;
    }
    const tq = state.totalQuestions ?? 0;
    const cq = state.currentQuestionNumber ?? 0;
    if (state.current?.type === "task") {
      return <span className="text-slate-500 dark:text-slate-400">Preparing workshop…</span>;
    }
    if (tq <= 0) return <span className="text-slate-500 dark:text-slate-400">—</span>;
    return (
      <span className="text-slate-600 dark:text-slate-300">
        Question {cq} / {tq}
      </span>
    );
  }, [state]);

  const overlayLabel = useMemo(() => {
    if (!busy || !current) return null;
    if (current.type === "task") return "Preparing environment…";
    if (current.type === "question" && !current.setupDone) return "Setting up…";
    return null;
  }, [busy, current]);

  const typeBadge = current?.type === "task" ? "Setup" : current?.type === "question" ? "Question" : "";

  const shell = theme === "dark" ? "dark" : "";

  return (
    <div
      className={`${shell} flex h-full min-h-0 flex-col bg-slate-50 text-slate-900 dark:bg-gradient-to-b dark:from-[#070b14] dark:via-[#0a1020] dark:to-[#060912] dark:text-slate-100`}
    >
      <header className="flex shrink-0 items-center justify-between gap-2 border-b border-slate-200/90 bg-white/90 px-4 py-2.5 backdrop-blur-sm dark:border-slate-700/50 dark:bg-slate-900/75 dark:shadow-[inset_0_-1px_0_0_rgba(56,189,248,0.08)]">
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold tracking-tight text-slate-900 dark:bg-gradient-to-r dark:from-slate-50 dark:to-cyan-100 dark:bg-clip-text dark:text-transparent">
            {state?.name ?? "K3sLab"}
          </div>
          <div className="truncate text-xs">{progressLine}</div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
          <button
            type="button"
            className="rounded-lg border border-slate-300 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-800 shadow-sm hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-800/90 dark:text-slate-100 dark:hover:bg-slate-700/90"
            onClick={() => setTheme((t) => (t === "dark" ? "light" : "dark"))}
            title={theme === "dark" ? "Light mode" : "Dark mode"}
          >
            {theme === "dark" ? "Light" : "Dark"}
          </button>
          <button
            type="button"
            className="rounded-lg border border-slate-300 bg-white px-2.5 py-1.5 text-xs font-medium text-slate-800 shadow-sm hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-800/90 dark:text-slate-100 dark:hover:bg-slate-700/90"
            onClick={() => void refresh()}
          >
            Refresh
          </button>
          <button
            type="button"
            className="rounded-lg border border-amber-400/70 bg-amber-50 px-2.5 py-1.5 text-xs font-medium text-amber-950 shadow-sm hover:bg-amber-100 dark:border-amber-500/40 dark:bg-amber-950/45 dark:text-amber-100 dark:hover:bg-amber-900/55"
            onClick={() => void onRestart()}
          >
            Restart workshop
          </button>
        </div>
      </header>

      <main className="min-h-0 flex-1 p-2.5">
        <PanelGroup direction="horizontal" className="h-full">
          <Panel defaultSize={32} minSize={22} className="min-h-0 min-w-[280px]">
            <aside className="relative flex h-full min-h-0 flex-col gap-3 overflow-auto rounded-xl border border-slate-200/90 bg-white/95 p-3.5 shadow-sm dark:border-slate-700/60 dark:bg-slate-900/55 dark:shadow-[0_0_0_1px_rgba(15,23,42,0.5),inset_0_1px_0_0_rgba(148,163,184,0.06)]">
              {overlayLabel && (
                <div className="absolute inset-0 z-20 flex flex-col items-center justify-center gap-3 rounded-xl bg-white/85 backdrop-blur-md dark:bg-slate-950/80 dark:backdrop-blur-md">
                  <span className="h-10 w-10 animate-spin rounded-full border-2 border-slate-200 border-t-teal-600 dark:border-slate-600 dark:border-t-cyan-400" />
                  <p className="text-sm font-medium text-slate-700 dark:text-slate-200">{overlayLabel}</p>
                </div>
              )}

              {verifyOutcome !== "idle" && (
                <div
                  className={`relative z-[35] rounded-xl border px-4 py-3.5 ${
                    verifyOutcome === "success"
                      ? "k3-verify-enter k3-success-flash border-emerald-400/95 bg-emerald-50 text-emerald-950 shadow-none ring-0 dark:border-emerald-400/50 dark:bg-emerald-950/55 dark:text-emerald-50 dark:shadow-none"
                      : "k3-verify-enter border-rose-300/90 bg-rose-50 text-rose-950 shadow-lg dark:border-rose-500/35 dark:bg-rose-950/40 dark:text-rose-50 dark:shadow-lg dark:shadow-rose-950/30"
                  }`}
                >
                  <div className="flex items-start gap-3">
                    <span className="text-2xl leading-none">{verifyOutcome === "success" ? "✓" : "✕"}</span>
                    <div className="min-w-0 flex-1">
                      <div className="text-sm font-semibold">{verifyOutcome === "success" ? "Correct" : "Not quite"}</div>
                      {verifyOutcome === "success" ? (
                        <p className="mt-1 text-xs leading-relaxed opacity-90">Nice work — on to the next question.</p>
                      ) : wrongAnswerCopy.markdown ? (
                        <div className="mt-2 text-xs leading-relaxed text-rose-900 dark:text-rose-100 [&_code]:rounded-md [&_code]:bg-rose-100 [&_code]:px-1.5 dark:[&_code]:bg-rose-950/80 [&_p]:mb-2 [&_p:last-child]:mb-0 [&_strong]:font-semibold">
                          <ReactMarkdown>{wrongAnswerCopy.text}</ReactMarkdown>
                        </div>
                      ) : (
                        <p className="mt-1 text-xs leading-relaxed opacity-90">{wrongAnswerCopy.text}</p>
                      )}
                    </div>
                  </div>
                </div>
              )}

              {loadErr && (
                <div className="rounded-lg border border-rose-200 bg-rose-50 p-2 text-sm text-rose-900 dark:border-rose-900/60 dark:bg-rose-950/35 dark:text-rose-100">
                  Failed to load: {loadErr}
                </div>
              )}
              {state?.error && (
                <div className="rounded-lg border border-amber-200 bg-amber-50 p-2 text-sm text-amber-950 dark:border-amber-800/50 dark:bg-amber-950/35 dark:text-amber-100">
                  Workshop: {state.error}
                </div>
              )}
              {actionErr && (
                <div className="rounded-lg border border-rose-200 bg-rose-50 p-2 text-sm text-rose-900 dark:border-rose-900/60 dark:bg-rose-950/35 dark:text-rose-100">
                  {actionErr}
                </div>
              )}

              {state?.done && !state.error && (
                <div className="text-sm font-medium text-emerald-700 dark:text-emerald-400">You finished every question. Well done.</div>
              )}

              {current && !state?.done && (
                <>
                  <div>
                    {typeBadge && (
                      <div className="text-[11px] font-semibold uppercase tracking-wider text-teal-700 dark:text-cyan-400/90">
                        {typeBadge}
                      </div>
                    )}
                    <h2 className="text-lg font-semibold tracking-tight text-slate-900 dark:text-slate-50">{current.title}</h2>
                  </div>

                  {current.type === "question" && current.description && (
                    <div className="text-sm leading-relaxed text-slate-800 dark:text-slate-200 [&_code]:rounded-md [&_code]:bg-slate-100 [&_code]:px-1.5 [&_code]:py-0.5 dark:[&_code]:bg-slate-950/80 dark:[&_code]:text-cyan-100">
                      <ReactMarkdown>{current.description}</ReactMarkdown>
                    </div>
                  )}

                  {current.type === "question" && current.setupDone && (
                    <div className="space-y-2">
                      {current.answer_type === "text" && (
                        <label className="block text-sm font-medium text-slate-700 dark:text-slate-300">
                          Your answer
                          <input
                            className="mt-1.5 w-full rounded-lg border border-slate-300 bg-white px-2.5 py-2 text-sm text-slate-900 shadow-inner outline-none ring-teal-500/0 transition-shadow focus:border-teal-500/50 focus:ring-2 focus:ring-teal-500/25 dark:border-slate-600 dark:bg-slate-950/80 dark:text-slate-100 dark:focus:border-cyan-500/40 dark:focus:ring-cyan-500/20"
                            value={answer}
                            onChange={(e) => setAnswer(e.target.value)}
                            disabled={busy}
                          />
                        </label>
                      )}

                      {current.answer_type === "single_choice" && (
                        <div className="space-y-2 text-sm">
                          <div className="font-medium text-slate-700 dark:text-slate-300">Choose one</div>
                          {(current.options ?? []).map((opt) => (
                            <label
                              key={opt}
                              className="flex cursor-pointer items-center gap-2 rounded-lg border border-slate-200 bg-slate-50/80 px-2.5 py-2 hover:border-teal-400/60 dark:border-slate-700 dark:bg-slate-950/50 dark:hover:border-cyan-500/40"
                            >
                              <input type="radio" name="choice" checked={answer === opt} onChange={() => setAnswer(opt)} disabled={busy} />
                              <span className="font-mono text-xs text-slate-800 dark:text-slate-200">{opt}</span>
                            </label>
                          ))}
                        </div>
                      )}

                      <div className="flex flex-wrap gap-2">
                        <button
                          type="button"
                          disabled={!canSubmit || busy}
                          className="rounded-lg bg-teal-600 px-3.5 py-2 text-sm font-semibold text-white shadow-md shadow-teal-600/25 hover:bg-teal-500 disabled:cursor-not-allowed disabled:opacity-40 dark:bg-emerald-600 dark:shadow-emerald-900/40 dark:hover:bg-emerald-500"
                          onClick={() => void onSubmit()}
                        >
                          Submit
                        </button>
                        {hasMoreHints && (
                          <button
                            type="button"
                            className="rounded-lg border border-slate-300 bg-white px-3.5 py-2 text-sm font-medium text-slate-800 shadow-sm hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-800/90 dark:text-slate-100 dark:hover:bg-slate-700/90"
                            onClick={() => setHintCount((c) => c + 1)}
                            disabled={busy}
                          >
                            Show next hint
                          </button>
                        )}
                      </div>

                      {showHints.length > 0 && (
                        <div className="rounded-lg border border-slate-200 bg-slate-50/90 p-2.5 text-sm text-slate-800 dark:border-slate-700 dark:bg-slate-950/50 dark:text-slate-300">
                          <div className="mb-1 text-[11px] font-semibold uppercase tracking-wide text-slate-500 dark:text-slate-500">Hints</div>
                          <ul className="list-decimal space-y-1 pl-4">
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
            </aside>
          </Panel>

          <PanelResizeHandle className="mx-1 w-1 cursor-col-resize rounded-full bg-slate-200 hover:bg-teal-400/50 dark:bg-slate-700 dark:hover:bg-cyan-500/40" />

          <Panel defaultSize={68} minSize={40}>
            <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-xl border border-slate-200/90 bg-white/80 shadow-sm dark:border-slate-700/60 dark:bg-slate-900/40 dark:shadow-[inset_0_1px_0_0_rgba(148,163,184,0.05)]">
              <div className="border-b border-slate-200/90 px-3 py-2 text-[11px] font-semibold uppercase tracking-wider text-slate-500 dark:border-slate-700/60 dark:text-slate-400">
                Terminal
              </div>
              <div className="min-h-0 flex-1 p-2">
                <TerminalPane theme={theme} />
              </div>
            </section>
          </Panel>
        </PanelGroup>
      </main>
    </div>
  );
}
