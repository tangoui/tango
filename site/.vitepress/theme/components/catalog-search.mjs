// Keep public API names, Chinese scenario titles and group filtering aligned.
export function searchCatalog(catalog, search, group, mode) {
  const sceneGroup=(name,original)=>name.startsWith('Chart')?'图表':name.startsWith('Mobile')?'移动端':original;
  const entries=mode==='components'
    ? catalog.components.map(item=>({
      id:item.id,label:item.names.slice(0,3).join(' · '),group:item.group,
      search:[...item.names,item.reference,...item.demos.map(name=>catalog.demos.find(demo=>demo.name===name)?.title ?? '')].join(' '),
    }))
    : catalog.demos.map(item=>({id:item.name,label:item.title,group:sceneGroup(item.name,item.group),search:item.name+' '+item.title}));
  const query=search.trim().toLowerCase();
  return entries.filter(item=>(group==='全部'||item.group===group)&&`${item.label} ${item.search} ${item.group}`.toLowerCase().includes(query));
}
