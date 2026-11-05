<script setup lang="ts">
const model = defineModel<boolean>({ default: true });

defineProps<{
  label?: string;
}>();
</script>

<template>
  <label class="inline-flex cursor-pointer items-center">
    <input
      v-model="model"
      type="checkbox"
      class="peer sr-only"
      :aria-label="label ?? 'Toggle setting'"
    />

    <span
      class="relative h-6 w-11 rounded-full bg-gray-300 transition-colors
             after:absolute after:left-0.5 after:top-0.5 after:size-5
             after:rounded-full after:bg-white after:transition-transform
             peer-checked:bg-indigo-500
             peer-checked:after:translate-x-5"
    />
  </label>
</template>
