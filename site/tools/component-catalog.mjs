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

export function mapComponentCatalog(rows, pages) {
  const demos=pages.filter(page=>!page.variant);
  const known=new Set(demos.map(page=>page.name));
  const missing=[];
  const components=rows.map(row=>{
    const targets=[...new Set([...(aliases[row.names[0]]??[]),...row.names.filter(name=>known.has(name))])];
    if (!targets.length || targets.some(name=>!known.has(name))) missing.push(row.names[0]);
    for (const page of demos) if (targets.includes(page.parent) && !targets.includes(page.name)) targets.push(page.name);
    return {...row,id:row.names[0],slug:row.names[0].replaceAll('.','-').toLowerCase(),demos:targets};
  });
  if (missing.length) throw new Error(`Components without a registered live demo: ${missing.join(', ')}`);
  return {groups:catalogGroups,components,demos};
}
