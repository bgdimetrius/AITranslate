export interface Capabilities {
  source: string[];
  target: string[];
  auto_detect: boolean;
}

export type LanguagesResponse = Record<string, Capabilities>;
