import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { terminalWsUrl } from "../api";
import type { ThemeMode } from "../theme";
import { TERMINAL_CHANNEL, type TerminalChannelMessage, type TerminalStatus } from "./channel";

const MAX_BUFFER_BYTES = 512 * 1024;

type OutputListener = (data: Uint8Array) => void;

type TerminalSessionValue = {
  status: TerminalStatus;
  subscribeOutput: (listener: OutputListener) => () => void;
  getReplayBuffer: () => Uint8Array;
  sendInput: (data: string) => void;
  sendResize: (cols: number, rows: number) => void;
  publishTheme: (theme: ThemeMode) => void;
  /** Open a fresh PTY (e.g. after switching labs so cwd matches the active lab). */
  reconnectTerminal: () => void;
  /** Bumps when reconnectTerminal opens a new backend shell. */
  connectionGeneration: number;
};

const TerminalSessionContext = createContext<TerminalSessionValue | null>(null);

export function useTerminalSession(): TerminalSessionValue {
  const ctx = useContext(TerminalSessionContext);
  if (!ctx) throw new Error("useTerminalSession must be used within TerminalSessionProvider");
  return ctx;
}

type TerminalSessionProviderProps = {
  theme: ThemeMode;
  children: ReactNode;
};

export function TerminalSessionProvider({ theme, children }: TerminalSessionProviderProps) {
  const [status, setStatus] = useState<TerminalStatus>("connecting");
  const [connectionGeneration, setConnectionGeneration] = useState(0);
  const wsRef = useRef<WebSocket | null>(null);
  const bufferChunksRef = useRef<Uint8Array[]>([]);
  const bufferSizeRef = useRef(0);
  const listenersRef = useRef(new Set<OutputListener>());
  const themeRef = useRef(theme);
  const statusRef = useRef<TerminalStatus>("connecting");
  const broadcastRef = useRef<BroadcastChannel | null>(null);

  themeRef.current = theme;
  statusRef.current = status;

  const postToPopout = useCallback((msg: TerminalChannelMessage) => {
    try {
      broadcastRef.current?.postMessage(msg);
    } catch {
      /* ignore */
    }
  }, []);

  const appendBuffer = useCallback((data: Uint8Array) => {
    bufferChunksRef.current.push(data);
    bufferSizeRef.current += data.length;
    while (bufferSizeRef.current > MAX_BUFFER_BYTES && bufferChunksRef.current.length > 0) {
      const removed = bufferChunksRef.current.shift()!;
      bufferSizeRef.current -= removed.length;
    }
  }, []);

  const notifyOutput = useCallback((data: Uint8Array) => {
    appendBuffer(data);
    for (const listener of listenersRef.current) {
      listener(data);
    }
    postToPopout({ type: "output", data });
  }, [appendBuffer, postToPopout]);

  const getReplayBuffer = useCallback(() => {
    const total = bufferSizeRef.current;
    const out = new Uint8Array(total);
    let offset = 0;
    for (const chunk of bufferChunksRef.current) {
      out.set(chunk, offset);
      offset += chunk.length;
    }
    return out;
  }, []);

  const sendResize = useCallback((cols: number, rows: number) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN || cols < 1 || rows < 1) return;
    ws.send(JSON.stringify({ type: "resize", cols, rows }));
  }, []);

  const sendInput = useCallback((data: string) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(new TextEncoder().encode(data));
  }, []);

  const publishTheme = useCallback(
    (t: ThemeMode) => {
      postToPopout({ type: "theme", theme: t });
    },
    [postToPopout],
  );

  const reconnectTerminal = useCallback(() => {
    bufferChunksRef.current = [];
    bufferSizeRef.current = 0;
    setConnectionGeneration((g) => g + 1);
  }, []);

  const handlePopoutMessage = useCallback(
    (msg: TerminalChannelMessage) => {
      switch (msg.type) {
        case "ready": {
          postToPopout({ type: "status", status: statusRef.current });
          postToPopout({ type: "theme", theme: themeRef.current });
          const replay = getReplayBuffer();
          if (replay.length > 0) {
            postToPopout({ type: "replay", data: replay });
          }
          break;
        }
        case "input":
          sendInput(msg.data);
          break;
        case "resize":
          sendResize(msg.cols, msg.rows);
          break;
        default:
          break;
      }
    },
    [getReplayBuffer, sendInput, sendResize, postToPopout],
  );

  useEffect(() => {
    publishTheme(theme);
  }, [theme, publishTheme]);

  useEffect(() => {
    const ch = new BroadcastChannel(TERMINAL_CHANNEL);
    broadcastRef.current = ch;
    ch.onmessage = (ev: MessageEvent<TerminalChannelMessage>) => {
      if (ev.data && typeof ev.data === "object" && "type" in ev.data) {
        handlePopoutMessage(ev.data);
      }
    };
    return () => {
      ch.close();
      broadcastRef.current = null;
    };
  }, [handlePopoutMessage]);

  useEffect(() => {
    setStatus("connecting");
    statusRef.current = "connecting";

    const ws = new WebSocket(terminalWsUrl());
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;

    ws.onopen = () => {
      statusRef.current = "open";
      setStatus("open");
      postToPopout({ type: "status", status: "open" });
    };

    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") return;
      notifyOutput(new Uint8Array(ev.data as ArrayBuffer));
    };

    ws.onclose = () => {
      statusRef.current = "closed";
      setStatus("closed");
      postToPopout({ type: "status", status: "closed" });
    };

    ws.onerror = () => {
      ws.close();
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [notifyOutput, postToPopout, connectionGeneration]);

  useEffect(() => {
    const onBeforeUnload = () => {
      postToPopout({ type: "status", status: "closed" });
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [postToPopout]);

  const subscribeOutput = useCallback((listener: OutputListener) => {
    listenersRef.current.add(listener);
    return () => listenersRef.current.delete(listener);
  }, []);

  const value = useMemo(
    () => ({
      status,
      subscribeOutput,
      getReplayBuffer,
      sendInput,
      sendResize,
      publishTheme,
      reconnectTerminal,
      connectionGeneration,
    }),
    [
      status,
      subscribeOutput,
      getReplayBuffer,
      sendInput,
      sendResize,
      publishTheme,
      reconnectTerminal,
      connectionGeneration,
    ],
  );

  return <TerminalSessionContext.Provider value={value}>{children}</TerminalSessionContext.Provider>;
}
