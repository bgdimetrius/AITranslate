<script setup lang="ts">
defineProps<{
  disabled?: boolean;
}>();

defineEmits<{
  save: [];
}>();
</script>

<template>
  <button
    type="button"
    :disabled="disabled"
    @click="$emit('save')"
  >
    Save Settings
  </button>
</template>
