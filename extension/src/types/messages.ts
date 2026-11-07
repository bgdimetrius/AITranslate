import type {
  ApplicationFeedbackRequest,
  Interpretation,
  LanguagesResponse,
  TranslationFeedbackRequest,
} from './api';

export const TRANSLATION_PORT_NAME = 'ai-translate-translation';

export interface InlineTranslateMessage {
  type: 'inline:translate';
  text: string;
}

export interface TranslationStartCommand {
  type: 'translation:start';
  requestId: string;
  mode: 'auto' | 'translate' | 'interpret';
  text: string;
  targetLanguage: string;
  sourceLanguage?: string;
  translator?: 'chatgpt' | 'deepl';
}

export interface TranslationCancelCommand {
  type: 'translation:cancel';
  requestId: string;
}

export type TranslationCommand =
  | TranslationStartCommand
  | TranslationCancelCommand;

export type TranslationEvent =
  | {
      type: 'translation:started';
      requestId: string;
    }
  | {
      type: 'translation:chunk';
      requestId: string;
      text: string;
    }
  | {
      type: 'translation:meta';
      requestId: string;
      mode: 'translate' | 'interpret';
      sourceLanguage?: string;
    }
  | {
      type: 'translation:interpretation';
      requestId: string;
      interpretation: Interpretation;
    }
  | {
      type: 'translation:done';
      requestId: string;
    }
  | {
      type: 'translation:error';
      requestId: string;
      message: string;
      status?: number;
    };

export type ApiRequestMessage =
  | { type: 'api:languages' }
  | { type: 'api:application-feedback'; payload: ApplicationFeedbackRequest }
  | { type: 'api:feedback-lead'; email: string }
  | { type: 'api:translation-feedback'; payload: TranslationFeedbackRequest };

export type ApiMessageResponse =
  | { ok: true; data?: LanguagesResponse }
  | { ok: false; message: string; status?: number };
