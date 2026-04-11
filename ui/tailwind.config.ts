import type { Config } from "tailwindcss";

const config: Config = {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      borderColor: {
        skin: "var(--line)",
        soft: "var(--line-soft)",
      },
      borderRadius: {
        xl2: "1.5rem",
        xl3: "1.75rem",
      },
      boxShadow: {
        panel: "var(--shadow-panel)",
        soft: "var(--shadow-soft)",
      },
      colors: {
        canvas: "var(--canvas)",
        ink: "var(--ink)",
        mute: "var(--mute)",
        panel: "var(--panel)",
        panelAlt: "var(--panel-alt)",
        line: "var(--line)",
        accent: "var(--accent)",
        accentSoft: "var(--accent-soft)",
        glow: "var(--glow)",
      },
      fontFamily: {
        sans: [
          "\"Plus Jakarta Sans\"",
          "\"PingFang SC\"",
          "\"Hiragino Sans GB\"",
          "\"Microsoft YaHei UI\"",
          "\"Segoe UI Variable\"",
          "sans-serif",
        ],
      },
    },
  },
  plugins: [],
};

export default config;
