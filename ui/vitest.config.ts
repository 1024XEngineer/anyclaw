import viteConfig from "./vite.config";
import { defineConfig, mergeConfig } from "vitest/config";

export default mergeConfig(viteConfig, defineConfig({
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: "./src/test/setup.ts",
  },
}));
