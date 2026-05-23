export const THEME_KEY = "k3slab-theme";
export type ThemeMode = "light" | "dark";

export function readStoredTheme(): ThemeMode {
  try {
    const v = localStorage.getItem(THEME_KEY);
    if (v === "light" || v === "dark") return v;
  } catch {
    /* ignore */
  }
  return "dark";
}
