<script lang="ts" setup>
import { computed, onMounted, ref, watch } from "vue";
import LanguageSelector from "./LanguageSelector.vue";
import SwapButton from "./LanguageSwitcher.vue";
import { useLanguages } from "@/composables/useLanguages";

const { data, load } = useLanguages();

const DEFAULT_TARGET = "en-us";

const provider = ref("");
const source = ref("");
const target = ref("");

const caps = computed(() => data.value[provider.value]);
const sourceOptions = computed(() =>
  caps.value ? [...(caps.value.auto_detect ? ["auto"] : []), ...caps.value.source] : []
);
const targetOptions = computed(() => caps.value?.target ?? []);

function pickDefaultTarget(list: string[]) {
  return list.find((l) => l.toLowerCase() === DEFAULT_TARGET) ?? list[0] ?? "";
}

watch(data, (d) => {
  if (!provider.value) provider.value = Object.keys(d)[0] ?? "";
});

watch(
  [sourceOptions, targetOptions],
  ([s, t]) => {
    if (!s.includes(source.value)) source.value = s[0] ?? "";
    if (!t.includes(target.value)) target.value = pickDefaultTarget(t);
  },
  { immediate: true }
);

const canSwap = computed(
  () =>
    source.value !== "auto" &&
    targetOptions.value.includes(source.value) &&
    sourceOptions.value.includes(target.value)
);

function swap() {
  if (!canSwap.value) return;
  const s = source.value;
  source.value = target.value;
  target.value = s;
}

onMounted(load);
</script>

<template>
  <div class="flex items-center justify-center gap-3.5 py-3 px-4 bg-(--lang-selector-bg) border-b border-(--border-color)">
    <LanguageSelector v-model="source" :options="sourceOptions" label="Source language" />
    <SwapButton :disabled="!canSwap" @swap="swap" />
    <LanguageSelector v-model="target" :options="targetOptions" label="Target language" />
  </div>
</template>
