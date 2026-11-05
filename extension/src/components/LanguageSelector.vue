<script lang="ts" setup>
defineProps<{
  options: string[];
  label?: string;
  disabled?: boolean;
}>();

const model = defineModel<string>({ required: true });
</script>

<template>
    <div class="relative w-44">
        <select
          v-model="model"
          :disabled="disabled"
          :aria-label="label"
          class="w-full py-2 pl-3 pr-8 font-medium border border-(--border-color) appearance-none cursor-pointer bg-chevron bg-no-repeat bg-position-[right_12px_center] transition-[background-color,border-color] duration-150"
        >
            <option v-for="lang in options" :key="lang" :value="lang">{{ lang }}</option>
        </select>
    </div>
</template>
