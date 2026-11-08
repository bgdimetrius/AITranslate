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
const DEFAULT_MARGIN = 15;
const MAX_HEIGHT = 500;
const DEFAULT_WIDTH = 400;
const DEFAULT_HEIGHT = 55;
const OFFSET = 10;

const styles = `
  :host { position: fixed; inset: 0; z-index: 2147483647; pointer-events: none; }
  .window {
    display: none;
    position: fixed;
    width: 380px;
    max-height: 500px;
    pointer-events: auto;
    background: #ffffff;
    color: #111827;
    border: 1px solid #e5e7eb;
    border-radius: 12px;
    box-shadow:
      0 10px 25px -3px rgba(0, 0, 0, 0.1),
      0 4px 6px -2px rgba(0, 0, 0, 0.05),
      0 0 0 1px rgba(0, 0, 0, 0.05);
    z-index: 10000;
    font-family: 'Inter', sans-serif;
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    overflow: hidden;
  }
  .window.open { display: block; }
  .window-content {
    padding: 16px;
    font-family: 'Inter', sans-serif;
    font-size: 14px;
    line-height: 1.5;
    color: #374151;
    background: #ffffff;
    overflow-y: auto;
    max-height: 460px;
    scrollbar-width: thin;
    scrollbar-color: #cbd5e1 #f1f5f9;
  }
  .window-content::-webkit-scrollbar { width: 6px; }
  .window-content::-webkit-scrollbar-track { background: #f1f5f9; border-radius: 3px; }
  .window-content::-webkit-scrollbar-thumb { background: #cbd5e1; border-radius: 3px; }
  .window-content::-webkit-scrollbar-thumb:hover { background: #94a3b8; }
  .status-message {
    font-size: 14px;
    color: #6b7280;
    font-style: italic;
    background: linear-gradient(90deg, #9ca3af 25%, #6b7280 50%, #9ca3af 75%);
    background-size: 200% auto;
    -webkit-background-clip: text;
    background-clip: text;
    color: transparent;
    animation: shimmer 1.5s linear infinite;
  }
  .status-message.error {
    background: none;
    color: #b91c1c;
    animation: none;
    font-style: normal;
  }
  .phrase {
    display: none;
    color: #111827;
    font-family: 'Inter', sans-serif;
    font-size: 16px;
  }
  .translation-result {
    padding: 10px;
    font-family: 'Inter', sans-serif;
    color: #111827;
  }
  .translation-result .part-of-speech {
    font-weight: 600;
    font-size: 14px;
    margin-bottom: 8px;
    text-transform: capitalize;
  }
  .meaning-card {
    background: #f3f4f6;
    border-radius: 12px;
    padding: 10px;
    margin-bottom: 8px;
  }
  .meaning-card .context { font-weight: 500; color: #2563eb; font-size: 13px; }
  .meaning-card .translation { margin-top: 4px; font-size: 14px; font-weight: 600; color: #16a34a; }
  .meaning-card .example { margin-top: 6px; font-size: 13px; font-style: italic; color: #6b7280; }
  @keyframes shimmer {
    0% { background-position: -200% 0; }
    100% { background-position: 200% 0; }
  }
`;

interface InlineController {
  open(text: string, anchor?: DOMRect): Promise<void>;
  showError(message: string, anchor?: DOMRect): void;
  close(): void;
}

interface Placement {
  direction: 'up' | 'down' | 'fixed';
  top: number;
  bottom?: number;
}

function selectionRect(anchor?: DOMRect) {
  if (anchor) return anchor;
  const selection = window.getSelection();
  return selection && selection.rangeCount > 0
    ? selection.getRangeAt(0).getBoundingClientRect()
    : undefined;
}

function placePanel(panel: HTMLElement, anchor?: DOMRect): Placement | undefined {
  if (window.innerWidth < DEFAULT_WIDTH) return undefined;

  const docsCanvas = location.hostname === 'docs.google.com'
    ? document.querySelector<HTMLElement>('.canvas-first-page')
    : null;
  if (docsCanvas) {
    const rect = docsCanvas.getBoundingClientRect();
    const availableWidth = window.innerWidth - rect.right;
    const left = availableWidth >= DEFAULT_WIDTH
      ? rect.right + (availableWidth - DEFAULT_WIDTH) / 2
      : window.innerWidth - DEFAULT_WIDTH;
    panel.style.left = `${left}px`;
    panel.style.top = `${rect.top}px`;
    return { direction: 'fixed', top: rect.top };
  }

  const rect = selectionRect(anchor);
  if (!rect) return undefined;

  const left = window.innerWidth < rect.left + DEFAULT_WIDTH
    ? window.innerWidth - DEFAULT_WIDTH
    : rect.left;
  panel.style.left = `${left}px`;

  if (rect.top > window.innerHeight - rect.bottom) {
    const top = Math.max(0, rect.top - OFFSET - DEFAULT_HEIGHT);
    panel.style.top = `${top}px`;
    return { direction: 'up', top, bottom: rect.top - OFFSET };
  }

  const top = rect.bottom + OFFSET;
  panel.style.top = `${top}px`;
  return { direction: 'down', top };
}

