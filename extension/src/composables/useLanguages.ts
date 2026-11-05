import { ref } from "vue";
import type { LanguagesResponse } from "@/types/languages";

const API_URL = "https://ai-translate.bogdanantonovich.com/api/v1";

const data = ref<LanguagesResponse>({});
const loading = ref(false);

export function useLanguages() {
  async function load() {
    loading.value = true;
    try {
      const res = await fetch(`${API_URL}/languages`);
      data.value = await res.json();
    } finally {
      loading.value = false;
    }
  }
  return { data, loading, load };
}
