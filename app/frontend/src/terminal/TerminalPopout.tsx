import { useEffect, useState } from "react";
import { MIcon } from "../components/MIcon";
import { readStoredTheme, THEME_KEY, type ThemeMode } from "../theme";
import { createTerminalChannel, postTerminalMessage } from "./channel";
import { TerminalView, useRelayTerminalStatus } from "./TerminalView";

export function TerminalPopout() {
  const [theme, setTheme] = useState<ThemeMode>(() => readStoredTheme());
  const status = useRelayTerminalStatus();

  useEffect(() => {
    document.title = "K3sLab — Terminal";
    document.documentElement.classList.add("k3-terminal-popout");
    return () => document.documentElement.classList.remove("k3-terminal-popout");
  }, []);

  useEffect(() => {
    const onStorage = (ev: StorageEvent) => {
      if (ev.key === THEME_KEY && (ev.newValue === "light" || ev.newValue === "dark")) {
        setTheme(ev.newValue);
      }
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  useEffect(() => {
    const announce = () => postTerminalMessage({ type: "popout-here" });

    const ch = createTerminalChannel((msg) => {
      if (msg.type === "theme") setTheme(msg.theme);
      if (msg.type === "dock") window.close();
      if (msg.type === "lab-query-popout") announce();
    });

    announce();

    const onPageHide = () => postTerminalMessage({ type: "popout-closing" });
    window.addEventListener("pagehide", onPageHide);

    return () => {
      ch.close();
      window.removeEventListener("pagehide", onPageHide);
    };
  }, []);

  const dock = () => {
    postTerminalMessage({ type: "dock" });
    window.close();
  };

  const shell = theme === "dark" ? "dark" : "";
  const statusLabel =
    status === "open" ? "connected" : status === "connecting" ? "connecting…" : "lab window closed";

  return (
    <div
      className={`${shell} flex h-full min-h-0 flex-col bg-[#e4e7ee] text-slate-800 dark:bg-k3-surface-lowest dark:text-k3-on-background`}
    >
      <header className="flex h-12 shrink-0 items-center gap-2 border-b border-slate-400/20 bg-[#d8dce5] px-3 dark:border-k3-outline-variant dark:bg-k3-surface-container-high">
        <div className="flex shrink-0 gap-1">
          <div className="h-3 w-3 rounded-full bg-red-500/50" />
          <div className="h-3 w-3 rounded-full bg-amber-400/50" />
          <div className="h-3 w-3 rounded-full bg-emerald-500/50" />
        </div>
        <span className="hidden items-center gap-2 truncate font-mono text-xs text-slate-600 sm:flex dark:text-k3-on-surface-variant">
          <span
            className={`h-2 w-2 shrink-0 rounded-full ${status === "open" ? "animate-pulse bg-k3-secondary" : "bg-slate-400"}`}
          />
          {statusLabel}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            type="button"
            className="flex items-center gap-1 rounded-md border border-slate-400/30 bg-slate-200/80 px-2 py-1 font-mono text-xs font-medium text-slate-800 transition hover:border-k3-secondary/50 hover:bg-white dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface dark:hover:bg-k3-surface-container-high"
            aria-label="Dock terminal to lab"
            title="Dock terminal to lab"
            onClick={dock}
          >
            <MIcon name="close_fullscreen" className="!text-base" />
            Dock to lab
          </button>
        </div>
      </header>

      <div className="relative min-h-0 flex-1 overflow-hidden p-2">
        {status === "closed" ? (
          <div className="flex h-full flex-col items-center justify-center gap-2 px-4 text-center text-sm text-slate-600 dark:text-k3-on-surface-variant">
            <MIcon name="link_off" className="text-3xl opacity-60" />
            <p>The lab window was closed. Reopen the workshop to use the terminal.</p>
          </div>
        ) : (
          <TerminalView theme={theme} mode="relay" active />
        )}
      </div>

      <footer className="flex h-8 shrink-0 items-center justify-between border-t border-slate-400/20 bg-[#dce0e8] px-3 font-mono text-xs text-slate-600 dark:border-k3-outline-variant dark:bg-k3-surface-container dark:text-k3-on-surface-variant">
        <span>K3sLab shell</span>
        <span className="text-slate-500 dark:text-k3-on-surface-variant">Pop-out</span>
      </footer>
    </div>
  );
}
