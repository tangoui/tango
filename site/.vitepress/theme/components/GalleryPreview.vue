<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import catalog from '../../generated/components.json';
import ComponentPreview from './ComponentPreview.vue';
const props = defineProps<{ component?: string; scene?: string }>();
const entry = computed(() => catalog.components.find(item => item.id === props.component));
const choices = computed(() => entry.value?.demos ?? (props.scene ? [props.scene] : []));
const selected = ref(choices.value[0] ?? '');
watch(() => [props.component, props.scene], () => { selected.value = choices.value[0] ?? ''; });
const current = computed(() => catalog.demos.find(item => item.name === selected.value));
</script>
<template>
  <div v-if="current">
    <label v-if="choices.length > 1" class="catalog-scene-select">示例场景
      <select v-model="selected"><option v-for="name in choices" :key="name" :value="name">{{ catalog.demos.find(item => item.name === name)?.title || name }}</option></select>
    </label>
    <ComponentPreview :component="`gallery:${selected}`" :title="current.title" />
    <p class="catalog-source"><a :href="`https://github.com/tangoui/tango/blob/main/${current.source}#L${current.line}`" target="_blank" rel="noopener">查看此场景的 Go 源码 ↗</a></p>
  </div>
  <p v-else role="alert">未找到对应的组件演示，请重新构建文档。</p>
</template>
