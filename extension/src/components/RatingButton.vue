<script setup lang="ts">
type Rating = "like" | "dislike";

const props = defineProps<{
  type: Rating;
  active?: boolean;
}>();

defineEmits<{
  rate: [type: Rating];
}>();
</script>

<template>
  <button
    type="button"
    :aria-label="props.type"
    :aria-pressed="props.active"
    @click="$emit('rate', props.type)"
  >
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      :fill="props.active ? 'currentColor' : 'none'"
      stroke="currentColor"
      stroke-width="2"
      :class="['size-5', { 'rotate-180': props.type === 'dislike' }]"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        d="M7 10v12M15 5.88 14 10h5.83a2 2 0 0 1 1.92 2.56l-2.33 8A2 2 0 0 1 17.5 22H4a2 2 0 0 1-2-2v-8a2 2 0 0 1 2-2h2.76a2 2 0 0 0 1.79-1.11L12 2a3.13 3.13 0 0 1 3 3.88Z"
      />
    </svg>
  </button>
</template>
