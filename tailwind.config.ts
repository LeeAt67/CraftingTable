import animate from "tailwindcss-animate";
import type { Config } from "tailwindcss";

export default {
  darkMode: ["class", "[data-theme='dark']"],
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        background: "var(--background)",
        foreground: "var(--textPrimary)",
        surface: "var(--surface)",
        surfaceSubtle: "var(--surfaceSubtle)",
        textPrimary: "var(--textPrimary)",
        textBody: "var(--textBody)",
        textSecondary: "var(--textSecondary)",
        border: "var(--border)",
        brand: "var(--brand)",
        brandHover: "var(--brandHover)",
        brandSoft: "var(--brandSoft)",
        success: "var(--success)",
        card: {
          DEFAULT: "var(--surface)",
          foreground: "var(--textPrimary)",
        },
        popover: {
          DEFAULT: "var(--surface)",
          foreground: "var(--textPrimary)",
        },
        primary: {
          DEFAULT: "var(--brand)",
          foreground: "#FFFFFF",
        },
        secondary: {
          DEFAULT: "var(--surfaceSubtle)",
          foreground: "var(--textPrimary)",
        },
        muted: {
          DEFAULT: "var(--surfaceSubtle)",
          foreground: "var(--textSecondary)",
        },
        accent: {
          DEFAULT: "var(--brandSoft)",
          foreground: "var(--brand)",
        },
        destructive: {
          DEFAULT: "var(--destructive)",
          foreground: "#FFFFFF",
        },
        input: "var(--border)",
        ring: "var(--brand)",
        ringOffset: "var(--background)",
      },
      borderRadius: {
        lg: "var(--radius)",
        md: "calc(var(--radius) - 2px)",
        sm: "calc(var(--radius) - 4px)",
      },
    },
  },
  plugins: [animate],
} satisfies Config;
