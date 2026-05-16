/** @type {import('tailwindcss').Config} */
export default {
  darkMode: "class",
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        k3: {
          background: "#051424",
          surface: "#051424",
          "surface-low": "#0d1c2d",
          "surface-lowest": "#010f1f",
          "surface-container": "#122131",
          "surface-container-high": "#1c2b3c",
          "surface-container-highest": "#273647",
          "surface-variant": "#273647",
          "on-background": "#d4e4fa",
          "on-surface": "#d4e4fa",
          "on-surface-variant": "#c3c6d6",
          "outline-variant": "#434654",
          primary: "#b2c5ff",
          "primary-container": "#326ce5",
          "on-primary-container": "#faf9ff",
          secondary: "#4edea3",
          "on-secondary": "#003824",
          "secondary-container": "#00a572",
          "on-secondary-container": "#00311f",
          error: "#ffb4ab",
          "on-error-container": "#ffdad6",
          "error-container": "#93000a",
        },
      },
      fontFamily: {
        sans: ["Inter", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "ui-monospace", "monospace"],
        display: ["Geist", "Inter", "ui-sans-serif", "system-ui", "sans-serif"],
      },
    },
  },
  plugins: [],
};
