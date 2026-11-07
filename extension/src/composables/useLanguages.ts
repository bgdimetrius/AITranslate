import { ref } from "vue";
import { storage } from "wxt/utils/storage";
import type { LanguagesResponse } from "@/types/languages";
import type { ApiMessageResponse } from "@/types/messages";

const MIRROR_KEY = "languages";
const storedLanguages = storage.defineItem<LanguagesResponse>("local:languages", {
  fallback: {},
});

function readMirroredLanguages(): LanguagesResponse {
  try {
    const value = localStorage.getItem(MIRROR_KEY);
    return value ? JSON.parse(value) : {};
  } catch {
    return {};
  }
}

function mirrorLanguages(languages: LanguagesResponse) {
  try {
    localStorage.setItem(MIRROR_KEY, JSON.stringify(languages));
  } catch (error) {
    console.error("Failed to mirror languages", error);
  }
}

const data = ref<LanguagesResponse>(readMirroredLanguages());
const loading = ref(false);
let hydration: Promise<void> | undefined;

function hydrate() {
  hydration ??= storedLanguages
    .getValue()
    .then((cachedLanguages) => {
      if (Object.keys(cachedLanguages).length > 0) {
        data.value = cachedLanguages;
        mirrorLanguages(cachedLanguages);
      }
    })
    .catch((error) => {
      console.error("Failed to read stored languages", error);
    });

  return hydration;
}

export function useLanguages() {
  async function load() {
    loading.value = true;
    try {
      await hydrate();

      const response = await browser.runtime.sendMessage({
        type: "api:languages",
      }) as ApiMessageResponse;
      if (!response.ok || !response.data) {
        throw new Error(response.ok ? "No languages are available" : response.message);
      }

      const languages: LanguagesResponse = response.data;
      data.value = languages;
      mirrorLanguages(languages);
      await storedLanguages.setValue(languages);
    } finally {
      loading.value = false;
    }
  }
  return { data, loading, hydrate, load };
}
