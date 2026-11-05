<script setup lang="ts">
defineProps<{
  placeholderText: string;
  spellcheck: boolean;
  readonly: boolean;
}>();

const model = defineModel<string>({ required: true });
</script>

<template>
  <textarea
    v-model="model"
    :placeholder="placeholderText"
    :spellcheck="spellcheck"
    :readonly="readonly"
    class="flex-1 p-5 border-none outline-none resize-none font-[inherit] text-base leading-[1.6] bg-transparent"
  />
</template>
