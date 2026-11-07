import {
  API_URL,
  type ApplicationFeedbackRequest,
  type DeepLTranslation,
  type Interpretation,
  type LanguagesResponse,
  type TranslateRequest,
  type TranslationFeedbackRequest,
} from '@/types/api';

interface ErrorPayload {
  message?: string;
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly requestId?: string,
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

let sessionRecovery: Promise<void> | undefined;

function endpoint(path: string) {
  return `${API_URL}${path}`;
}

async function errorFromResponse(response: Response) {
  let message = `API request failed (${response.status})`;

  try {
    const payload = (await response.clone().json()) as ErrorPayload;
    if (payload.message) message = payload.message;
  } catch {
    // Some successful streaming routes and several status-only routes have no JSON body.
  }

  return new ApiError(
    message,
    response.status,
    response.headers.get('X-Request-ID') ?? undefined,
  );
}

async function rawFetch(path: string, init: RequestInit = {}) {
  return fetch(endpoint(path), {
    ...init,
    credentials: 'include',
  });
}

async function createInstallation() {
  const response = await rawFetch('/users/new', { method: 'POST' });
  if (!response.ok) throw await errorFromResponse(response);
}

async function refreshSession() {
  const response = await rawFetch('/auth/handshake', { method: 'POST' });
  if (!response.ok) throw await errorFromResponse(response);
}

async function recoverSession() {
  sessionRecovery ??= (async () => {
    try {
      await refreshSession();
    } catch (error) {
      if (
        !(error instanceof ApiError) ||
        ![400, 401, 403].includes(error.status)
      ) {
        throw error;
      }

      await createInstallation();
    }
  })().finally(() => {
    sessionRecovery = undefined;
  });

  return sessionRecovery;
}

async function authenticatedFetch(path: string, init: RequestInit = {}) {
  let response = await rawFetch(path, init);

  if (response.status === 401) {
    await recoverSession();
    response = await rawFetch(path, init);
  }

  if (!response.ok) throw await errorFromResponse(response);
  return response;
}

function jsonInit(method: 'POST' | 'PUT', value: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(value),
  };
}

function translationQuery(request: TranslateRequest) {
  const query = new URLSearchParams({
    target_lang: request.targetLanguage,
    translator: request.translator ?? 'chatgpt',
    text: request.text,
  });

  if (request.sourceLanguage && request.sourceLanguage !== 'auto') {
    query.set('source_lang', request.sourceLanguage);
  }

  return query;
}

export const api = {
  createInstallation,
  refreshSession,

  async languages(signal?: AbortSignal): Promise<LanguagesResponse> {
    const response = await rawFetch('/languages', { signal });
    if (!response.ok) throw await errorFromResponse(response);
    return response.json() as Promise<LanguagesResponse>;
  },

  translate(request: TranslateRequest, signal?: AbortSignal) {
    return authenticatedFetch(`/translate?${translationQuery(request)}`, {
      signal,
    });
  },

  async interpret(
    text: string,
    targetLanguage: string,
    signal?: AbortSignal,
  ): Promise<Interpretation> {
    const query = new URLSearchParams({
      text,
      target_lang: targetLanguage,
    });
    const response = await authenticatedFetch(`/interpretate?${query}`, {
      signal,
    });
    return response.json() as Promise<Interpretation>;
  },

  async analyze(text: string, signal?: AbortSignal) {
    const query = new URLSearchParams({ text });
    const response = await rawFetch(`/interpretate/analyze?${query}`, {
      method: 'POST',
      signal,
    });

    if (response.status === 401) {
      await recoverSession();
      const retried = await rawFetch(`/interpretate/analyze?${query}`, {
        method: 'POST',
        signal,
      });
      if (retried.status === 406) return false;
      if (!retried.ok) throw await errorFromResponse(retried);
      return true;
    }

    if (response.status === 406) return false;
    if (!response.ok) throw await errorFromResponse(response);
    return true;
  },

  async applicationFeedback(request: ApplicationFeedbackRequest) {
    await authenticatedFetch(
      '/analytics/feedback',
      jsonInit('PUT', request),
    );
  },

  async feedbackLead(email: string) {
    await authenticatedFetch(
      '/analytics/feedback/order',
      jsonInit('PUT', { email }),
    );
  },

  async translationFeedback(request: TranslationFeedbackRequest) {
    await authenticatedFetch(
      '/analytics/feedback/translation',
      jsonInit('PUT', request),
    );
  },
};

export function isDeepLTranslation(value: unknown): value is DeepLTranslation {
  if (!value || typeof value !== 'object') return false;
  const translation = value as Partial<DeepLTranslation>;
  return (
    typeof translation.source_lang === 'string' &&
    typeof translation.text === 'string'
  );
}
