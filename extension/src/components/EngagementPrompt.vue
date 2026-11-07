<script setup lang="ts">
import { ref } from "vue";
import GoBackButton from "@/components/GoBackButton.vue";
import type { EngagementPrompt } from "@/composables/useEngagementPrompts";

const CHROME_WEB_STORE_REVIEW_URL =
  "https://chromewebstore.google.com/detail/blcdghlkkelnjabklhgoenenkefifoeo/reviews";

const props = defineProps<{
  prompt: EngagementPrompt;
  selectedStars: number;
  saving: boolean;
  error: string;
}>();

const emit = defineEmits<{
  back: [];
  close: [];
  rate: [stars: number];
  submitEmail: [email: string];
}>();

const hoveredStars = ref(0);
const email = ref("");

function submitEmail() {
  const value = email.value.trim();
  if (value) emit("submitEmail", value);
}

async function openStoreReview() {
  await browser.tabs.create({ url: CHROME_WEB_STORE_REVIEW_URL });
  emit("close");
}
</script>

<template>
  <aside
    class="absolute inset-0 z-30 flex flex-col bg-white p-5 shadow-lg"
    role="dialog"
    aria-modal="true"
    :aria-label="prompt === 'email' ? 'Contact invitation' : 'Rate AI Translate'"
  >
    <header class="flex items-center justify-between">
      <GoBackButton
        class="rounded-md p-1 text-gray-600 transition-colors hover:bg-indigo-50 hover:text-indigo-600"
        @back="emit('back')"
      />
      <button
        type="button"
        aria-label="Close"
        class="rounded-md p-1 text-gray-500 transition-colors hover:bg-indigo-50 hover:text-indigo-600"
        @click="emit('close')"
      >
        <svg
          aria-hidden="true"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          class="size-5"
        >
          <path stroke-linecap="round" d="M6 6l12 12M18 6 6 18" />
        </svg>
      </button>
    </header>

    <section
      v-if="prompt === 'rating'"
      class="flex min-h-0 flex-1 flex-col items-center justify-center gap-8 px-12 text-center"
    >
      <p class="max-w-md text-xl font-semibold leading-8 text-gray-800">
        You've been using AI Translate for a while now. How would you rate your
        experience?
      </p>

      <div
        class="flex items-center gap-2"
        role="radiogroup"
        aria-label="Rate your experience from 1 to 5 stars"
        @mouseleave="hoveredStars = 0"
      >
        <button
          v-for="star in 5"
          :key="star"
          type="button"
          role="radio"
          :aria-label="`${star} ${star === 1 ? 'star' : 'stars'}`"
          :aria-checked="selectedStars === star"
          :disabled="saving"
          class="rounded-md p-1 text-amber-400 transition-transform hover:scale-110 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-wait disabled:opacity-60"
          @mouseenter="hoveredStars = star"
          @focus="hoveredStars = star"
          @blur="hoveredStars = 0"
          @click="emit('rate', star)"
        >
          <svg
            aria-hidden="true"
            viewBox="0 0 24 24"
            stroke="currentColor"
            stroke-width="1.6"
            :fill="star <= (hoveredStars || selectedStars) ? 'currentColor' : 'none'"
            class="size-10"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="m12 2.75 2.83 5.73 6.32.92-4.58 4.46 1.08 6.3L12 17.19l-5.65 2.97 1.08-6.3L2.85 9.4l6.32-.92L12 2.75Z"
            />
          </svg>
        </button>
      </div>

      <p v-if="error" class="text-sm text-red-600" role="alert">
        {{ error }}
      </p>
      <p v-else-if="saving" class="text-sm text-gray-500" role="status">
        Saving your feedback…
      </p>
    </section>

    <section
      v-else-if="prompt === 'review'"
      class="flex min-h-0 flex-1 flex-col items-center justify-center gap-8 px-12 text-center"
    >
      <p class="max-w-md text-xl font-semibold leading-8 text-gray-800">
        Thanks for your feedback! Would you help others find us by leaving a
        review?
      </p>

      <button
        type="button"
        class="inline-flex h-12 items-center gap-3 rounded-xl bg-indigo-600 px-5 font-semibold text-white shadow-sm transition-colors hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600"
        @click="openStoreReview"
      >
        <svg aria-hidden="true" viewBox="0 0 32 32" class="size-6">
          <path fill="#fff" d="M5 10h22l-2 17H7L5 10Z" />
          <path fill="#fbbc04" d="M8 10h16V8a8 8 0 0 0-16 0v2Zm3 0V8a5 5 0 0 1 10 0v2H11Z" />
          <circle cx="16" cy="18" r="5.5" fill="#4285f4" />
          <path fill="#34a853" d="m16 18 4.76-2.75A5.5 5.5 0 0 1 16 23.5V18Z" />
          <path fill="#ea4335" d="m16 18-4.76-2.75A5.5 5.5 0 0 1 20.76 15.25L16 18Z" />
        </svg>
        Rate us on Chrome Web Store
      </button>
    </section>

    <section
      v-else
      class="flex min-h-0 flex-1 flex-col items-center justify-center px-12 text-center"
    >
      <form class="flex w-full max-w-sm flex-col items-center gap-6" @submit.prevent="submitEmail">
        <p class="text-xl font-semibold leading-8 text-gray-800">
          You're one of our most active users. We'd love to learn what would
          make AI Translate better. May we contact you?
        </p>

        <label class="w-full text-left text-sm font-medium text-gray-700">
          Email address
          <input
            v-model="email"
            type="email"
            required
            autocomplete="email"
            placeholder="you@example.com"
            :disabled="saving"
            class="mt-2 h-11 w-full rounded-xl border border-gray-300 bg-white px-3.5 text-base text-gray-800 outline-none transition-colors placeholder:text-gray-400 focus:border-indigo-600 focus:ring-2 focus:ring-indigo-100 disabled:cursor-wait disabled:bg-gray-50"
          />
        </label>

        <button
          type="submit"
          :disabled="saving || !email.trim()"
          class="h-11 w-full rounded-xl bg-indigo-600 px-5 font-semibold text-white shadow-sm transition-colors hover:bg-indigo-700 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-600 disabled:cursor-not-allowed disabled:bg-indigo-300"
        >
          {{ saving ? "Sending…" : "Send my email" }}
        </button>

        <p v-if="error" class="text-sm text-red-600" role="alert">
          {{ error }}
        </p>
      </form>
    </section>
  </aside>
</template>
