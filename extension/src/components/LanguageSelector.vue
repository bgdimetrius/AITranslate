<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref, useTemplateRef } from "vue";
import checkmarkUrl from "../../../icons/checkmark_black.png";
import { getLanguageTitle } from "@/utils/languages";

const props = withDefaults(
  defineProps<{
    options: string[];
    label?: string;
    disabled?: boolean;
    variant?: "main" | "settings";
  }>(),
  {
    label: undefined,
    disabled: false,
    variant: "main",
  },
);

const model = defineModel<string>({ required: true });
const root = useTemplateRef<HTMLElement>("root");
const open = ref(false);

const columns = computed(() => {
  const middle = Math.ceil(props.options.length / 2);
  return [props.options.slice(0, middle), props.options.slice(middle)];
});

const menuPosition = computed(() =>
  props.variant === "settings"
    ? "left-[130px] top-[166px] max-h-[174px]"
    : "left-[90px] top-[55px] max-h-[274px]",
);

function toggle() {
  if (!props.disabled) open.value = !open.value;
}

function select(language: string) {
  model.value = language;
  open.value = false;
}

function handlePointerDown(event: PointerEvent) {
  if (open.value && !root.value?.contains(event.target as Node)) {
    open.value = false;
  }
}

function handleKeyDown(event: KeyboardEvent) {
  if (event.key === "Escape") open.value = false;
}

onMounted(() => {
  document.addEventListener("pointerdown", handlePointerDown);
  document.addEventListener("keydown", handleKeyDown);
});

onBeforeUnmount(() => {
  document.removeEventListener("pointerdown", handlePointerDown);
  document.removeEventListener("keydown", handleKeyDown);
});
</script>

<template>
  <div ref="root" class="relative w-44">
    <button
      type="button"
      :disabled="disabled"
      :aria-label="label"
      aria-haspopup="listbox"
      :aria-expanded="open"
      class="flex h-9 w-44 items-center rounded-lg border border-(--border-color) bg-white px-3 text-left text-sm font-medium text-gray-800 outline-none transition-[background-color,border-color] duration-150 active:border-indigo-600 active:bg-indigo-50 disabled:cursor-not-allowed disabled:opacity-50"
      :class="disabled ? '' : 'cursor-pointer'"
      @click="toggle"
    >
      <span class="min-w-0 flex-1 truncate">{{ getLanguageTitle(model) }}</span>
      <svg
        aria-hidden="true"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        class="ml-2 size-3 shrink-0 text-gray-500"
      >
        <path stroke-linecap="round" stroke-linejoin="round" d="m6 9 6 6 6-6" />
      </svg>
    </button>

    <div
      v-if="open"
      role="listbox"
      :aria-label="label"
      class="fixed z-50 flex w-118 overflow-x-hidden overflow-y-auto rounded-lg border border-gray-200 bg-white py-1.5 shadow-[0_4px_14px_rgba(0,0,0,0.12)]"
      :class="menuPosition"
    >
      <div v-for="(column, columnIndex) in columns" :key="columnIndex" class="w-57 shrink-0">
        <button
          v-for="language in column"
          :key="language"
          type="button"
          role="option"
          :aria-selected="language === model"
          class="flex w-57 items-center gap-1.25 px-3.5 py-2 text-left text-sm leading-4.2 text-gray-800 hover:bg-indigo-50"
          @click="select(language)"
        >
          <img
            v-if="language === model"
            :src="checkmarkUrl"
            alt=""
            class="size-3.75 shrink-0"
          />
          <span>{{ getLanguageTitle(language) }}</span>
        </button>
      </div>
    </div>
  </div>
</template>
