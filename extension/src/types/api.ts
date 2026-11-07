import type { LanguagesResponse } from './languages';

export const API_URL = 'https://ai-translate.bogdanantonovich.com/api/v1';

export type TranslationProvider = 'chatgpt' | 'deepl';
export type TranslationAssessment = 'like' | 'dislike';

export interface TranslateRequest {
  text: string;
  targetLanguage: string;
  sourceLanguage?: string;
  translator?: TranslationProvider;
}

export interface DeepLTranslation {
  source_lang: string;
  text: string;
}

export interface InterpretationMeaning {
  context: string;
  translation: string;
  example: string;
}

export interface Interpretation {
  part_of_speech: string;
  meanings: InterpretationMeaning[];
}

export interface ApplicationFeedbackRequest {
  rating: 1 | -1;
  text?: string;
}

export interface TranslationFeedbackRequest {
  translation_id: string;
  revision: number;
  translator: TranslationProvider;
  source_lang: string;
  target_lang: string;
  source_text: string;
  translation_text: string;
  assessment: TranslationAssessment;
}

export type { LanguagesResponse };
