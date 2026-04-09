import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  build: {
    target: 'es2020',
    rollupOptions: {
      output: {
        manualChunks(id) {
          const normalizedId = id.replace(/\\/g, '/');

          const chatMarkdownPackages = [
            'react-markdown',
            'remark-gfm',
            'remark-',
            'rehype-',
            'unified',
            'trough',
            'micromark',
            'mdast-util',
            'hast-util',
            'estree-util',
            'unist-util',
            'property-information',
            'space-separated-tokens',
            'comma-separated-tokens',
            'style-to-object',
            'style-to-js',
            'vfile',
            'decode-named-character-reference',
            'html-url-attributes',
            'longest-streak',
            'markdown-table',
            'trim-lines',
            'ccount',
            'bail',
            'devlop',
            'character-entities',
          ];

          const codeHighlighterPackages = [
            'react-syntax-highlighter',
            'prismjs',
            'refractor',
            'highlight.js',
            'lowlight',
          ];

          if (
            codeHighlighterPackages.some(
              (pkg) => normalizedId.includes(`/node_modules/${pkg}`),
            )
          ) {
            return 'code-highlighter';
          }

          if (
            chatMarkdownPackages.some(
              (pkg) => normalizedId.includes(`/node_modules/${pkg}`),
            )
          ) {
            return 'chat-markdown';
          }

          if (
            normalizedId.includes('/node_modules/react/') ||
            normalizedId.includes('/node_modules/react-dom/') ||
            normalizedId.includes('/node_modules/scheduler/')
          ) {
            return 'react-core';
          }

          if (normalizedId.includes('/node_modules/@tauri-apps/api/')) {
            return 'tauri';
          }
        },
      },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/rpc': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        timeout: 600000
      },
      '/health': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        timeout: 600000
      },
      '/api/channels': {
        target: 'http://localhost:8080',
        changeOrigin: true,
        timeout: 600000
      },
      '/ws': {
        target: 'ws://localhost:28789',
        ws: true,
      },
    },
  },
});
