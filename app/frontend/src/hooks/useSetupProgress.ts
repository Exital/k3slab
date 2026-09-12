import { useEffect, useState } from "react";
import { getProgress, progressStreamUrl, type SetupProgress } from "../api";

const idleProgress: SetupProgress = { pct: 0, message: "", active: false };

/** Live setup/task progress while `active` is true; resets to 0% when activated. */
export function useSetupProgress(active: boolean): SetupProgress {
  const [progress, setProgress] = useState<SetupProgress>(idleProgress);

  useEffect(() => {
    if (!active) {
      setProgress(idleProgress);
      return;
    }

    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;

    setProgress({ pct: 0, message: "", active: true });

    const apply = (snap: SetupProgress) => {
      setProgress({
        pct: Math.max(0, Math.min(100, Math.round(snap.pct ?? 0))),
        message: snap.message ?? "",
        active: snap.active ?? true,
      });
    };

    const connect = () => {
      if (closed) return;
      es = new EventSource(progressStreamUrl());
      es.onmessage = (ev) => {
        try {
          apply(JSON.parse(ev.data) as SetupProgress);
        } catch {
          /* ignore malformed */
        }
      };
      es.onerror = () => {
        es?.close();
        es = null;
        void getProgress()
          .then(apply)
          .catch(() => {});
        if (!closed) {
          reconnectTimer = setTimeout(connect, 3000);
        }
      };
    };

    void getProgress()
      .then(apply)
      .catch(() => {});
    connect();

    return () => {
      closed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      es?.close();
    };
  }, [active]);

  return progress;
}
