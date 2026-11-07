import type { ContentScriptContext } from 'wxt/utils/content-script-context';
import { storage } from 'wxt/utils/storage';

const DOCS_BUBBLE_ID = 'docs-instant-bubble';
const BUTTON_MARKER = 'data-ai-translate-docs-button';
const ENABLED_KEY = 'local:context_menu_enabled' as const;

export interface GoogleDocsTranslationTarget {
  text: string;
  anchor: DOMRect;
}

interface GoogleDocsIntegrationOptions {
  onTranslate(target: GoogleDocsTranslationTarget): void | Promise<void>;
  onError(message: string, anchor: DOMRect): void;
}

function directSelection() {
  return window.getSelection()?.toString().trim() ?? '';
}

function iframeSelection() {
  for (const frame of document.querySelectorAll('iframe')) {
    try {
      const text = frame.contentWindow?.getSelection()?.toString().trim();
      if (text) return text;
    } catch {
      // Cross-origin frames cannot expose their selection.
    }
  }
  return '';
}

async function clipboardSelection() {
  let previous: string | undefined;

  try {
    previous = await navigator.clipboard.readText();
  } catch {
    // Clipboard read can be unavailable despite extension permissions.
  }

  try {
    if (!document.execCommand('copy')) return '';
    await new Promise((resolve) => window.setTimeout(resolve, 40));
    return (await navigator.clipboard.readText()).trim();
  } catch {
    return '';
  } finally {
    if (previous !== undefined) {
      try {
        await navigator.clipboard.writeText(previous);
      } catch {
        // Restoring the previous clipboard is best-effort.
      }
    }
  }
}

async function selectedDocsText(cachedSelection: string) {
  return (
    directSelection() ||
    iframeSelection() ||
    (await clipboardSelection()) ||
    cachedSelection
  );
}

function createButton(
  bubble: HTMLElement,
  getCachedSelection: () => string,
  options: GoogleDocsIntegrationOptions,
) {
  if (bubble.querySelector(`[${BUTTON_MARKER}]`)) return;

  const host = document.createElement('div');
  host.setAttribute(BUTTON_MARKER, '');
  host.style.cssText = 'display:flex;align-items:center;justify-content:center;';

  const shadow = host.attachShadow({ mode: 'open' });
  const style = document.createElement('style');
  style.textContent = `
    :host { display: flex; align-items: center; justify-content: center; }
    button { display: grid; width: 36px; height: 36px; padding: 5px; place-items: center; border: 0; border-radius: 50%; background: transparent; cursor: pointer; }
    button:hover { background: rgba(79, 70, 229, .1); }
    button:focus-visible { outline: 2px solid #4f46e5; outline-offset: 1px; }
    img { width: 26px; height: 26px; object-fit: contain; }
  `;

  const button = document.createElement('button');
  button.type = 'button';
  button.title = 'Translate selection with AI';
  button.setAttribute('aria-label', 'Translate selection with AI');
  const icon = document.createElement('img');
  icon.src = browser.runtime.getURL('/icon/48.png');
  icon.alt = '';
  button.append(icon);
  shadow.append(style, button);

  button.addEventListener('mousedown', (event) => {
    // Keep the editor selection active while the toolbar button receives focus.
    event.preventDefault();
    event.stopPropagation();
  });
  button.addEventListener('click', (event) => {
    event.preventDefault();
    event.stopPropagation();

    const anchor = bubble.getBoundingClientRect();
    void selectedDocsText(getCachedSelection()).then((text) => {
      if (!text) {
        options.onError('Select some text in the document first.', anchor);
        return;
      }
      return options.onTranslate({ text, anchor });
    });
  });

  const reference = bubble.querySelector<HTMLElement>(
    '.superfab-button-container',
  );
  if (reference) {
    const size = reference.getBoundingClientRect();
    if (size.width) host.style.width = `${size.width}px`;
    if (size.height) host.style.height = `${size.height}px`;
  }

  bubble.prepend(host);
}

export function installGoogleDocsIntegration(
  ctx: ContentScriptContext,
  options: GoogleDocsIntegrationOptions,
) {
  let cachedSelection = '';
  let enabled = true;

  const removeButtons = () => {
    document
      .querySelectorAll<HTMLElement>(`[${BUTTON_MARKER}]`)
      .forEach((button) => button.remove());
  };

  const updateCachedSelection = () => {
    const text = directSelection() || iframeSelection();
    if (text) cachedSelection = text;
  };

  const attachButton = () => {
    if (!enabled) {
      removeButtons();
      return;
    }
    const bubble = document.getElementById(DOCS_BUBBLE_ID);
    if (bubble instanceof HTMLElement) {
      createButton(bubble, () => cachedSelection, options);
    }
  };

  ctx.addEventListener(document, 'selectionchange', updateCachedSelection);
  ctx.addEventListener(document, 'mouseup', () => {
    ctx.setTimeout(updateCachedSelection, 100);
  });
  ctx.addEventListener(document, 'keyup', updateCachedSelection);

  const observer = new MutationObserver(attachButton);
  observer.observe(document.body, { childList: true, subtree: true });
  attachButton();

  void storage.getItem<boolean>(ENABLED_KEY).then((saved) => {
    enabled = saved ?? true;
    attachButton();
  });
  const unwatch = storage.watch<boolean>(ENABLED_KEY, (value) => {
    enabled = value ?? true;
    attachButton();
  });

  ctx.onInvalidated(() => {
    observer.disconnect();
    unwatch();
    removeButtons();
  });
}
