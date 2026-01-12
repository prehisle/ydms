import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 后端 API 地址，支持环境变量覆盖
// 本地开发: VITE_API_TARGET=http://localhost:9002 npm run dev
// 默认使用 9180 (生产/测试环境端口)
const apiTarget = process.env.VITE_API_TARGET || "http://localhost:9180";

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    open: true,
    host: "0.0.0.0",
    proxy: {
      "/api": {
        target: apiTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          'monaco-editor': ['@monaco-editor/react', 'monaco-editor'],
          'yaml-parser': ['js-yaml'],
          'html-sanitizer': ['dompurify'],
          'react-vendor': ['react', 'react-dom', 'react-router-dom'],
          'antd-vendor': ['antd'],
        },
      },
    },
    chunkSizeWarningLimit: 1000,
  },
});
