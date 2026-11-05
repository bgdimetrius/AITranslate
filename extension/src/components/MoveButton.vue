<script setup lang="ts">
type Direction = "back" | "forward";

const props = defineProps<{
  direction: Direction;
  disabled?: boolean;
}>();

defineEmits<{
  move: [direction: Direction];
}>();
</script>

<template>
  <button
    type="button"
    :disabled="props.disabled"
    :aria-label="props.direction"
    @click="$emit('move', props.direction)"
  >
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      :class="['size-5', { 'rotate-180': props.direction === 'back' }]"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M5 12h14m-6-6 6 6-6 6"
      />
    </svg>
  </button>
</template>
