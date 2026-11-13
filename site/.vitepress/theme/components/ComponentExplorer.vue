<script setup lang="ts">
import { computed, ref } from 'vue';
import { withBase } from 'vitepress';
import catalog from '../../generated/components.json';
import GalleryPreview from './GalleryPreview.vue';
import { searchCatalog } from './catalog-search.mjs';
const search = ref('');
const group = ref('全部');
const mode = ref<'components' | 'scenes'>('components');
const selected = ref({ kind: 'components', id: catalog.components[0].id });
const visible = computed(() => searchCatalog(catalog, search.value, group.value, mode.value));
const component = computed(() => selected.value.kind === 'components' ? catalog.components.find(item => item.id === selected.value.id) : undefined);
const scene = computed(() => selected.value.kind === 'scenes' ? catalog.demos.find(item => item.name === selected.value.id) : undefined);
</script>
<template>
  <div class="catalog-toolbar">
    <label class="catalog-search">搜索组件<input v-model="search" type="search" placeholder="搜索名称、API 或中文用途" /></label>
    <label>浏览范围<select v-model="mode"><option value="components">全部组件（{{ catalog.components.length }}）</option><option value="scenes">全部场景（{{ catalog.demos.length }}）</option></select></label>
  </div>
  <div class="tango-explorer-tabs catalog-groups" role="group" aria-label="组件分类">
    <button v-for="name in ['全部', ...catalog.groups]" :key="name" type="button" :aria-pressed="group === name" @click="group = name">{{ name }}</button>
  </div>
  <div class="catalog-layout">
    <nav class="catalog-list" aria-label="在线组件目录">
      <p class="catalog-result-count">{{ visible.length }} 项结果</p>
      <button v-for="item in visible" :key="`${mode}:${item.id}`" type="button" :aria-pressed="selected.kind === mode && selected.id === item.id" @click="selected = { kind:mode, id:item.id }"><span>{{ item.label }}</span><small>{{ item.group }}</small></button>
      <p v-if="!visible.length" role="status">没有匹配的组件，请换一个关键词。</p>
    </nav>
    <section class="catalog-detail" aria-label="组件效果预览">
      <template v-if="component"><h2>{{ component.names.slice(0,3).join(' · ') }}</h2><p class="catalog-component-apis">{{ component.names.join(' / ') }}</p><a :href="withBase(`/components/catalog/${component.slug}`)">用法、场景和源码 →</a><GalleryPreview :component="component.id" /></template>
      <template v-else-if="scene"><h2>{{ scene.title }}</h2><GalleryPreview :scene="scene.name" /></template>
    </section>
  </div>
</template>
