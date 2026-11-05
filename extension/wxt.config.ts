import { defineConfig } from 'wxt';
import tailwindcss from '@tailwindcss/vite';

// See https://wxt.dev/api/reference/wxt/interfaces/inlineconfig.html
export default defineConfig({
  srcDir: 'src',
  modules: ['@wxt-dev/module-vue'],
  manifest: {
    host_permissions: ['https://ai-translate.bogdanantonovich.com/*'],
  },
  vite: () => ({
    plugins: [tailwindcss()],
  }),
});
