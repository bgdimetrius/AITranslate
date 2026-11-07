import { browser } from "wxt/browser";

const ENGLISH_TARGETS = ["en-us", "en", "en-gb"];
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
