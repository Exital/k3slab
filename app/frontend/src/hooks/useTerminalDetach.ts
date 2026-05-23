import { useCallback, useEffect, useRef, useState } from "react";
import {
  POPOUT_WINDOW_NAME,
  createTerminalChannel,
  postTerminalMessage,
} from "../terminal/channel";

export const DETACHED_KEY = "k3slab-terminal-detached";

const POPOUT_WIDTH = 960;
const POPOUT_HEIGHT = 640;

/** Window features that request a true popup (not a tab) in Chromium, Firefox, and Safari. */
function popoutWindowFeatures(): string {
  const dualScreenLeft = window.screenLeft ?? window.screenX ?? 0;
  const dualScreenTop = window.screenTop ?? window.screenY ?? 0;
  const viewportWidth = window.outerWidth ?? window.innerWidth ?? POPOUT_WIDTH;
  const viewportHeight = window.outerHeight ?? window.innerHeight ?? POPOUT_HEIGHT;
  const left = Math.round(dualScreenLeft + (viewportWidth - POPOUT_WIDTH) / 2);
  const top = Math.round(dualScreenTop + (viewportHeight - POPOUT_HEIGHT) / 2);

  return [
    "popup=1",
    `width=${POPOUT_WIDTH}`,
    `height=${POPOUT_HEIGHT}`,
    `left=${left}`,
    `top=${top}`,
    "menubar=no",
    "toolbar=no",
    "location=no",
    "status=no",
    "scrollbars=yes",
    "resizable=yes",
  ].join(",");
}

/** Only clear a stale detached flag if no pop-out answers for this long. */
const POPOUT_STALE_MS = 2000;

function popoutUrl() {
  const u = new URL(location.href);
  u.searchParams.set("terminal", "popout");
  return u.toString();
}

export function readDetachedFlag(): boolean {
  try {
    return localStorage.getItem(DETACHED_KEY) === "true";
  } catch {
    return false;
  }
}

function writeDetachedFlag(detached: boolean) {
  try {
    if (detached) localStorage.setItem(DETACHED_KEY, "true");
    else localStorage.removeItem(DETACHED_KEY);
  } catch {
    /* ignore */
  }
}

function isPopoutLocation(win: Window): boolean {
  try {
    return new URL(win.location.href).searchParams.get("terminal") === "popout";
  } catch {
    return false;
  }
}

/** Open a new pop-out (or navigate the named target to the pop-out URL). */
function openPopoutWindow(): Window | null {
  return window.open(popoutUrl(), POPOUT_WINDOW_NAME, popoutWindowFeatures());
}

/** Return the existing named pop-out without changing its URL, if it is our terminal page. */
function getExistingPopout(): Window | null {
  const win = window.open("", POPOUT_WINDOW_NAME, popoutWindowFeatures());
  if (!win || win.closed) return null;
  return isPopoutLocation(win) ? win : null;
}

export function useTerminalDetach() {
  const [detached, setDetached] = useState(readDetachedFlag);
  const [popupBlocked, setPopupBlocked] = useState(false);
  const popoutRef = useRef<Window | null>(null);

  const setDetachedState = useCallback((value: boolean) => {
    setDetached(value);
    writeDetachedFlag(value);
  }, []);

  const dock = useCallback(() => {
    const win = popoutRef.current ?? getExistingPopout();
    if (win && !win.closed) {
      win.close();
    }
    popoutRef.current = null;
    setDetachedState(false);
    setPopupBlocked(false);
  }, [setDetachedState]);

  const attachToPopout = useCallback(
    (win: Window, { focus = true }: { focus?: boolean } = {}) => {
      popoutRef.current = win;
      setDetachedState(true);
      setPopupBlocked(false);
      if (focus) win.focus();
    },
    [setDetachedState],
  );

  const detach = useCallback(() => {
    const existing = popoutRef.current;
    if (existing && !existing.closed) {
      attachToPopout(existing);
      return;
    }

    const reused = getExistingPopout();
    if (reused) {
      attachToPopout(reused);
      return;
    }

    const win = openPopoutWindow();
    if (!win) {
      setPopupBlocked(true);
      setDetachedState(false);
      return;
    }

    attachToPopout(win);
  }, [attachToPopout, setDetachedState]);

  useEffect(() => {
    let popoutAnswered = false;

    const ch = createTerminalChannel((msg) => {
      if (msg.type === "dock") dock();
      if (msg.type === "popout-here") {
        popoutAnswered = true;
        const win = getExistingPopout();
        if (win) attachToPopout(win, { focus: false });
        else setDetachedState(true);
      }
      if (msg.type === "popout-closing") {
        popoutRef.current = null;
        setDetachedState(false);
      }
    });

    postTerminalMessage({ type: "lab-query-popout" });

    const staleTimer = window.setTimeout(() => {
      if (popoutAnswered) return;
      if (!readDetachedFlag()) return;
      setDetachedState(false);
    }, POPOUT_STALE_MS);

    return () => {
      ch.close();
      window.clearTimeout(staleTimer);
    };
  }, [attachToPopout, dock, setDetachedState]);

  useEffect(() => {
    if (!detached) return;
    const id = window.setInterval(() => {
      const win = popoutRef.current;
      if (!win) return;
      if (win.closed) {
        popoutRef.current = null;
        setDetachedState(false);
      }
    }, 400);
    return () => window.clearInterval(id);
  }, [detached, setDetachedState]);

  return { detached, detach, dock, popupBlocked };
}
