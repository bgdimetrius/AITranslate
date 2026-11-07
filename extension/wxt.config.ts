import { defineConfig } from 'wxt';
import tailwindcss from '@tailwindcss/vite';

// See https://wxt.dev/api/reference/wxt/interfaces/inlineconfig.html
export default defineConfig({
  srcDir: 'src',
  modules: ['@wxt-dev/module-vue'],
  manifest: {
    name: 'In-line AI Translator for Language Learning',
    description:
      'A fast, minimal in-line translator for language learners reading and working online.',
    version: '0.2.3',
    permissions: [
      'storage',
      'contextMenus',
      'clipboardRead',
      'clipboardWrite',
    ],
    host_permissions: ['https://ai-translate.bogdanantonovich.com/*'],
    web_accessible_resources: [
      {
        resources: ['icon/48.png'],
        matches: ['https://docs.google.com/*'],
      },
    ],
    action: {
      default_title: 'AI Translate',
    },
  },
  vite: () => ({
    plugins: [tailwindcss()],
  }),
});
