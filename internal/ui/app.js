(() => {
  'use strict';
  const paths = {
    pause:'<path d="M9 5v14M15 5v14"/>',play:'<path d="m9 5 10 7-10 7z"/>',settings:'<path d="m9.5 4 .7-2h3.6l.7 2 2 .9 2.1-.3 1.8 3.1-1.3 1.7.1 2.3 1.2 1.8-1.8 3.1-2.2-.3-1.9 1-.7 2h-3.6l-.7-2-2-1-2.1.3-1.8-3.1 1.3-1.8V9.5L3.6 7.7l1.8-3.1 2.2.3z" transform="translate(0 1)"/><circle cx="12" cy="11.5" r="3"/>',
    search:'<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4.5 4.5"/>',close:'<path d="m6 6 12 12M18 6 6 18"/>',text:'<rect x="5" y="3" width="14" height="18" rx="2"/><path d="M9 8h6M9 12h6M9 16h3"/>',link:'<path d="m10 13 4-4m-6 7-1 1a4.3 4.3 0 0 1-6-6l4-4a4.3 4.3 0 0 1 6 0m2 0 1-1a4.3 4.3 0 0 1 6 6l-4 4a4.3 4.3 0 0 1-6 0" transform="translate(1 1)"/>',
    image:'<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8" cy="8" r="1.5"/><path d="m21 15-5-5L5 21"/>',files:'<path d="M3 7a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',data:'<rect x="4" y="4" width="16" height="16" rx="3"/><path d="M8 9h8M8 13h8M8 17h4"/>',copy:'<rect x="8" y="8" width="12" height="13" rx="2"/><path d="M15 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h3"/>',
    pin:'<path d="m15 3 6 6-3 1-4 4-1 4-3-3-6 6m4-8L5 10l4-1 4-4z"/>',trash:'<path d="M3 6h18M9 6V3h6v3M6 6l1 15h10l1-15M10 10v7M14 10v7"/>',info:'<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7v.1"/>',shield:'<path d="m12 2 8 3v6c0 5-8 11-8 11S4 16 4 11V5z"/><path d="m8 12 3 3 5-6"/>',stack:'<rect x="8" y="3" width="13" height="15" rx="3"/><rect x="3" y="7" width="13" height="15" rx="3"/>',check:'<path d="m5 12 4 4L19 6"/>'
  };
  const icon = name => `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[name] || paths.data}</svg>`;
  document.querySelectorAll('[data-icon]').forEach(el => { el.innerHTML = icon(el.dataset.icon); });
  const $ = id => document.getElementById(id);
  let filter = 'all', limit = 200, entries = [], selected = 0, request = 0, searchTimer, toastTimer, current = {}, detailId, busy = false;
  const types = {text:'Texte',link:'Lien',image:'Image',files:'Fichiers',data:'Données'};
  const clock = iso => new Intl.DateTimeFormat('fr-FR',{hour:'2-digit',minute:'2-digit'}).format(new Date(iso));
  const date = iso => new Intl.DateTimeFormat('fr-FR',{day:'numeric',month:'short'}).format(new Date(iso));
  const size = bytes => bytes >= 1048576 ? `${(bytes/1048576).toFixed(1)} Mo` : `${Math.max(1,Math.ceil(bytes/1024))} Ko`;
  function toast(message,error=false) { clearTimeout(toastTimer); $('toast').textContent=String(message);$('toast').classList.toggle('error',error);$('toast').hidden=false;toastTimer=setTimeout(()=>{$('toast').hidden=true;},4000); }
  async function run(action) { try { return await action(); } catch(error) { toast(error?.message || error,true);return undefined; } }
  function empty(title,body) { const box=document.createElement('div');box.className='empty';const symbol=document.createElement('div');symbol.className='empty-symbol';symbol.innerHTML=icon('stack');const heading=document.createElement('h3');heading.textContent=title;const p=document.createElement('p');p.textContent=body;box.append(symbol,heading,p);$('history').replaceChildren(box); }
  async function refresh() {
    const token=++request;
    try {
      const state=await window.kolState($('search').value,filter,limit);
      if(token!==request)return;
      current=state;entries=state.entries||[];selected=Math.min(selected,Math.max(entries.length-1,0));
      $('count').textContent=`${state.matched} élément${state.matched===1?'':'s'}`;
      const today=new Date().toDateString();
      $('list-title').textContent=$('search').value?'RÉSULTATS':(entries.length&&new Date(entries[0].lastAt).toDateString()===today?'AUJOURD’HUI':'VOS COPIES');
      $('status-text').textContent=state.paused?'Capture en pause':'Capture active';
      document.body.classList.toggle('paused',state.paused);
      $('pause').innerHTML=icon(state.paused?'play':'pause');
      $('pause').classList.toggle('active',state.paused);
      $('pause').setAttribute('aria-label',state.paused?'Reprendre la capture':'Mettre la capture en pause');
      $('pause').title=$('pause').getAttribute('aria-label');
      $('notice').hidden=!state.lastError;$('notice').textContent=state.lastError||'';
      $('history').setAttribute('aria-busy','false');
      render();
    } catch(error) { if(token!==request)return;empty('Historique indisponible','Réessayez dans un instant.');$('count').textContent='';$('history').setAttribute('aria-busy','false');toast(error?.message||error,true); }
  }
  function render() {
    const list=$('history'),scroll=list.scrollTop;
    if(!entries.length){empty($('search').value?'Rien par ici.':filter==='pinned'?'Gardez l’essentiel.':filter!=='all'?'Pas encore de copie ici.':'Vos copies ont de la mémoire.', $('search').value?'Essayez un autre mot ou un autre filtre.':filter==='pinned'?'Épinglez une copie pour la retrouver facilement.':'Copiez un texte, une image ou un fichier. KOL s’occupe de la suite.');return;}
    const fragment=document.createDocumentFragment();
    entries.forEach((entry,index)=>{
      const row=document.createElement('article');row.className='entry'+(index===selected?' selected':'');row.dataset.id=entry.id;
      const main=document.createElement('button');main.className='item-copy';main.type='button';main.setAttribute('aria-label',`Recopier : ${entry.title}`);
      const thumb=document.createElement('span');thumb.className='thumb';
      thumb.innerHTML=icon(entry.kind);
      const content=document.createElement('span');content.className='copy-content';const title=document.createElement('span');title.className='copy-title';title.textContent=entry.title;
      content.append(title);
      main.append(thumb,content);
      if(entry.kind==='image'&&/^data:image\/png;base64,/.test(entry.preview||'')){
        row.classList.add('image-entry'); main.classList.add('image-copy'); thumb.innerHTML=icon('image');thumb.classList.remove('image');
        const heading=document.createElement('span');heading.className='image-heading';heading.append(thumb,content);
        const image=document.createElement('img');image.className='image-preview';image.src=entry.preview;image.alt='Aperçu de l’image copiée';
        main.replaceChildren(heading,image);
      }
      main.addEventListener('click',()=>copy(entry.id));
      main.addEventListener('contextmenu',event=>{event.preventDefault();showDetail(entry.id);});
      main.setAttribute('aria-description','Clic droit pour les informations de la copie');
      const actions=document.createElement('div');actions.className='row-actions';const time=document.createElement('span');time.className='row-time';time.textContent=new Date(entry.lastAt).toDateString()===new Date().toDateString()?clock(entry.lastAt):date(entry.lastAt);time.title=new Date(entry.lastAt).toLocaleString('fr-FR');
      const button=(name,label,fn)=>{const b=document.createElement('button');b.className='row-button '+name;b.type='button';b.innerHTML=icon(name==='delete'?'trash':name);b.setAttribute('aria-label',label);b.title=label;b.addEventListener('click',fn);return b;};
      const pin=button('pin',entry.pinned?'Désépingler':'Épingler',()=>run(async()=>{await window.kolPin(entry.id,!entry.pinned);await refresh();}));pin.classList.toggle('pinned',entry.pinned);pin.setAttribute('aria-pressed',String(entry.pinned));
      actions.append(button('delete','Supprimer cette copie',()=>run(async()=>{await window.kolDelete(entry.id);await refresh();})),pin,time);
      row.append(main,actions);row.addEventListener('mouseenter',()=>{selected=index;highlight(false);});fragment.append(row);
    });
    if(entries.length<current.matched){const more=document.createElement('button');more.className='load-more';more.textContent='Afficher la suite';more.addEventListener('click',()=>{limit+=200;refresh();});fragment.append(more);}
    list.replaceChildren(fragment);list.scrollTop=scroll;
  }
  function highlight(scroll=true){document.querySelectorAll('.entry').forEach((row,index)=>{row.classList.toggle('selected',index===selected);if(scroll&&index===selected)row.scrollIntoView({block:'nearest'});});}
  async function copy(id){if(busy)return;busy=true;await run(async()=>{await window.kolCopy(id);toast('Copie prête. Collez avec Ctrl + V.');});busy=false;}
  async function showDetail(id){await run(async()=>{const detail=await window.kolDetail(id);detailId=id;$('detail-text').textContent=detail.text||detail.files?.join('\n')||'Contenu binaire — conservé dans ses formats d’origine.';$('detail-meta').textContent=[types[detail.kind],size(detail.size),detail.source,(detail.formatNames||[]).join(', ')].filter(Boolean).join(' · ');$('detail-omitted').hidden=!detail.omitted?.length;$('detail-omitted').textContent=(detail.omitted||[]).join('\n');$('detail-dialog').showModal();});}
  $('search').addEventListener('input',()=>{clearTimeout(searchTimer);searchTimer=setTimeout(()=>{selected=0;limit=200;refresh();},100);});
  document.querySelectorAll('[data-filter]').forEach(button=>button.addEventListener('click',()=>{filter=button.dataset.filter;selected=0;limit=200;document.querySelectorAll('[data-filter]').forEach(b=>b.setAttribute('aria-pressed',String(b===button)));refresh();}));
  $('pause').addEventListener('click',()=>run(async()=>{await window.kolPause(!current.paused);await refresh();}));
  $('settings-open').addEventListener('click',()=>{$('startup').checked=!!current.settings?.startAtLogin;const max=String(current.settings?.maxItemMiB||64);if(![...$('max-size').options].some(o=>o.value===max))$('max-size').add(new Option(`${max} Mo`,max));$('max-size').value=max;$('version').textContent=`KOL ${current.version||''}`;$('settings-dialog').showModal();});
  document.querySelectorAll('[data-close]').forEach(b=>b.addEventListener('click',()=>$(b.dataset.close).close()));
  document.querySelectorAll('dialog').forEach(d=>d.addEventListener('click',event=>{const r=d.getBoundingClientRect();if(event.target===d&&(event.clientX<r.left||event.clientX>r.right||event.clientY<r.top||event.clientY>r.bottom))d.close();}));
  $('settings-save').addEventListener('click',()=>run(async()=>{await window.kolSettings({startAtLogin:$('startup').checked,maxItemMiB:Number($('max-size').value)});$('settings-dialog').close();await refresh();toast('Préférences enregistrées.');}));
  $('clear-open').addEventListener('click',()=>$('clear-dialog').showModal());
  $('clear-confirm').addEventListener('click',()=>run(async()=>{await window.kolClear();$('clear-dialog').close();$('settings-dialog').close();await refresh();toast('Historique effacé. Les épingles restent.');}));
  $('quit').addEventListener('click',()=>run(()=>window.kolQuit()));
  $('detail-copy').addEventListener('click',()=>copy(detailId));
  document.addEventListener('keydown',event=>{
    if(event.ctrlKey&&event.key.toLowerCase()==='f'){event.preventDefault();$('search').focus();$('search').select();return;}
    if(document.querySelector('dialog[open]'))return;
    if(event.key==='Escape'){event.preventDefault();run(()=>window.kolHide());return;}
    if(event.key==='ArrowDown'||event.key==='ArrowUp'){event.preventDefault();selected=Math.max(0,Math.min(entries.length-1,selected+(event.key==='ArrowDown'?1:-1)));highlight();return;}
    if(event.key==='Enter'&&(event.target===$('search')||event.target===document.body)&&entries[selected]){event.preventDefault();copy(entries[selected].id);}
  });
  window.addEventListener('kol:changed',refresh);
  window.addEventListener('kol:shown',()=>{document.querySelectorAll('dialog[open]').forEach(d=>d.close());refresh();$('search').focus();});
  if(typeof window.kolState!=='function'){empty('Ouvrez KOL sur Windows.','Ce panneau fonctionne à l’intérieur de l’application.');$('count').textContent='';$('history').setAttribute('aria-busy','false');return;}
  refresh().then(()=>run(()=>window.kolReady()));
})();
