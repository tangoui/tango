import { mkdirSync, writeFileSync, rmSync } from 'node:fs';
import path from 'node:path';

export function generateComponentPages(site, catalog) {
  const directory = path.join(site,'components/catalog');
  rmSync(directory,{recursive:true,force:true});
  mkdirSync(directory,{recursive:true});
  for (const entry of catalog.components) {
    const title=entry.names.slice(0,3).join(' · ');
    const sourceLinks=entry.demos.map(name=>catalog.demos.find(demo=>demo.name===name)).filter(Boolean)
      .map(demo=>`- [${demo.title}](https://github.com/tangoui/tango/blob/main/${demo.source}#L${demo.line})`);
    writeFileSync(path.join(directory,entry.slug+'.md'),`---
title: ${JSON.stringify(title)}
editLink: false
---
# ${title}

${entry.group}组件。公开入口：${entry.names.map(name=>'`'+name+'`').join('、')}。

## 在线演示

本页复用官方 Go gallery 的真实示例。下方可以切换场景、主题和深浅色；长页面支持滚动，复杂布局可以横向滚动或在独立页面打开。

<GalleryPreview component="${entry.id}" />

## 示例与源码

下面的 Go 示例由同一个构建流程编译进网页预览。场景中的表单、表格、图表和移动端屏幕使用示例数据；系统文件选择、网络请求等动作以示例中的实现为准。

${sourceLinks.join('\n')}

## 在应用中使用

- [Go GUI 入门与窗口初始化](/guide/ui)
- [完整组件与场景浏览](/components/)
- [Go API 参考](/reference/api/${entry.id.startsWith('editor.')?'ui-editor':entry.group==='图表'?'ui-chart':entry.group==='移动端'?'ui-mobile':'ui'})

本页由完整组件表和实际 gallery 注册信息生成；新增组件未接入演示时，文档构建会报错。
`);
  }
}
