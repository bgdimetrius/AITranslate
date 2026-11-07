import { storage } from 'wxt/utils/storage';
import type { Interpretation } from '@/types/api';
import {
  TRANSLATION_PORT_NAME,
  type ApiMessageResponse,
  type InlineTranslateMessage,
  type TranslationEvent,
} from '@/types/messages';
import { pickPreferredTarget } from '@/utils/languages';
import { installGoogleDocsIntegration } from '@/utils/google-docs';

const TARGET_LANGUAGE_KEY = 'local:settings_target_lang' as const;

const styles = `
  :host { position: fixed; inset: 0; z-index: 2147483647; pointer-events: none; }
  .panel { position: fixed; display: none; box-sizing: border-box; width: min(380px, calc(100vw - 24px)); max-height: min(500px, calc(100vh - 24px)); overflow: hidden; pointer-events: auto; border: 1px solid #e5e7eb; border-radius: 12px; background: #fff; color: #111827; box-shadow: 0 18px 45px rgba(15, 23, 42, .18); font-family: Inter, ui-sans-serif, system-ui, sans-serif; }
  .panel.open { display: flex; flex-direction: column; animation: appear .14s ease-out; }
  .header { display: flex; align-items: center; justify-content: space-between; min-height: 40px; padding: 0 10px 0 14px; border-bottom: 1px solid #eef0f4; }
  .title { font-size: 13px; font-weight: 650; color: #4f46e5; }
  button { border: 0; background: transparent; color: inherit; font: inherit; }
  .icon-button { display: grid; width: 30px; height: 30px; place-items: center; border-radius: 7px; cursor: pointer; color: #64748b; }
  .icon-button:hover { background: #eef2ff; color: #4338ca; }
  .content { min-height: 58px; overflow: auto; padding: 14px 16px; font-size: 14px; line-height: 1.55; white-space: pre-wrap; overflow-wrap: anywhere; }
  .status { color: #64748b; }
  .status.loading { background: linear-gradient(90deg, #94a3b8, #475569, #94a3b8); background-size: 200% 100%; color: transparent; background-clip: text; animation: shimmer 1.3s linear infinite; }
  .status.error { color: #b91c1c; }
  .result:empty { display: none; }
  .interpretation { display: grid; gap: 8px; }
  .part { font-weight: 650; text-transform: capitalize; }
  .meaning { border-radius: 9px; background: #f8fafc; padding: 10px; }
  .meaning-context { color: #4338ca; font-size: 13px; font-weight: 550; }
  .meaning-translation { margin-top: 3px; font-weight: 650; }
  .meaning-example { margin-top: 5px; color: #64748b; font-size: 13px; font-style: italic; }
  .footer { display: none; justify-content: flex-end; padding: 8px 10px; border-top: 1px solid #eef0f4; }
  .footer.visible { display: flex; }
  .copy { border-radius: 7px; padding: 6px 10px; cursor: pointer; color: #475569; font-size: 12px; font-weight: 600; }
  .copy:hover { background: #eef2ff; color: #4338ca; }
  @keyframes appear { from { opacity: 0; transform: translateY(-4px) scale(.98); } }
  @keyframes shimmer { to { background-position: -200% 0; } }
`;

interface InlineController {
  open(text: string, anchor?: DOMRect): Promise<void>;
  showError(message: string, anchor?: DOMRect): void;
  close(): void;
}

function placePanel(panel: HTMLElement, anchor?: DOMRect) {
  const margin = 12;
  const gap = 10;
  const selection = window.getSelection();
  const rect = anchor ?? (selection && selection.rangeCount > 0
    ? selection.getRangeAt(0).getBoundingClientRect()
    : undefined);
  const desiredLeft = rect?.left ?? (window.innerWidth - 380) / 2;
  const left = Math.max(margin, Math.min(desiredLeft, window.innerWidth - panel.offsetWidth - margin));
  const below = (rect?.bottom ?? margin) + gap;
  const above = (rect?.top ?? window.innerHeight / 2) - panel.offsetHeight - gap;
  const top = below + panel.offsetHeight <= window.innerHeight - margin
    ? below
    : Math.max(margin, above);
  panel.style.left = `${left}px`;
  panel.style.top = `${top}px`;
}

function renderInterpretation(container: HTMLElement, value: Interpretation) {
  container.replaceChildren();
  const wrapper = document.createElement('div');
  wrapper.className = 'interpretation';
  const part = document.createElement('div');
  part.className = 'part';
  part.textContent = value.part_of_speech;
  wrapper.append(part);

  for (const meaning of value.meanings) {
    const card = document.createElement('div');
    card.className = 'meaning';
    const context = document.createElement('div');
    context.className = 'meaning-context';
    context.textContent = meaning.context;
    const translation = document.createElement('div');
    translation.className = 'meaning-translation';
    translation.textContent = meaning.translation;
    const example = document.createElement('div');
    example.className = 'meaning-example';
    example.textContent = meaning.example;
    card.append(context, translation, example);
    wrapper.append(card);
  }
  container.append(wrapper);
}

async function preferredTargetLanguage() {
  const saved = await storage.getItem<string>(TARGET_LANGUAGE_KEY);
  if (saved) return saved;
  const response = await browser.runtime.sendMessage({ type: 'api:languages' }) as ApiMessageResponse;
  if (!response.ok || !response.data) {
    throw new Error(response.ok ? 'No languages are available.' : response.message);
  }
  return pickPreferredTarget(response.data.chatgpt?.target ?? []);
}

