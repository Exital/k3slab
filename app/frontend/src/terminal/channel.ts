import type { ThemeMode } from "../theme";

export const TERMINAL_CHANNEL = "k3slab-terminal";

export type TerminalStatus = "connecting" | "open" | "closed";

export type TerminalChannelMessage =
  | { type: "ready" }
  | { type: "output"; data: Uint8Array }
  | { type: "replay"; data: Uint8Array }
  | { type: "input"; data: string }
  | { type: "resize"; cols: number; rows: number }
  | { type: "theme"; theme: ThemeMode }
  | { type: "status"; status: TerminalStatus }
  | { type: "dock" }
  | { type: "lab-query-popout" }
  | { type: "popout-here" }
  | { type: "popout-closing" };

/** Singleton pop-out target — reuses the same window instead of spawning duplicates. */
export const POPOUT_WINDOW_NAME = "k3slab-terminal-popout";

export function postTerminalMessage(msg: TerminalChannelMessage) {
  const ch = new BroadcastChannel(TERMINAL_CHANNEL);
  ch.postMessage(msg);
  ch.close();
}

export function createTerminalChannel(onMessage: (msg: TerminalChannelMessage) => void) {
  const ch = new BroadcastChannel(TERMINAL_CHANNEL);
  ch.onmessage = (ev: MessageEvent<TerminalChannelMessage>) => {
    if (ev.data && typeof ev.data === "object" && "type" in ev.data) {
      onMessage(ev.data);
    }
  };
  return ch;
}
