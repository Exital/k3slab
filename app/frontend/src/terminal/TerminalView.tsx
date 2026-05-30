import { forwardRef, useEffect, useRef, useState } from "react";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import type { ThemeMode } from "../theme";
import {
  createTerminalChannel,
  postTerminalMessage,
  type TerminalChannelMessage,
  type TerminalStatus,
} from "./channel";
import { useTerminalSession } from "./session";

export type TerminalViewMode = "session" | "relay";

type TerminalViewProps = {
  theme: ThemeMode;
  mode: TerminalViewMode;
  /** When false, skip PTY resize (e.g. lab placeholder while detached). */
  active?: boolean;
};

function terminalTheme(dark: boolean) {
  return dark
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
      };
}

export function TerminalView({ theme, mode, active = true }: TerminalViewProps) {
  if (mode === "relay") {
    return <TerminalViewRelay theme={theme} active={active} />;
  }
  return <TerminalViewSession theme={theme} active={active} />;
}

function TerminalViewSession({ theme, active }: { theme: ThemeMode; active: boolean }) {
  const session = useTerminalSession();
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
      theme: terminalTheme(dark),
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(el);

    const replay = session.getReplayBuffer();
    if (replay.length > 0) term.write(replay);

    const sendResize = () => {
      if (!active) return;
      const dims = fit.proposeDimensions();
      if (!dims || dims.cols < 1 || dims.rows < 1) return;
      session.sendResize(dims.cols, dims.rows);
    };

    const unsub = session.subscribeOutput((data) => term.write(data));
    const onData = term.onData((data) => session.sendInput(data));

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

    requestAnimationFrame(() => {
      fit.fit();
      sendResize();
    });

    return () => {
      window.removeEventListener("resize", onWin);
      ro.disconnect();
      onData.dispose();
      unsub();
      term.dispose();
    };
  }, [theme, active, session, session.connectionGeneration]);

  return <TerminalViewContainer ref={containerRef} theme={theme} />;
}

function TerminalViewRelay({ theme, active }: { theme: ThemeMode; active: boolean }) {
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
      theme: terminalTheme(dark),
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(el);

    const sendResize = () => {
      if (!active) return;
      const dims = fit.proposeDimensions();
      if (!dims || dims.cols < 1 || dims.rows < 1) return;
      postTerminalMessage({ type: "resize", cols: dims.cols, rows: dims.rows });
    };

    const onData = term.onData((data) => postTerminalMessage({ type: "input", data }));

    const ch = createTerminalChannel((msg: TerminalChannelMessage) => {
      switch (msg.type) {
        case "output":
        case "replay":
          term.write(msg.data);
          break;
        default:
          break;
      }
    });

    postTerminalMessage({ type: "ready" });

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

    requestAnimationFrame(() => {
      fit.fit();
      sendResize();
    });

    return () => {
      window.removeEventListener("resize", onWin);
      ro.disconnect();
      onData.dispose();
      ch.close();
      term.dispose();
    };
  }, [theme, active]);

  return <TerminalViewContainer ref={containerRef} theme={theme} />;
}

const TerminalViewContainer = forwardRef<HTMLDivElement, { theme: ThemeMode }>(function TerminalViewContainer(
  { theme },
  ref,
) {
  const border =
    theme === "dark"
      ? "border-k3-outline-variant bg-k3-surface-lowest"
      : "border-slate-200 bg-slate-50";

  return <div ref={ref} className={`h-full min-h-[280px] w-full overflow-hidden p-0.5 ${border}`} />;
});

/** Relay-only connection status for pop-out chrome. */
export function useRelayTerminalStatus(): TerminalStatus {
  const [status, setStatus] = useState<TerminalStatus>("connecting");

  useEffect(() => {
    const ch = createTerminalChannel((msg) => {
      if (msg.type === "status") setStatus(msg.status);
    });
    return () => ch.close();
  }, []);

  return status;
}
