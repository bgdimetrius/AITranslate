<script lang="ts" setup>
defineProps<{
  disabled?: boolean;
  label?: string;
}>();

defineEmits<{
  (e: "swap"): void;
}>();
</script>

<template>
  <button
    type="button"
    :disabled="disabled"
    :aria-label="label ?? 'Swap languages'"
    class="flex items-center justify-center size-9 cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed transition-colors duration-150"
    @click="$emit('swap')"
  >
    <svg
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      class="size-5"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4"
      />
    </svg>
  </button>
</template>