function mountInlineUi(container: HTMLElement): InlineController {
  const panel = document.createElement('section');
  panel.className = 'panel';
  panel.setAttribute('role', 'dialog');
  panel.setAttribute('aria-label', 'AI translation');
  const header = document.createElement('header');
  header.className = 'header';
  const title = document.createElement('span');
  title.className = 'title';
  title.textContent = 'AI Translate';
  const closeButton = document.createElement('button');
  closeButton.className = 'icon-button close';
  closeButton.type = 'button';
  closeButton.setAttribute('aria-label', 'Close');
  closeButton.textContent = '✕';
  header.append(title, closeButton);

  const content = document.createElement('div');
  content.className = 'content';
  const status = document.createElement('div');
  status.className = 'status';
  const result = document.createElement('div');
  result.className = 'result';
  content.append(status, result);

  const footer = document.createElement('footer');
  footer.className = 'footer';
  const copy = document.createElement('button');
  copy.className = 'copy';
  copy.type = 'button';
  copy.textContent = 'Copy';
  footer.append(copy);

  panel.append(header, content, footer);
  container.append(panel);
  let port: Browser.runtime.Port | undefined;
  let requestId: string | undefined;
  let outputText = '';
  let anchorRect: DOMRect | undefined;

  function cancel() {
    if (!port || !requestId) return;
    port.postMessage({ type: 'translation:cancel', requestId });
    requestId = undefined;
  }

  function close() {
    cancel();
    panel.classList.remove('open');
  }

  function ensurePort() {
    if (port) return port;
    port = browser.runtime.connect({ name: TRANSLATION_PORT_NAME });
    port.onMessage.addListener((event: TranslationEvent) => {
      if (event.requestId !== requestId) return;
      switch (event.type) {
        case 'translation:started': break;
        case 'translation:meta':
          status.textContent = event.mode === 'interpret' ? 'Interpreting…' : 'Translating…';
          break;
        case 'translation:chunk':
          outputText += event.text;
          result.textContent = outputText;
          status.hidden = true;
          footer.classList.add('visible');
          placePanel(panel, anchorRect);
          break;
        case 'translation:interpretation':
          outputText = event.interpretation.meanings.map((meaning) => meaning.translation).join(', ');
          renderInterpretation(result, event.interpretation);
          status.hidden = true;
          footer.classList.add('visible');
          placePanel(panel, anchorRect);
          break;
        case 'translation:done':
          status.classList.remove('loading');
          requestId = undefined;
          break;
        case 'translation:error':
          status.hidden = false;
          status.className = 'status error';
          status.textContent = event.message;
          requestId = undefined;
          placePanel(panel, anchorRect);
          break;
      }
    });
    port.onDisconnect.addListener(() => {
      port = undefined;
      if (!requestId) return;
      status.hidden = false;
      status.className = 'status error';
      status.textContent = 'The translation connection was interrupted.';
      requestId = undefined;
    });
    return port;
  }

  function showError(message: string, anchor?: DOMRect) {
    cancel();
    anchorRect = anchor;
    outputText = '';
    result.replaceChildren();
    footer.classList.remove('visible');
    status.hidden = false;
    status.className = 'status error';
    status.textContent = message;
    panel.classList.add('open');
    placePanel(panel, anchorRect);
  }

  async function open(text: string, anchor?: DOMRect) {
    cancel();
    anchorRect = anchor;
    outputText = '';
    result.replaceChildren();
    footer.classList.remove('visible');
    status.hidden = false;
    status.className = 'status loading';
    status.textContent = 'Getting translation…';
    panel.classList.add('open');
    placePanel(panel, anchorRect);
    try {
      const targetLanguage = await preferredTargetLanguage();
      if (!targetLanguage) throw new Error('Choose a target language in settings.');
      requestId = crypto.randomUUID();
      ensurePort().postMessage({
        type: 'translation:start', requestId, mode: 'auto', text,
        targetLanguage, translator: 'chatgpt',
      });
    } catch (error) {
      status.className = 'status error';
      status.textContent = error instanceof Error ? error.message : 'Could not start translation.';
    }
  }

  closeButton.addEventListener('click', close);
  copy.addEventListener('click', () => {
    void navigator.clipboard.writeText(outputText).then(() => {
      copy.textContent = 'Copied';
      window.setTimeout(() => (copy.textContent = 'Copy'), 1200);
    });
  });
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') close();
  });
  return { open, showError, close };
}

export default defineContentScript({
  matches: ['<all_urls>'],
  runAt: 'document_end',
  cssInjectionMode: 'ui',
  async main(ctx) {
    const ui = await createShadowRootUi(ctx, {
      name: 'ai-translate-inline',
      position: 'inline',
      anchor: 'body',
      css: styles,
      isolateEvents: true,
      onMount: mountInlineUi,
    });
    ui.mount();

    if (location.hostname === 'docs.google.com' && location.pathname.startsWith('/document/')) {
      installGoogleDocsIntegration(ctx, {
        onTranslate: ({ text, anchor }) => ui.mounted?.open(text, anchor),
        onError: (message, anchor) => ui.mounted?.showError(message, anchor),
      });
    }

    browser.runtime.onMessage.addListener((message: unknown) => {
      const inline = message as Partial<InlineTranslateMessage>;
      if (inline.type !== 'inline:translate' || typeof inline.text !== 'string') return undefined;
      void ui.mounted?.open(inline.text.trim());
      return undefined;
    });
  },
});
