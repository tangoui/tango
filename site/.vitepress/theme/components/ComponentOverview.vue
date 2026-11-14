<script setup lang="ts">
import { computed, ref } from 'vue';
import { withBase } from 'vitepress';
import catalog from '../../generated/components.json';

// The overview of /components/: one card per catalog entry, grouped like
// the sidebar; the filter matches the label and the public API names.
type Entry = { group: string; names: string[]; slug: string; label?: string; doc?: string; reference?: string };
const query = ref('');
const groups = computed(() => {
  const q = query.value.trim().toLowerCase();
  const match = (e: Entry) => !q || (e.label ?? '').toLowerCase().includes(q) || e.names.some(n => n.toLowerCase().includes(q));
  return (catalog.groups as string[])
    .map(group => ({ group, items: (catalog.components as Entry[]).filter(e => e.group === group && match(e)) }))
    .filter(g => g.items.length);
});
const total = computed(() => groups.value.reduce((n, g) => n + g.items.length, 0));
const link = (e: Entry) => withBase(e.doc ?? `/components/catalog/${e.slug}`);
</script>
<template>
  <div class="overview-filter">
    <input v-model="query" type="search" placeholder="搜索组件名称或 API，例如 Table、日期" aria-label="搜索组件" />
    <span>{{ total }} 个组件</span>
  </div>
  <section v-for="g in groups" :key="g.group" class="overview-group">
    <h2 :id="g.group">{{ g.group }} <small>{{ g.items.length }}</small></h2>
    <div class="overview-grid">
      <a v-for="e in g.items" :key="e.slug" class="overview-card" :href="link(e)">
        <strong>{{ e.label ?? e.names[0] }}</strong>
        <span>{{ e.names.join(' · ') }}</span>
      </a>
    </div>
  </section>
  <p v-if="!total" role="status">没有匹配的组件，请换一个关键词。</p>
</template>
