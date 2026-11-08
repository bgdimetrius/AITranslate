import { ref } from "vue";
import { storage } from "wxt/utils/storage";
import type { ApiMessageResponse } from "@/types/messages";

export type EngagementPrompt = "rating" | "review" | "email";

const FIRST_FEEDBACK_OPEN = 10;
const FEEDBACK_LEAD_OPEN = 20;

const popupOpenCount = storage.defineItem<number>("local:popup-open-count", {
  fallback: 0,
});
const applicationFeedbackHandled = storage.defineItem<boolean>(
  "local:application-feedback-handled",
  { fallback: false },
);
const feedbackLeadHandled = storage.defineItem<boolean>(
  "local:feedback-lead-handled",
  { fallback: false },
);

function normalizedCount(value: number) {
  return Number.isSafeInteger(value) && value >= 0 ? value : 0;
}

export function useEngagementPrompts() {
  const activePrompt = ref<EngagementPrompt | null>(null);
  const selectedStars = ref(0);
  const saving = ref(false);
  const error = ref("");

  async function initialize() {
    try {
      const nextOpenCount = normalizedCount(await popupOpenCount.getValue()) + 1;
      await popupOpenCount.setValue(nextOpenCount);

      const [feedbackHandled, leadHandled] = await Promise.all([
        applicationFeedbackHandled.getValue(),
        feedbackLeadHandled.getValue(),
      ]);

      if (nextOpenCount >= FEEDBACK_LEAD_OPEN && !leadHandled) {
        activePrompt.value = "email";
      } else if (nextOpenCount >= FIRST_FEEDBACK_OPEN && !feedbackHandled) {
        activePrompt.value = "rating";
      }
    } catch (storageError) {
      console.error("Failed to initialize engagement prompts", storageError);
    }
  }

  async function close() {
    const prompt = activePrompt.value;
    activePrompt.value = null;
    error.value = "";

    try {
      if (prompt === "rating") {
        await applicationFeedbackHandled.setValue(true);
      } else if (prompt === "email") {
        await feedbackLeadHandled.setValue(true);
      }
    } catch (storageError) {
      console.error("Failed to store the engagement prompt state", storageError);
    }
  }

  async function goBack() {
    if (activePrompt.value === "review") {
      activePrompt.value = "rating";
      error.value = "";
      return;
    }

    await close();
  }

  async function submitRating(stars: number) {
    if (saving.value || stars < 1 || stars > 5) return;

    selectedStars.value = stars;
    saving.value = true;
    error.value = "";

    try {
      const response = await browser.runtime.sendMessage({
        type: "api:application-feedback",
        payload: {
          rating: stars >= 4 ? 1 : -1,
          text: `${stars} out of 5 stars`,
        },
      }) as ApiMessageResponse;

      if (!response.ok) throw new Error(response.message);
      await applicationFeedbackHandled.setValue(true);
      activePrompt.value = "review";
    } catch (requestError) {
      error.value = requestError instanceof Error
        ? requestError.message
        : "We couldn't save your rating. Please try again.";
      console.error("Failed to save application feedback", requestError);
    } finally {
      saving.value = false;
    }
  }

  async function submitEmail(email: string) {
    if (saving.value) return;

    saving.value = true;
    error.value = "";

    try {
      const response = await browser.runtime.sendMessage({
        type: "api:feedback-lead",
        email,
      }) as ApiMessageResponse;

      if (!response.ok) throw new Error(response.message);
      await feedbackLeadHandled.setValue(true);
      activePrompt.value = null;
    } catch (requestError) {
      error.value = requestError instanceof Error
        ? requestError.message
        : "We couldn't save your email. Please try again.";
      console.error("Failed to save feedback lead", requestError);
    } finally {
      saving.value = false;
    }
  }

  return {
    activePrompt,
    selectedStars,
    saving,
    error,
    initialize,
    close,
    goBack,
    submitRating,
    submitEmail,
  };
}
