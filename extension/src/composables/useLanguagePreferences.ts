import { ref, watch } from "vue";
import { storage } from "wxt/utils/storage";

interface StoredLanguagePreferences {
  source: string;
  target: string;
  sourceChosen?: boolean;
  targetChosen?: boolean;
}

interface LanguagePreferences {
  source: string;
  target: string;
  sourceChosen: boolean;
  targetChosen: boolean;
}

const MIRROR_KEY = "language-preferences";
const storedPreferences =
  storage.defineItem<StoredLanguagePreferences>("local:language-preferences");

function isEnglish(language: string) {
  return ["en", "en-us", "en-gb"].includes(language.toLowerCase());
}

function normalizePreferences(value: unknown): LanguagePreferences | null {
  if (!value || typeof value !== "object") return null;

  const preferences = value as Partial<StoredLanguagePreferences>;
  const source = typeof preferences.source === "string" ? preferences.source : "auto";
  const target = typeof preferences.target === "string" ? preferences.target : "";

  return {
    source,
    target,
    sourceChosen:
      typeof preferences.sourceChosen === "boolean"
        ? preferences.sourceChosen
        : source.toLowerCase() !== "auto",
    targetChosen:
      typeof preferences.targetChosen === "boolean"
        ? preferences.targetChosen
        : target !== "" && !isEnglish(target),
  };
}

function readMirroredPreferences(): LanguagePreferences {
  try {
    const value = localStorage.getItem(MIRROR_KEY);
    return normalizePreferences(value ? JSON.parse(value) : null) ?? {
      source: "auto",
      target: "",
      sourceChosen: false,
      targetChosen: false,
    };
  } catch {
    return {
      source: "auto",
      target: "",
      sourceChosen: false,
      targetChosen: false,
    };
  }
}

const mirroredPreferences = readMirroredPreferences();
const source = ref(mirroredPreferences.source);
const target = ref(mirroredPreferences.target);
const sourceChosen = ref(mirroredPreferences.sourceChosen);
const targetChosen = ref(mirroredPreferences.targetChosen);

let hydrated = false;
let hydration: Promise<void> | undefined;
let writeQueue = Promise.resolve();

function persist() {
  const preferences: LanguagePreferences = {
    source: source.value,
    target: target.value,
    sourceChosen: sourceChosen.value,
    targetChosen: targetChosen.value,
  };

  try {
    localStorage.setItem(MIRROR_KEY, JSON.stringify(preferences));
  } catch (error) {
    console.error("Failed to mirror language preferences", error);
  }

  if (!hydrated) return;

  writeQueue = writeQueue
    .then(() => storedPreferences.setValue(preferences))
    .catch((error) => {
      console.error("Failed to store language preferences", error);
    });
}

watch([source, target, sourceChosen, targetChosen], persist, { flush: "sync" });

function chooseSource(value: string) {
  sourceChosen.value = true;
  source.value = value;
}

function chooseTarget(value: string) {
  targetChosen.value = true;
  target.value = value;
}

function chooseBoth(newSource: string, newTarget: string) {
  sourceChosen.value = true;
  targetChosen.value = true;
  source.value = newSource;
  target.value = newTarget;
}

function hydrate() {
  hydration ??= storedPreferences
    .getValue()
    .then((storedValue) => {
      const preferences = normalizePreferences(storedValue);
      if (!preferences) return;

      source.value = preferences.source;
      target.value = preferences.target;
      sourceChosen.value = preferences.sourceChosen;
      targetChosen.value = preferences.targetChosen;
    })
    .catch((error) => {
      console.error("Failed to read language preferences", error);
    })
    .finally(() => {
      hydrated = true;
      persist();
    });

  return hydration;
}

export function useLanguagePreferences() {
  return {
    source,
    target,
    sourceChosen,
    targetChosen,
    chooseSource,
    chooseTarget,
    chooseBoth,
    hydrate,
  };
}
