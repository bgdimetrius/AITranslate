<script lang="ts" setup>
import { computed, onMounted, watch } from "vue";
import LanguageSelector from "./LanguageSelector.vue";
import SwapButton from "./LanguageSwitcher.vue";
import { useLanguagePreferences } from "@/composables/useLanguagePreferences";
import { useLanguages } from "@/composables/useLanguages";
import {
  pickPreferredTarget,
  resolveCompatibleSource,
  resolveCompatibleTarget,
} from "@/utils/languages";

const { data, load } = useLanguages();
const {
  source,
  target,
  sourceChosen,
  targetChosen,
  chooseSource,
  chooseTarget,
  chooseBoth,
  hydrate: hydratePreferences,
} = useLanguagePreferences();

const caps = computed(() => data.value.deepl);
const sourceOptions = computed(() =>
  caps.value ? [...(caps.value.auto_detect ? ["auto"] : []), ...caps.value.source] : []
);
const targetOptions = computed(() => caps.value?.target ?? []);

watch(
  [sourceOptions, targetOptions, source, target, sourceChosen, targetChosen],
  ([s, t]) => {
    if (!sourceChosen.value || !s.includes(source.value)) {
      if (sourceChosen.value) sourceChosen.value = false;
      source.value =
        s.find((language) => language.toLowerCase() === "auto") ?? s[0] ?? "";
    }
    if (!targetChosen.value || !t.includes(target.value)) {
      if (targetChosen.value) targetChosen.value = false;
      target.value = pickPreferredTarget(t);
    }
  },
  { immediate: true },
);

const swappedLanguages = computed(() => {
  if (source.value.toLowerCase() === "auto" || !caps.value) return null;

  const newSource = resolveCompatibleSource(target.value, caps.value.source);
  const newTarget = resolveCompatibleTarget(source.value, caps.value.target);
  return newSource && newTarget ? { source: newSource, target: newTarget } : null;
});

const canSwap = computed(() => swappedLanguages.value !== null);
const swapLabel = computed(() =>
  canSwap.value
    ? "Swap languages"
    : "These source and target languages cannot be swapped",
);

function swap() {
  const languages = swappedLanguages.value;
  if (!languages) return;
  chooseBoth(languages.source, languages.target);
}

onMounted(() => {
  void hydratePreferences();
  void load();
});
</script>

<template>
  <div class="flex items-center justify-center gap-3.5 py-3 px-4 bg-(--lang-selector-bg) border-b border-(--border-color)">
    <LanguageSelector
      v-model="source"
      :options="sourceOptions"
      label="Source language"
      @update:model-value="chooseSource"
    />
    <SwapButton
      :disabled="!canSwap"
      :label="swapLabel"
      @swap="swap"
    />
    <LanguageSelector
      v-model="target"
      :options="targetOptions"
      label="Target language"
      @update:model-value="chooseTarget"
    />
  </div>
</template>
