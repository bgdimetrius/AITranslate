<script lang="ts" setup>
import {
  computed,
  onBeforeUnmount,
  onMounted,
  ref,
  shallowRef,
  watch,
  type Component,
} from "vue";
import CopyButton from "@/components/CopyButton.vue";
import DefaultSwitcher from "@/components/DefaultSwitcher.vue";
import EngagementPrompt from "@/components/EngagementPrompt.vue";
import GoBackButton from "@/components/GoBackButton.vue";
import LanguageBar from "@/components/LanguageBar.vue";
import LanguageSelector from "@/components/LanguageSelector.vue";
import MoveButton from "@/components/MoveButton.vue";
import RatingButton from "@/components/RatingButton.vue";
import SaveSettingsButton from "@/components/SaveSettingsButton.vue";
import SettingsButton from "@/components/SettingsButton.vue";
import TextArea from "@/components/TextArea.vue";
import { useLanguages } from "@/composables/useLanguages";
import {
  useEngagementPrompts,
  type EngagementPrompt as EngagementPromptName,
} from "@/composables/useEngagementPrompts";
import { usePageTranslationSettings } from "@/composables/usePageTranslationSettings";
import { useLanguagePreferences } from "@/composables/useLanguagePreferences";
import { useTranslation } from "@/composables/useTranslation";
import { useTranslationHistory } from "@/composables/useTranslationHistory";
import type { ApiMessageResponse } from "@/types/messages";
import { pickPreferredTarget } from "@/utils/languages";

type Direction = "back" | "forward";
type Rating = "like" | "dislike";

const { data: languages } = useLanguages();
const { source, target } = useLanguagePreferences();
const {
  output,
  loading: translationLoading,
  error: translationError,
  translationId,
  detectedSourceLanguage,
  translate,
  clear: clearTranslation,
  restore: restoreTranslation,
} = useTranslation();
const {
  target: settingsTarget,
  enabled: contextEnabled,
  hydrate: hydratePageTranslationSettings,
  save: savePageTranslationSettings,
  saveTarget: saveSettingsTarget,
  saveEnabled: saveContextEnabled,
} = usePageTranslationSettings();
const {
  current: currentHistoryEntry,
  canGoBack,
  canGoForward,
  add: addHistoryEntry,
  updateTranslation: updateHistoryTranslation,
  updateEntry: updateHistoryEntry,
  updateCurrent: updateCurrentHistoryEntry,
  goBack,
  goForward,
} = useTranslationHistory();
const {
  activePrompt: engagementPrompt,
  selectedStars,
  saving: engagementSaving,
  error: engagementError,
  initialize: initializeEngagementPrompts,
  show: showEngagementPrompt,
  close: closeEngagementPrompt,
  goBack: goBackEngagementPrompt,
  submitRating: submitApplicationRating,
  submitEmail: submitFeedbackEmail,
} = useEngagementPrompts();

const input = ref("");
const rating = ref<Rating | null>(null);
const feedbackSaving = ref(false);
const feedbackError = ref("");
const settingsOpen = ref(false);
const settingsSaved = ref(false);
const debugPanelVisible = ref(true);
const debugPanelComponent = shallowRef<Component | null>(null);

if (import.meta.env.DEV) {
  void import("@/components/EngagementDebugPanel.vue")
    .then((module) => {
      debugPanelComponent.value = module.default;
    })
    .catch((error) => {
      console.error("Failed to load engagement debug controls", error);
    });
}

const settingsLanguages = computed(() => {
  return languages.value.chatgpt?.target ?? [];
});

let historyTimer: ReturnType<typeof setTimeout> | undefined;
let translationTimer: ReturnType<typeof setTimeout> | undefined;
let savedTimer: ReturnType<typeof setTimeout> | undefined;
let restoringHistory = false;
let restoringTranslation = false;

watch(input, (value) => {
  if (restoringHistory) {
    restoringHistory = false;
    return;
  }

  clearTimeout(historyTimer);
  historyTimer = setTimeout(() => {
    addHistoryEntry(value);
  }, 500);
});

watch([input, source, target], ([text, sourceLanguage, targetLanguage]) => {
  clearTimeout(translationTimer);

  if (restoringTranslation) {
    restoringTranslation = false;
    return;
  }

  clearTranslation();
  rating.value = null;
  feedbackError.value = "";

  if (!text.trim() || !targetLanguage) {
    return;
  }

  translationTimer = setTimeout(() => {
    const entry = addHistoryEntry(text);
    const id = translate({
      text,
      sourceLanguage,
      targetLanguage,
      translator: "deepl",
    });
    if (!entry || !id) return;
    updateHistoryEntry(entry, {
      translationId: id,
      requestedSourceLanguage: sourceLanguage,
      detectedSourceLanguage: "",
      targetLanguage,
      assessment: null,
      feedbackRevision: 0,
    });
  }, 350);
});

watch(output, (value) => {
  if (currentHistoryEntry.value?.sourceText !== input.value) return;
  updateHistoryTranslation(value);
});

watch(detectedSourceLanguage, (value) => {
  const entry = currentHistoryEntry.value;
  if (!value || !entry || entry.translationId !== translationId.value) return;
  updateCurrentHistoryEntry({ detectedSourceLanguage: value });
});

watch(
  [settingsLanguages, settingsTarget],
  ([options]) => {
    if (!options.includes(settingsTarget.value)) {
      settingsTarget.value = pickPreferredTarget(options);
    }
  },
  { immediate: true },
);

function move(direction: Direction) {
  clearTimeout(historyTimer);
  clearTimeout(translationTimer);
  const entry = direction === "back" ? goBack() : goForward();
  if (!entry) return;

  restoringHistory = true;
  restoringTranslation = true;
  input.value = entry.sourceText;
  restoreTranslation(
    entry.translatedText,
    entry.translationId,
    entry.detectedSourceLanguage,
  );
  rating.value = entry.assessment;
  feedbackError.value = "";
}

