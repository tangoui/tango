import DefaultTheme from 'vitepress/theme';
import type { Theme } from 'vitepress';
import ComponentPreview from './components/ComponentPreview.vue';
import ComponentExplorer from './components/ComponentExplorer.vue';
import GalleryPreview from './components/GalleryPreview.vue';
import HomeStats from './components/HomeStats.vue';
import './custom.css';

export default {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component('ComponentPreview', ComponentPreview);
    app.component('ComponentExplorer', ComponentExplorer);
    app.component('GalleryPreview', GalleryPreview);
    app.component('HomeStats', HomeStats);
  },
} satisfies Theme;
