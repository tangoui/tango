// Component groups and public names have one source of truth: AUTHORING §14.
export const catalogGroups = ['通用','布局','导航','数据录入','数据展示','反馈','图表','移动端'];

export function parseComponentTable(authoring) {
  const section = authoring.split('## 14.')[1]?.split(/\n## /)[0];
  if (!section) throw new Error('AUTHORING section 14 is missing');
  const rows=[];
  for (const line of section.split('\n')) {
    if (!line.startsWith('|')) continue;
    const cells=line.split('|').slice(1,-1).map(value=>value.trim());
    if (cells.length!==4 || !catalogGroups.includes(cells[0])) continue;
    const names=[...cells[1].matchAll(/`([A-Za-z][\w./]*)`/g)].flatMap(match=>match[1].split('/'));
    if (names.length) rows.push({group:cells[0],names,comps:cells[1],reference:cells[2],source:cells[3]});
  }
  return rows;
}

// Composite/helper APIs are demonstrated in the real parent scenario, rather
// than by a fabricated screenshot or a fallback Button page.
const aliases = {
  "editor.View":["CodeEditor"],
  Text:['Typography','TypographyEditable'],
  ProLayout:['ProLayoutTop','ShadcnDashboard'],
  CollapsibleSider:['SiderTrigger'], GridRow:['Grid','GridMore'],
  Affix:['Anchor'], Kbd:['Toggle','Command'], Slider:['SliderAdvanced'],
  Avatar:['Badge'], 'chart.Line':['ChartLine'], 'chart.Column':['ChartColumn'],
  'chart.Pie':['ChartPie'], 'chart.Scatter':['ChartScatter'], 'chart.Radar':['ChartRadar'],
  'chart.Gauge':['ChartGauge'], 'chart.Funnel':['ChartFunnel'], 'chart.Heatmap':['ChartHeatmap'],
  'chart.DualAxes':['ChartDualAxes'], 'chart.TinyLine':['ChartTiny'], 'chart.Liquid':['ChartGauge'],
  'mobile.Provider':['MobileNav','MobileApp'], 'mobile.Area':['MobileGesture'],
  'mobile.Scroller':['MobilePull','MobileList'], 'mobile.ScrollView':['MobilePull','MobileList'],
  'mobile.NavBar':['MobileNav'], 'mobile.List':['MobileList'],
  'mobile.Popup':['MobilePopup','MobileActionsheet','MobileDisplay'],
  'mobile.PickerView':['MobileCascade','MobilePicker'], 'mobile.Form':['MobileForm'],
  'mobile.Tabs':['MobileNav','MobileDisplay'], 'mobile.Button':['MobileForm','MobileDisplay','MobileNav'],
  'mobile.IndexBar':['MobileIndexbar'],
};

// componentDocPath returns the documentation page of a catalog entry: the
// hand-written components/<slug>.md when it exists (it then embeds the
// gallery scenes itself), else the generated components/catalog/<slug>.
export function componentDocPath(entry, handWritten) {
  return handWritten.has(entry.slug) ? `/components/${entry.slug}` : `/components/catalog/${entry.slug}`;
}

export function mapComponentCatalog(rows, pages) {
  const demos=pages.filter(page=>!page.variant);
  const known=new Set(demos.map(page=>page.name));
  const missing=[];
  const entryIDs=new Set(rows.map(row=>row.names[0]));
  const borrowed=new Set();
  const components=rows.map(row=>{
    const targets=[...new Set([...(aliases[row.names[0]]??[]),...row.names.filter(name=>known.has(name))])];
    if (!targets.length || targets.some(name=>!known.has(name))) missing.push(row.names[0]);
    for (const page of demos) if (targets.includes(page.parent) && !targets.includes(page.name)) targets.push(page.name);
    // label: the title, up to its first " / ", of the component's own
    // gallery page, else of its first demo unless that demo is another
    // entry's own page (Kbd is shown on Toggle's: it keeps its API name).
    // A borrowed title goes to the first entry (table order) using that
    // demo; later ones keep their API name, so no two entries share a
    // label. The "Mobile · " prefix is dropped: those entries sit in the
    // 移动端 group. Sidebar and overview show it ("Button 按钮",
    // "Text" → "Typography 排版").
    let own=demos.find(page=>page.name===row.names[0]);
    if (!own) {
      own=demos.find(page=>page.name===targets[0]&&!entryIDs.has(page.name)&&!borrowed.has(page.name));
      if (own) borrowed.add(own.name);
    }
    const label=own?.title ? own.title.split(' / ')[0].replace(/^Mobile · /,'') : row.names[0];
    return {...row,id:row.names[0],slug:row.names[0].replaceAll('.','-').toLowerCase(),label,demos:targets};
  });
  if (missing.length) throw new Error(`Components without a registered live demo: ${missing.join(', ')}`);
  return {groups:catalogGroups,components,demos};
}
