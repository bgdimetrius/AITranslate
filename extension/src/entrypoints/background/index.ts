import { registerApiMessages } from './api-messages';
import { registerContextMenu } from './context-menu';
import { registerTranslationPort } from './translation-port';

export default defineBackground(() => {
  registerApiMessages();
  registerContextMenu();
  registerTranslationPort();
});
