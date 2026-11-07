<script setup lang="ts">
defineEmits<{
  trigger: [prompt: "rating" | "email"];
}>();
</script>

<template>
  <section
    class="border-t border-dashed border-amber-300 bg-amber-50 px-5 py-4"
    aria-label="Engagement prompt debug controls"
  >
    <div class="mb-3 flex items-center justify-between gap-4">
      <div>
        <h2 class="text-sm font-semibold text-amber-950">Debug</h2>
        <p class="text-xs text-amber-800">Preview production engagement prompts.</p>
      </div>
      <span class="rounded-full bg-amber-200 px-2 py-1 text-[10px] font-semibold uppercase tracking-wide text-amber-900">
        Development only
      </span>
    </div>
    <div class="flex gap-3">
      <button
        type="button"
        class="rounded-lg bg-amber-900 px-3 py-2 text-xs font-semibold text-white transition-colors hover:bg-amber-800"
        @click="$emit('trigger', 'rating')"
      >
        Trigger first feedback
      </button>
      <button
        type="button"
        class="rounded-lg bg-amber-900 px-3 py-2 text-xs font-semibold text-white transition-colors hover:bg-amber-800"
        @click="$emit('trigger', 'email')"
      >
        Trigger second feedback
      </button>
    </div>
  </section>
</template>
