import {
  TRANSLATION_PORT_NAME,
  type TranslationCommand,
  type TranslationEvent,
  type TranslationStartCommand,
} from '@/types/messages';
import { ApiError, api, isDeepLTranslation } from '@/utils/api-client';

function post(port: Browser.runtime.Port, event: TranslationEvent) {
  port.postMessage(event);
}

function hasAtMostInterpretationLimit(text: string) {
  return text.trim().split(/\s+/u).length <= 21;
}

async function resolveMode(
  command: TranslationStartCommand,
  signal: AbortSignal,
): Promise<'translate' | 'interpret'> {
  if (command.mode !== 'auto') return command.mode;
  if (!hasAtMostInterpretationLimit(command.text)) return 'translate';
  return (await api.analyze(command.text, signal)) ? 'interpret' : 'translate';
}

async function performTranslation(
  port: Browser.runtime.Port,
  command: TranslationStartCommand,
  signal: AbortSignal,
) {
  const translator = command.translator ?? 'chatgpt';
  const response = await api.translate(
    {
      text: command.text,
      targetLanguage: command.targetLanguage,
      sourceLanguage: command.sourceLanguage,
      translator,
    },
    signal,
  );

  if (translator === 'deepl') {
    const payload: unknown = await response.json();
    if (!isDeepLTranslation(payload)) {
      throw new Error('The translation service returned an invalid response.');
    }

    post(port, {
      type: 'translation:meta',
      requestId: command.requestId,
      mode: 'translate',
      sourceLanguage: payload.source_lang,
    });
    post(port, {
      type: 'translation:chunk',
      requestId: command.requestId,
      text: payload.text,
    });
    return;
  }

  if (!response.body) {
    throw new Error('Streaming is not available in this browser.');
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;

      const text = decoder.decode(value, { stream: true });
      if (text) {
        post(port, {
          type: 'translation:chunk',
          requestId: command.requestId,
          text,
        });
      }
    }

    const remaining = decoder.decode();
    if (remaining) {
      post(port, {
        type: 'translation:chunk',
        requestId: command.requestId,
        text: remaining,
      });
    }
  } finally {
    reader.releaseLock();
  }
}

async function handleTranslation(
  port: Browser.runtime.Port,
  command: TranslationStartCommand,
  controllers: Map<string, AbortController>,
) {
  controllers.get(command.requestId)?.abort();
  const controller = new AbortController();
  controllers.set(command.requestId, controller);

  post(port, {
    type: 'translation:started',
    requestId: command.requestId,
  });

  try {
    const mode = await resolveMode(command, controller.signal);
    post(port, {
      type: 'translation:meta',
      requestId: command.requestId,
      mode,
    });

    if (mode === 'interpret') {
      const interpretation = await api.interpret(
        command.text,
        command.targetLanguage,
        controller.signal,
      );
      post(port, {
        type: 'translation:interpretation',
        requestId: command.requestId,
        interpretation,
      });
    } else {
      await performTranslation(port, command, controller.signal);
    }

    post(port, { type: 'translation:done', requestId: command.requestId });
  } catch (error) {
    if (controller.signal.aborted) return;

    post(port, {
      type: 'translation:error',
      requestId: command.requestId,
      message:
        error instanceof Error ? error.message : 'The translation failed.',
      status: error instanceof ApiError ? error.status : undefined,
    });
  } finally {
    if (controllers.get(command.requestId) === controller) {
      controllers.delete(command.requestId);
    }
  }
}

export function registerTranslationPort() {
  browser.runtime.onConnect.addListener((port) => {
    if (port.name !== TRANSLATION_PORT_NAME) return;

    const controllers = new Map<string, AbortController>();

    port.onMessage.addListener((message: TranslationCommand) => {
      if (message.type === 'translation:cancel') {
        controllers.get(message.requestId)?.abort();
        controllers.delete(message.requestId);
        return;
      }

      if (message.type === 'translation:start') {
        void handleTranslation(port, message, controllers);
      }
    });

    port.onDisconnect.addListener(() => {
      for (const controller of controllers.values()) {
        controller.abort();
      }

      controllers.clear();
    });
  });
}
