import { onScopeDispose, ref } from 'vue';
import type { TranslationProvider } from '@/types/api';
import {
  TRANSLATION_PORT_NAME,
  type TranslationEvent,
} from '@/types/messages';

export interface TranslationOptions {
  text: string;
  sourceLanguage?: string;
  targetLanguage: string;
  translator?: TranslationProvider;
}

export function useTranslation() {
  const output = ref('');
  const loading = ref(false);
  const error = ref('');
  const translationId = ref('');
  const detectedSourceLanguage = ref('');
  let requestId: string | undefined;
  let port: Browser.runtime.Port | undefined;

  function cancel() {
    if (!port || !requestId) return;
    port.postMessage({ type: 'translation:cancel', requestId });
    requestId = undefined;
    loading.value = false;
  }

  function connect() {
    if (port) return port;
    port = browser.runtime.connect({ name: TRANSLATION_PORT_NAME });
    port.onMessage.addListener((event: TranslationEvent) => {
      if (event.requestId !== requestId) return;

      switch (event.type) {
        case 'translation:started':
          loading.value = true;
          break;
        case 'translation:chunk':
          output.value += event.text;
          break;
        case 'translation:interpretation':
          output.value = event.interpretation.meanings
            .map((meaning) => meaning.translation)
            .join(', ');
          break;
        case 'translation:done':
          loading.value = false;
          requestId = undefined;
          break;
        case 'translation:error':
          loading.value = false;
          error.value = event.message;
          translationId.value = '';
          detectedSourceLanguage.value = '';
          requestId = undefined;
          break;
        case 'translation:meta':
          if (event.sourceLanguage) {
            detectedSourceLanguage.value = event.sourceLanguage;
          }
          break;
      }
    });
    port.onDisconnect.addListener(() => {
      port = undefined;
      if (!requestId) return;
      requestId = undefined;
      loading.value = false;
      error.value = 'The translation connection was interrupted.';
      translationId.value = '';
      detectedSourceLanguage.value = '';
    });
    return port;
  }

  function translate(options: TranslationOptions) {
    clear();

    const text = options.text.trim();
    if (!text || !options.targetLanguage) return;

    requestId = crypto.randomUUID();
    translationId.value = requestId;
    connect().postMessage({
      type: 'translation:start',
      requestId,
      mode: 'translate',
      text,
      sourceLanguage:
        options.sourceLanguage === 'auto' ? undefined : options.sourceLanguage,
      targetLanguage: options.targetLanguage,
      translator: options.translator ?? 'chatgpt',
    });

    return requestId;
  }

  function clear() {
    cancel();
    output.value = '';
    error.value = '';
    translationId.value = '';
    detectedSourceLanguage.value = '';
  }

  function restore(
    translatedText: string,
    restoredTranslationId: string,
    restoredSourceLanguage: string,
  ) {
    clear();
    output.value = translatedText;
    translationId.value = restoredTranslationId;
    detectedSourceLanguage.value = restoredSourceLanguage;
  }

  onScopeDispose(() => {
    cancel();
    port?.disconnect();
    port = undefined;
  });

  return {
    output,
    loading,
    error,
    translationId,
    detectedSourceLanguage,
    translate,
    cancel,
    clear,
    restore,
  };
}
