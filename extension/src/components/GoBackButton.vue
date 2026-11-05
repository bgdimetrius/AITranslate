<script setup lang="ts">
defineEmits<{
  back: [];
}>();
</script>

<template>
  <button
    type="button"
    aria-label="Go back"
    @click="$emit('back')"
  >
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      class="size-5"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="m15 18-6-6 6-6"
      />
    </svg>
  </button>
</template>
