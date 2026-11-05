<script lang="ts" setup>
import { computed, onBeforeUnmount, ref, watch } from "vue";
import CopyButton from "@/components/CopyButton.vue";
import DefaultSwitcher from "@/components/DefaultSwitcher.vue";
import GoBackButton from "@/components/GoBackButton.vue";
import LanguageBar from "@/components/LanguageBar.vue";
import LanguageSelector from "@/components/LanguageSelector.vue";
import MoveButton from "@/components/MoveButton.vue";
import RatingButton from "@/components/RatingButton.vue";
import SaveSettingsButton from "@/components/SaveSettingsButton.vue";
import SettingsButton from "@/components/SettingsButton.vue";
import TextArea from "@/components/TextArea.vue";
import { useLanguages } from "@/composables/useLanguages";

type Direction = "back" | "forward";
type Rating = "like" | "dislike";

const { data: languages } = useLanguages();

const input = ref("");
const output = ref("");
const rating = ref<Rating | null>(null);
const settingsOpen = ref(false);
const contextEnabled = ref(true);
const settingsTarget = ref("");
const settingsSaved = ref(false);

const history = ref<string[]>([""]);
const historyPosition = ref(0);
const canGoBack = computed(() => historyPosition.value > 0);
const canGoForward = computed(
  () => historyPosition.value < history.value.length - 1,
);

const settingsLanguages = computed(() => {
  const targets = Object.values(languages.value).flatMap(
    (capabilities) => capabilities.target,
  );

  return [...new Set(targets)];
});

let historyTimer: ReturnType<typeof setTimeout> | undefined;
let savedTimer: ReturnType<typeof setTimeout> | undefined;
let restoringHistory = false;

watch(input, (value) => {
  if (restoringHistory) {
    restoringHistory = false;
    return;
  }

  clearTimeout(historyTimer);
  historyTimer = setTimeout(() => {
    if (history.value[historyPosition.value] === value) return;

    history.value = history.value.slice(0, historyPosition.value + 1);
    history.value.push(value);
    historyPosition.value = history.value.length - 1;
  }, 500);
});

watch(settingsLanguages, (options) => {
  if (!options.includes(settingsTarget.value)) {
    settingsTarget.value =
      options.find((language) => language.toLowerCase() === "en-us") ??
      options[0] ??
      "";
  }
});

function move(direction: Direction) {
  if (direction === "back" && canGoBack.value) {
    historyPosition.value -= 1;
  } else if (direction === "forward" && canGoForward.value) {
    historyPosition.value += 1;
  } else {
    return;
  }

  restoringHistory = true;
  input.value = history.value[historyPosition.value] ?? "";
}

async function copyOutput() {
  if (output.value) await navigator.clipboard.writeText(output.value);
}

function setRating(value: Rating) {
  rating.value = rating.value === value ? null : value;
}

function saveSettings() {
  settingsSaved.value = true;
  clearTimeout(savedTimer);
  savedTimer = setTimeout(() => {
    settingsSaved.value = false;
  }, 2000);
}

onBeforeUnmount(() => {
  clearTimeout(historyTimer);
  clearTimeout(savedTimer);
});
</script>

<template>
  <main
    class="relative flex h-[350px] w-[620px] overflow-hidden bg-white text-gray-800"
  >
    <section class="flex min-w-full flex-col">
      <SettingsButton
        class="absolute right-2.5 top-2.5 z-10 rounded-md p-1.5 text-gray-500 transition-colors hover:bg-indigo-50 hover:text-indigo-600"
        @open="settingsOpen = true"
      />

      <LanguageBar />

      <div class="flex min-h-0 flex-1 px-2.5 pb-2.5">
        <section
          class="flex min-w-0 flex-1 flex-col rounded-sm ring-1 ring-transparent focus-within:ring-indigo-600"
        >
          <TextArea
            v-model="input"
            placeholder-text="Type to translate."
            :spellcheck="false"
            :readonly="false"
            aria-label="Text to translate"
            class="min-h-0 text-gray-800"
          />

          <footer class="flex items-center px-4 py-3">
            <div class="flex gap-3">
              <MoveButton
                direction="back"
                :disabled="!canGoBack"
                class="rounded-md p-1.5 text-gray-600 transition-colors hover:bg-indigo-50 disabled:cursor-default disabled:opacity-40"
                @move="move"
              />
              <MoveButton
                direction="forward"
                :disabled="!canGoForward"
                class="rounded-md p-1.5 text-gray-600 transition-colors hover:bg-indigo-50 disabled:cursor-default disabled:opacity-40"
                @move="move"
              />
            </div>
          </footer>
        </section>

        <section class="flex min-w-0 flex-1 flex-col bg-[#f8f9ff]">
          <TextArea
            v-model="output"
            placeholder-text=""
            :spellcheck="false"
            :readonly="true"
            aria-label="Translation"
            class="min-h-0 text-gray-800"
          />

          <footer class="flex items-center justify-end gap-3 px-4 py-3">
            <CopyButton
              :disabled="!output"
              class="rounded-md p-1.5 text-gray-600 transition-colors hover:bg-indigo-50 disabled:cursor-default disabled:opacity-40"
              @copy="copyOutput"
            />
            <RatingButton
              type="like"
              :active="rating === 'like'"
              :disabled="!output"
              class="rounded-md p-1.5 text-gray-600 transition-colors hover:bg-indigo-50 disabled:cursor-default disabled:opacity-40"
              @rate="setRating"
            />
            <RatingButton
              type="dislike"
              :active="rating === 'dislike'"
              :disabled="!output"
              class="rounded-md p-1.5 text-gray-600 transition-colors hover:bg-indigo-50 disabled:cursor-default disabled:opacity-40"
              @rate="setRating"
            />
          </footer>
        </section>
      </div>
    </section>

    <aside
      class="absolute inset-0 z-20 flex flex-col gap-4 bg-white p-5 shadow-lg transition-transform duration-300"
      :class="settingsOpen ? 'translate-x-0' : 'translate-x-full'"
      :aria-hidden="!settingsOpen"
    >
      <header class="flex items-center gap-3">
        <GoBackButton
          class="rounded-md p-1 text-gray-600 transition-colors hover:bg-indigo-50"
          @back="settingsOpen = false"
        />
        <h1 class="text-lg font-semibold">Settings</h1>
      </header>

      <div class="flex items-center justify-between gap-3 py-2 text-sm">
        <span>Enable translation on pages</span>
        <DefaultSwitcher
          v-model="contextEnabled"
          label="Enable translation on pages"
        />
      </div>

      <div class="flex items-center justify-between gap-3 py-2 text-sm">
        <span>Target language for page translation</span>
        <LanguageSelector
          v-model="settingsTarget"
          :options="settingsLanguages"
          label="Target language for page translation"
        />
      </div>

      <div class="mt-auto">
        <p
          class="mb-2.5 text-center text-sm transition-opacity"
          :class="settingsSaved ? 'opacity-100' : 'opacity-0'"
          role="status"
          aria-live="polite"
        >
          Settings saved!
        </p>
        <SaveSettingsButton
          class="h-9 w-full rounded-[10px] bg-indigo-600 font-medium text-white transition-colors hover:bg-indigo-700"
          @save="saveSettings"
        />
      </div>
    </aside>
  </main>
</template>
