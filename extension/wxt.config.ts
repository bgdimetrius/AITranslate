import { defineConfig } from 'wxt';
import tailwindcss from '@tailwindcss/vite';

// See https://wxt.dev/api/reference/wxt/interfaces/inlineconfig.html
export default defineConfig({
  srcDir: 'src',
  modules: ['@wxt-dev/module-vue'],
  manifest: {
    name: 'AI Translator for Language Learning',
    description:
      'Translate words, phrases, and text instantly in-page or from the popup, with contextual meanings designed for language learners.',
    version: '0.2.3',
    icons: {
      16: 'icon/16.png',
      32: 'icon/32.png',
      48: 'icon/48.png',
      96: 'icon/96.png',
      128: 'icon/128.png',
    },
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
