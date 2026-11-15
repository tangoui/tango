<script setup lang="ts">
import { withBase } from "vitepress";
import { data as catalog } from "../data/catalog.data";

const props = withDefaults(defineProps<{ lang?: "zh" | "en" }>(), { lang: "zh" });
const zh = props.lang === "zh";
const sizeLink = withBase(zh ? "/guide/size" : "/en/guide/size");
const platformLink = withBase(zh ? "/guide/platforms" : "/en/guide/platforms");
const componentsLink = withBase("/components/");
</script>

<template>
  <section class="home-stats" :aria-label="zh ? '关键数字' : 'Key numbers'">
    <a class="home-stat" :href="sizeLink">
      <span class="home-stat-value">1.8 <small>MiB</small></span>
      <span class="home-stat-label">{{ zh ? "最小安装包" : "Smallest installer" }}</span>
      <span class="home-stat-note">{{ zh ? "macOS .dmg，一个加载网页的最小 WebView 应用，tan package 默认产物；同一应用的 Windows 安装程序 2.0 MiB" : "macOS .dmg of a minimal WebView app that loads one page, default tan package output; Windows installer of the same app: 2.0 MiB" }}</span>
    </a>
    <a class="home-stat" :href="componentsLink">
      <span class="home-stat-value">{{ catalog.entries ?? "—" }}</span>
      <span class="home-stat-label">{{ zh ? "内置组件" : "Built-in components" }}</span>
      <span class="home-stat-note" v-if="catalog.names">{{ zh ? `组件总表 ${catalog.entries} 项，${catalog.names} 个公开入口` : `${catalog.entries} catalog rows, ${catalog.names} public entry points` }}</span>
    </a>
    <div class="home-stat">
      <span class="home-stat-value">3</span>
      <span class="home-stat-label">{{ zh ? "套设计语言" : "Design languages" }}</span>
      <span class="home-stat-note">Ant Design · Element Plus · shadcn{{ zh ? "，各有深浅色" : ", light and dark" }}</span>
    </div>
    <a class="home-stat" :href="platformLink">
      <span class="home-stat-value">5</span>
      <span class="home-stat-label">{{ zh ? "个目标平台" : "Target platforms" }}</span>
      <span class="home-stat-note">macOS · Windows · Linux · iOS<sup>*</sup> · Android<sup>*</sup><br />{{ zh ? "* 实验性" : "* experimental" }}</span>
    </a>
  </section>
</template>
