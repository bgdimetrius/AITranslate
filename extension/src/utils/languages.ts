import { browser } from "wxt/browser";

const ENGLISH_TARGETS = ["en-us", "en", "en-gb"];
const DEFAULT_TARGET_VARIANTS: Record<string, string[]> = {
  en: ["en-us", "en-gb"],
  pt: ["pt-br", "pt-pt"],
  zh: ["zh-hans", "zh-hant"],
};
const languageNames = new Intl.DisplayNames(
  [browser.i18n.getUILanguage()],
  {
    type: "language",
    languageDisplay: "dialect",
    fallback: "code",
  },
);

export function getLanguageTitle(language: string) {
  if (language.toLowerCase() === "auto") return "Auto-detect";

  try {
    return languageNames.of(language.replaceAll("_", "-")) ?? language;
  } catch {
    return language;
  }
}

function normalizeLanguage(language: string) {
  return language.trim().toLowerCase().replaceAll("_", "-");
}

function baseLanguage(language: string) {
  return normalizeLanguage(language).split("-")[0] ?? "";
}

function exactLanguage(language: string, options: string[]) {
  const normalized = normalizeLanguage(language);
  return options.find((option) => normalizeLanguage(option) === normalized);
}

export function resolveCompatibleSource(
  language: string,
  options: string[],
) {
  const exact = exactLanguage(language, options);
  if (exact) return exact;

  const base = baseLanguage(language);
  return options.find((option) => baseLanguage(option) === base) ?? "";
}

export function resolveCompatibleTarget(
  language: string,
  options: string[],
) {
  const exact = exactLanguage(language, options);
  if (exact) return exact;

  const base = baseLanguage(language);
  const compatible = options.filter(
    (option) => baseLanguage(option) === base,
  );
  if (compatible.length === 0) return "";

  const browserLanguage = browser.i18n
    .getUILanguage()
    .toLowerCase()
    .replaceAll("_", "-");
  const browserVariant = exactLanguage(browserLanguage, compatible);
  if (browserVariant) return browserVariant;

  for (const variant of DEFAULT_TARGET_VARIANTS[base] ?? []) {
    const match = exactLanguage(variant, compatible);
    if (match) return match;
  }

  return compatible[0];
}

export function pickPreferredTarget(languages: string[]) {
  const browserLanguage = browser.i18n
    .getUILanguage()
    .toLowerCase()
    .replaceAll("_", "-");
  const browserLanguageCandidates = [
    browserLanguage,
    browserLanguage.split("-")[0],
  ];

  for (const candidate of browserLanguageCandidates) {
    const match = languages.find(
      (language) => language.toLowerCase() === candidate,
    );
    if (match) return match;
  }

  for (const englishTarget of ENGLISH_TARGETS) {
    const match = languages.find(
      (language) => language.toLowerCase() === englishTarget,
    );
    if (match) return match;
  }

  return languages[0] ?? "";
}
