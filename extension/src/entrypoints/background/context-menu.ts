import { storage } from 'wxt/utils/storage';
import type { InlineTranslateMessage } from '@/types/messages';

const CONTEXT_MENU_ID = 'ai-translate-selection';
const ENABLED_KEY = 'local:context_menu_enabled' as const;
let synchronization = Promise.resolve();

async function removeContextMenu() {
  try {
    await browser.contextMenus.remove(CONTEXT_MENU_ID);
  } catch {
    // It was not created yet.
  }
}

async function createContextMenu() {
  browser.contextMenus.create({
    id: CONTEXT_MENU_ID,
    title: 'Translate with AI',
    contexts: ['selection'],
  });
}

async function synchronizeContextMenu(enabled?: boolean | null) {
  const shouldEnable =
    enabled ??
    (await storage.getItem<boolean>(ENABLED_KEY)) ??
    true;

  await removeContextMenu();

  if (shouldEnable) {
    await createContextMenu();
  }
}

function scheduleSynchronization(enabled?: boolean | null) {
  synchronization = synchronization
    .then(() => synchronizeContextMenu(enabled))
    .catch((error) => {
      console.error('Could not synchronize the translation context menu', error);
    });
}

export function registerContextMenu() {
  scheduleSynchronization();

  storage.watch<boolean>(ENABLED_KEY, (enabled) => {
    scheduleSynchronization(enabled);
  });

  browser.contextMenus.onClicked.addListener((info, tab) => {
    const text = info.selectionText?.trim();

    if (
      info.menuItemId !== CONTEXT_MENU_ID ||
      !text ||
      tab?.id === undefined
    ) {
      return;
    }

    const message: InlineTranslateMessage = {
      type: 'inline:translate',
      text,
    };

    void browser.tabs.sendMessage(tab.id, message).catch((error) => {
      console.error('Could not open the translation popup', error);
    });
  });
}
