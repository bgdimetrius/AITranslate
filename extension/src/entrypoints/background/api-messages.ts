import type {
  ApiMessageResponse,
  ApiRequestMessage,
} from '@/types/messages';
import { ApiError, api } from '@/utils/api-client';

function isApiMessage(value: unknown): value is ApiRequestMessage {
  if (typeof value !== 'object' || value === null) return false;
  const type = (value as { type?: unknown }).type;
  return (
    type === 'api:languages' ||
    type === 'api:application-feedback' ||
    type === 'api:feedback-lead' ||
    type === 'api:translation-feedback'
  );
}

async function handleApiMessage(
  message: ApiRequestMessage,
): Promise<ApiMessageResponse> {
  try {
    switch (message.type) {
      case 'api:languages':
        return { ok: true, data: await api.languages() };
      case 'api:application-feedback':
        await api.applicationFeedback(message.payload);
        break;
      case 'api:feedback-lead':
        await api.feedbackLead(message.email);
        break;
      case 'api:translation-feedback':
        await api.translationFeedback(message.payload);
        break;
    }

    return { ok: true };
  } catch (error) {
    return {
      ok: false,
      message: error instanceof Error ? error.message : 'The API request failed.',
      status: error instanceof ApiError ? error.status : undefined,
    };
  }
}

export function registerApiMessages() {
  browser.runtime.onMessage.addListener((message: unknown) => {
    if (!isApiMessage(message)) return undefined;
    return handleApiMessage(message);
  });
}
