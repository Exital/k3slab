import { useEffect, useState } from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { TerminalPopout } from "./terminal/TerminalPopout";
import { TerminalSessionProvider } from "./terminal/session";
import { readStoredTheme, THEME_KEY, type ThemeMode } from "./theme";
import "./index.css";
import "@xterm/xterm/css/xterm.css";

const isPopout = new URLSearchParams(location.search).get("terminal") === "popout";

function LabRoot() {
  const [theme, setTheme] = useState<ThemeMode>(() => readStoredTheme());

  useEffect(() => {
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch {
      /* ignore */
    }
  }, [theme]);

  return (
    <TerminalSessionProvider theme={theme}>
      <App theme={theme} setTheme={setTheme} />
    </TerminalSessionProvider>
  );
}

const root = document.getElementById("root")!;
ReactDOM.createRoot(root).render(isPopout ? <TerminalPopout /> : <LabRoot />);