async function copyOutput() {
  if (output.value) await navigator.clipboard.writeText(output.value);
}

async function setRating(value: Rating) {
  if (
    feedbackSaving.value ||
    (rating.value === value && !feedbackError.value)
  ) return;

  const entry = currentHistoryEntry.value;
  if (
    !entry ||
    !entry.translationId ||
    !entry.targetLanguage ||
    !entry.sourceText ||
    !entry.translatedText
  ) return;

  const previous = entry.assessment;
  const revision = entry.feedbackRevision + 1;
  rating.value = value;
  feedbackSaving.value = true;
  feedbackError.value = "";
  updateHistoryEntry(entry, {
    assessment: value,
    feedbackRevision: revision,
  });

  try {
    const response = await browser.runtime.sendMessage({
      type: "api:translation-feedback",
      payload: {
        translation_id: entry.translationId,
        revision,
        translator: "deepl",
        source_lang:
          entry.detectedSourceLanguage ||
          (entry.requestedSourceLanguage === "auto"
            ? ""
            : entry.requestedSourceLanguage),
        target_lang: entry.targetLanguage,
        source_text: entry.sourceText,
        translation_text: entry.translatedText,
        assessment: value,
      },
    }) as ApiMessageResponse;
    if (!response.ok) throw new Error(response.message);
  } catch (error) {
    updateHistoryEntry(entry, { assessment: previous });
    if (currentHistoryEntry.value === entry) rating.value = previous;
    const message = error instanceof Error
      ? error.message
      : "Failed to save translation feedback.";
    if (currentHistoryEntry.value === entry) feedbackError.value = message;
    console.error("Failed to save translation feedback", error);
  } finally {
    feedbackSaving.value = false;
  }
}

async function saveSettings() {
  await savePageTranslationSettings();
  settingsSaved.value = true;
  clearTimeout(savedTimer);
  savedTimer = setTimeout(() => {
    settingsSaved.value = false;
  }, 2000);
}

function triggerDebugPrompt(
  prompt: Extract<EngagementPromptName, "rating" | "email">,
) {
  debugPanelVisible.value = false;
  showEngagementPrompt(prompt);
}

onMounted(() => {
  void hydratePageTranslationSettings();
  void initializeEngagementPrompts();
});

onBeforeUnmount(() => {
  clearTimeout(historyTimer);
  clearTimeout(translationTimer);
  clearTimeout(savedTimer);
});
</script>

<template>
  <div class="h-87.5 w-155 overflow-y-auto bg-white text-gray-800">
    <main class="relative flex h-87.5 w-full overflow-hidden bg-white">
      <section class="flex min-w-full flex-col">
        <SettingsButton
          class="absolute right-2.5 top-2.5 z-10 rounded-md p-1.5 text-gray-500 transition-colors hover:bg-indigo-50 hover:text-indigo-600"
          @open="settingsOpen = true"
        />

      <LanguageBar />

      <div class="flex min-h-0 flex-1 px-2.5 pb-2.5">
        <section
          class="flex min-w-0 flex-1 flex-col rounded-sm ring-1 ring-inset ring-transparent focus-within:ring-indigo-600"
        >
          <TextArea
            v-model="input"
            placeholder-text="Type to translate."
            :spellcheck="false"
            :readonly="false"
            aria-label="Text to translate"
            class="min-h-0 text-gray-800"
          />

          <p
            v-if="translationError"
            class="px-4 pb-2 text-xs text-red-600"
            role="alert"
          >
            {{ translationError }}
          </p>

          <p
            v-else-if="translationLoading && !output"
            class="px-4 pb-2 text-xs text-gray-500"
            role="status"
          >
            Translating…
          </p>

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

          <p
            v-if="feedbackError"
            class="px-4 pt-2 text-xs text-red-600"
            role="alert"
          >
            {{ feedbackError }}
          </p>

          <footer class="flex items-center justify-end gap-3 px-4 py-3">
            <CopyButton
              :disabled="!output"
              class="rounded-md p-1.5 text-gray-600 transition-colors hover:bg-indigo-50 disabled:cursor-default disabled:opacity-40"
              @copy="copyOutput"
            />
            <RatingButton
              type="like"
              :active="rating === 'like'"
              :disabled="!output || !translationId || feedbackSaving"
              class="rounded-md p-1.5 text-gray-600 transition-colors hover:bg-indigo-50 disabled:cursor-default disabled:opacity-40"
              @rate="setRating"
            />
            <RatingButton
              type="dislike"
              :active="rating === 'dislike'"
              :disabled="!output || !translationId || feedbackSaving"
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
          @update:model-value="saveContextEnabled"
        />
      </div>

      <div class="flex items-center justify-between gap-3 py-2 text-sm">
        <span>Target language for page translation</span>
        <LanguageSelector
          v-model="settingsTarget"
          :options="settingsLanguages"
          label="Target language for page translation"
          variant="settings"
          @update:model-value="saveSettingsTarget"
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

      <EngagementPrompt
        v-if="engagementPrompt"
        :prompt="engagementPrompt"
        :selected-stars="selectedStars"
        :saving="engagementSaving"
        :error="engagementError"
        @back="goBackEngagementPrompt"
        @close="closeEngagementPrompt"
        @rate="submitApplicationRating"
        @submit-email="submitFeedbackEmail"
      />
    </main>

    <component
      :is="debugPanelComponent"
      v-if="debugPanelComponent && debugPanelVisible"
      @trigger="triggerDebugPrompt"
    />
  </div>
</template>
