<script setup lang="ts">
import { computed } from 'vue';
import { withBase } from 'vitepress';
const props = defineProps<{ component: string; title?: string; height?: number }>();
const frameHeight = computed(() => props.height ?? (props.component.startsWith("gallery:") ? 728 : 448));
const src = computed(() => withBase(`/playground/index.html?component=${encodeURIComponent(props.component)}`));
</script>

<template>
  <section class="tango-preview" aria-label="组件在线交互">
    <div class="tango-preview-heading"><span class="tango-preview-dot" /><strong>{{ title || component }} · 在线体验</strong><span>真实 Go 组件</span></div>
    <iframe :key="src" :src="src" :title="`${title || component} 组件交互预览`" :style="{ height: `${frameHeight}px` }" loading="lazy" sandbox="allow-scripts allow-same-origin" />
    <a class="tango-preview-open" :href="src" target="_blank" rel="noopener">在独立页面打开 ↗</a>
  </section>
</template>
