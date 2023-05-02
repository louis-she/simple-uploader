import { resolve } from "path";
import { defineConfig } from "vite";
import dts from "vite-plugin-dts";

export default defineConfig({
  server: {
    proxy: {
      "^/files.*": "http://127.0.0.1:8080",
    },
  },
  build: {
    target: ["es2019"],
    lib: {
      entry: resolve(__dirname, "src/simple_uploader.ts"),
      name: "simple_uploader",
      fileName: "simple_uploader",
    },
    rollupOptions: {
      output: {
        globals: {
          SimpleUploader: "SimpleUploader",
        },
      },
    },
  },
  plugins: [dts()],
});