function resizePanel(panel: HTMLElement, placement?: Placement) {
  if (!placement) return;
  const height = Math.min(panel.offsetHeight, MAX_HEIGHT);

  if (placement.direction === 'up' && placement.bottom !== undefined) {
    panel.style.top = `${Math.max(DEFAULT_MARGIN, placement.bottom - height)}px`;
    return;
  }

  if (
    placement.direction === 'down' &&
    placement.top + height + DEFAULT_MARGIN > window.innerHeight
  ) {
    panel.style.top = `${Math.max(DEFAULT_MARGIN, window.innerHeight - height - DEFAULT_MARGIN)}px`;
  }
}

function renderInterpretation(container: HTMLElement, value: Interpretation) {
  container.replaceChildren();
  const wrapper = document.createElement('div');
  wrapper.className = 'translation-result';
  const part = document.createElement('div');
  part.className = 'part-of-speech';
  part.textContent = value.part_of_speech;
  wrapper.append(part);

  for (const meaning of value.meanings) {
    const card = document.createElement('div');
    card.className = 'meaning-card';
    const context = document.createElement('div');
    context.className = 'context';
    context.textContent = meaning.context;
    const translation = document.createElement('div');
    translation.className = 'translation';
    translation.textContent = meaning.translation;
    const example = document.createElement('div');
    example.className = 'example';
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
  panel.className = 'window';
  panel.setAttribute('role', 'dialog');
  panel.setAttribute('aria-label', 'AI translation');
  const content = document.createElement('div');
  content.className = 'window-content';
  const status = document.createElement('div');
  status.className = 'status-message';
  const result = document.createElement('div');
  result.className = 'phrase';
  content.append(status, result);
  panel.append(content);
  container.append(panel);
  let port: Browser.runtime.Port | undefined;
  let requestId: string | undefined;
  let outputText = '';
  let anchorRect: DOMRect | undefined;
  let placement: Placement | undefined;
  let operation = 0;
  let ignoreOutsideClicksUntil = 0;

  const updateSize = () => requestAnimationFrame(() => resizePanel(panel, placement));

  function resetOutput() {
    outputText = '';
    result.replaceChildren();
    result.style.display = 'none';
    result.removeAttribute('dir');
    result.removeAttribute('lang');
    result.style.textAlign = '';
    status.style.display = 'block';
  }

  function setDirection(targetLanguage: string) {
    const language = targetLanguage.toLowerCase().split('-', 1)[0] ?? '';
    if (!['ar', 'he', 'ur'].includes(language)) return;
    result.lang = language;
    result.dir = 'rtl';
    result.style.textAlign = 'right';
  }

  function cancel() {
    if (!port || !requestId) return;
    port.postMessage({ type: 'translation:cancel', requestId });
    requestId = undefined;
  }

  function close() {
    operation += 1;
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
        case 'translation:meta': break;
        case 'translation:chunk':
          outputText += event.text;
          result.textContent = outputText;
          result.style.display = 'block';
          status.style.display = 'none';
          updateSize();
          break;
        case 'translation:interpretation':
          outputText = event.interpretation.meanings.map((meaning) => meaning.translation).join(', ');
          renderInterpretation(result, event.interpretation);
          result.style.display = 'block';
          status.style.display = 'none';
          updateSize();
          break;
        case 'translation:done':
          requestId = undefined;
          break;
        case 'translation:error':
          result.style.display = 'none';
          status.style.display = 'block';
          status.className = 'status-message error';
          status.textContent = event.message;
          requestId = undefined;
          updateSize();
          break;
      }
    });
    port.onDisconnect.addListener(() => {
      port = undefined;
      if (!requestId) return;
      result.style.display = 'none';
      status.style.display = 'block';
      status.className = 'status-message error';
      status.textContent = 'The translation connection was interrupted.';
      requestId = undefined;
      updateSize();
    });
    return port;
  }

  function showError(message: string, anchor?: DOMRect) {
    operation += 1;
    cancel();
    anchorRect = anchor;
    resetOutput();
    status.className = 'status-message error';
    status.textContent = message;
    placement = placePanel(panel, anchorRect);
    if (!placement) return;
    panel.classList.add('open');
    ignoreOutsideClicksUntil = performance.now() + 500;
    updateSize();
  }

  async function open(text: string, anchor?: DOMRect) {
    const currentOperation = ++operation;
    cancel();
    anchorRect = anchor;
    resetOutput();
    status.className = 'status-message';
    status.textContent = 'Translating';
    placement = placePanel(panel, anchorRect);
    if (!placement) {
      panel.classList.remove('open');
      if (window.innerWidth < DEFAULT_WIDTH) window.alert('display is too small');
      return;
    }
    panel.classList.add('open');
    ignoreOutsideClicksUntil = performance.now() + 500;
    updateSize();
    try {
      const targetLanguage = await preferredTargetLanguage();
      if (currentOperation !== operation) return;
      if (!targetLanguage) throw new Error('Choose a target language in settings.');
      setDirection(targetLanguage);
      requestId = crypto.randomUUID();
      ensurePort().postMessage({
        type: 'translation:start', requestId, mode: 'auto', text,
        targetLanguage, translator: 'chatgpt',
      });
    } catch (error) {
      if (currentOperation !== operation) return;
      status.className = 'status-message error';
      status.textContent = error instanceof Error ? error.message : 'Could not start translation.';
      updateSize();
    }
  }

  document.addEventListener('click', (event) => {
    if (!panel.classList.contains('open') || performance.now() < ignoreOutsideClicksUntil) return;
    const rootNode = container.getRootNode();
    const host = rootNode instanceof ShadowRoot ? rootNode.host : container;
    if (!host.contains(event.target as Node)) close();
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
