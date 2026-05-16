import { useEffect, useState } from "react";
import { exposedStreamUrl, getExposed, type ExposedEndpoint } from "../api";

export function useExposedEndpoints(): ExposedEndpoint[] {
  const [endpoints, setEndpoints] = useState<ExposedEndpoint[]>([]);

  useEffect(() => {
    let es: EventSource | null = null;
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;

    const apply = (list: ExposedEndpoint[]) => {
      setEndpoints(list);
    };

    const connect = () => {
      if (closed) return;
      es = new EventSource(exposedStreamUrl());
      es.onmessage = (ev) => {
        try {
          const data = JSON.parse(ev.data) as { endpoints?: ExposedEndpoint[] };
          apply(data.endpoints ?? []);
        } catch {
          /* ignore malformed */
        }
      };
      es.onerror = () => {
        es?.close();
        es = null;
        void getExposed()
          .then((snap) => apply(snap.endpoints ?? []))
          .catch(() => {});
        if (!closed) {
          reconnectTimer = setTimeout(connect, 3000);
        }
      };
    };

    const poll = () => {
      void getExposed()
        .then((snap) => apply(snap.endpoints ?? []))
        .catch(() => {});
    };
    poll();
    const pollId = setInterval(poll, 5000);
    connect();

    return () => {
      closed = true;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      clearInterval(pollId);
      es?.close();
    };
  }, []);

  return endpoints;
}
