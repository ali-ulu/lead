const $ = id => document.getElementById(id);
const esc = v => String(v ?? '').replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
let selectedId = null;
let leads = [];
let websiteFilter = '';
let draftText = '';

const labels = {
  dentist:'Dentist', clinic:'Clinic', doctor:'Doctor', physiotherapy:'Physiotherapy', veterinary:'Veterinary',
  restaurant:'Restaurant', cafe:'Cafe', hotel:'Hotel / Guest House', beauty:'Beauty Salon', hairdresser:'Hairdresser',
  barber:'Barber', spa:'Spa', real_estate:'Real Estate', accountant:'Accountant', lawyer:'Lawyer', insurance:'Insurance',
  travel_agency:'Travel Agency', car_repair:'Car Repair', car_dealer:'Car Dealer', electrician:'Electrician', plumber:'Plumber',
  photographer:'Photographer', architect:'Architect', gym:'Gym / Fitness', bakery:'Bakery', florist:'Florist'
};

function toast(message, bad=false){
  const node=$('toast');
  node.textContent=message;
  node.className='toast show'+(bad?' bad':'');
  clearTimeout(toast.timer);
  toast.timer=setTimeout(()=>node.className='toast',3200);
}

async function api(url, options={}){
  const response=await fetch(url, options);
  let data={};
  try{data=await response.json()}catch{}
  if(!response.ok) throw new Error(data.error || `Request failed (${response.status})`);
  return data;
}

function filters(){
  const params=new URLSearchParams();
  if(websiteFilter) params.set('website_status',websiteFilter);
  const score=$('min_score').value.trim();
  if(score && score!=='0') params.set('min_score',score);
  const stage=$('pipeline_status').value;
  if(stage) params.set('pipeline_status',stage);
  return params;
}

async function load(){
  const data=await api('/api/leads?'+filters());
  leads=data.items||[];
  renderTable();
}

function websiteLabel(status){
  if(status==='missing') return ['No site found','missing'];
  if(status==='weak') return ['Weak','weak'];
  if(status==='healthy') return ['Healthy','healthy'];
  return ['Not checked','unknown'];
}

function renderTable(){
  const missing=leads.filter(x=>x.website_status==='missing').length;
  const weak=leads.filter(x=>x.website_status==='weak').length;
  const hot=leads.filter(x=>x.lead_score>=60).length;

  $('count').textContent=`${leads.length} leads`;
  $('summary').innerHTML=
    `<span><strong>${leads.length}</strong> shown</span>
     <span><strong class="hot">${hot}</strong> score 60+</span>
     <span><strong>${missing}</strong> no website</span>
     <span><strong>${weak}</strong> weak website</span>`;

  $('emptyResults').classList.toggle('hidden',leads.length>0);

  $('leads').innerHTML=leads.map(x=>{
    const [webText,webClass]=websiteLabel(x.website_status);
    const contacts=[x.phone?'Phone':'',x.email?'Email':'',x.social_url?'Social':''].filter(Boolean);
    return `<tr data-id="${x.id}" tabindex="0" role="button" aria-label="Open ${esc(x.name)}">
      <td class="business-cell">
        <strong>${esc(x.name)}</strong>
        <span>${esc(x.city||'Unknown')}${x.country?`, ${esc(x.country)}`:''} · ${esc(labels[x.category]||x.category||'')}</span>
      </td>
      <td><span class="website-pill ${webClass}">${webText}</span></td>
      <td><div class="contact-set">${contacts.length?contacts.map(c=>`<span class="contact-pill">${c}</span>`).join(''):'<span class="contact-pill none">No contact</span>'}</div></td>
      <td><span class="stage-pill">${esc(x.pipeline_status)}</span></td>
      <td class="score-number ${x.lead_score>=60?'hot':''}">${x.lead_score}</td>
    </tr>`;
  }).join('');

  document.querySelectorAll('#leads tr').forEach(row=>{row.addEventListener('click',()=>openLead(Number(row.dataset.id)));row.addEventListener('keydown',event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();openLead(Number(row.dataset.id))}})});
}

async function discover(){
  const country=$('country').value.trim();
  const city=$('city').value.trim();
  const category=$('category').value;
  const radius_km=Number($('radius_km').value||20);

  if(!city||!category){
    toast('City / area and industry are required.',true);
    return;
  }

  localStorage.setItem('leadHunterSearch',JSON.stringify({country,city,category,radius_km}));

  const btn=$('discover');
  btn.disabled=true;
  btn.classList.add('busy');
  $('areaLabel').textContent='Searching live open data…';

  try{
    const data=await api('/api/discover',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({country,city,category,radius_km})
    });
    $('areaLabel').textContent=`${data.area.display_name} · ${data.count} businesses found`;
    toast(`${data.count} businesses added.`);
    await load();
  }catch(error){
    $('areaLabel').textContent='Search failed. Try again or use the city name in English.';
    toast(error.message,true);
  }finally{
    btn.disabled=false;
    btn.classList.remove('busy');
  }
}

function safeUrl(value){
  if(!value) return '#';
  const text=String(value).trim();
  return /^https?:\/\//i.test(text)?text:`https://${text}`;
}

function openDrawer(){
  $('drawer').setAttribute('aria-hidden','false');
  document.body.style.overflow='hidden';
}

function closeDrawer(){
  $('drawer').setAttribute('aria-hidden','true');
  document.body.style.overflow='';
  selectedId=null;
  draftText='';
}

async function openLead(id){
  selectedId=id;
  draftText='';
  const lead=await api(`/api/leads/${id}`);
  renderDetail(lead);
  openDrawer();
}

function linkButtons(lead){
  const out=[];
  if(lead.website) out.push(`<a target="_blank" rel="noopener" href="${esc(safeUrl(lead.website))}">Website ↗</a>`);
  if(lead.email) out.push(`<a href="mailto:${encodeURIComponent(lead.email)}">Email</a>`);
  if(lead.phone) out.push(`<a href="tel:${esc(lead.phone)}">Call</a>`);
  if(lead.social_url) out.push(`<a target="_blank" rel="noopener" href="${esc(safeUrl(lead.social_url))}">Social ↗</a>`);
  if(lead.latitude&&lead.longitude){
    out.push(`<a target="_blank" rel="noopener" href="https://www.openstreetmap.org/?mlat=${encodeURIComponent(lead.latitude)}&mlon=${encodeURIComponent(lead.longitude)}">Map ↗</a>`);
  }
  return out.join('') || '<span>No direct contact links in source</span>';
}

function renderDetail(lead){
  $('drawerTitle').textContent=lead.name;

  const reasons=(lead.score_reasons||[])
    .map(reason=>`<div class="reason">${esc(reason)}</div>`)
    .join('') || '<p class="muted">No score evidence yet.</p>';

  $('detail').innerHTML=`
    <div class="lead-hero">
      <div>
        <h3>${esc(lead.name)}</h3>
        <div class="location">${esc(lead.city||'')}${lead.country?`, ${esc(lead.country)}`:''} · ${esc(labels[lead.category]||lead.category||'')}</div>
      </div>
      <div class="lead-score">${lead.lead_score}</div>
    </div>
    <div class="quick-links">${linkButtons(lead)}</div>
    <div class="detail-tabs">
      <button class="detail-tab active" data-tab="overview">Overview</button>
      <button class="detail-tab" data-tab="audit">Website</button>
      <button class="detail-tab" data-tab="message">Outreach</button>
    </div>
    <div id="tabContent"></div>`;

  document.querySelectorAll('.detail-tab').forEach(button=>{
    button.addEventListener('click',()=>switchTab(button.dataset.tab,lead,reasons));
  });
  switchTab('overview',lead,reasons);
}

function switchTab(tab,lead,reasons){
  document.querySelectorAll('.detail-tab').forEach(button=>{
    button.classList.toggle('active',button.dataset.tab===tab);
  });

  const node=$('tabContent');

  if(tab==='overview'){
    node.innerHTML=`
      <div class="tab-panel">
        <div class="detail-section">
          <h4>CONTACT & WEB</h4>
          <div class="kv">
            <span>Website</span><strong>${esc(lead.website||'Not found')}</strong>
            <span>Phone</span><strong>${esc(lead.phone||'Unknown')}</strong>
            <span>Email</span><strong>${esc(lead.email||'Unknown')}</strong>
            <span>Status</span><strong>${esc(lead.website_status)}</strong>
          </div>
        </div>
        <div class="detail-section"><h4>WHY THIS SCORE</h4>${reasons}</div>
        <div class="detail-section">
          <h4>PIPELINE</h4>
          <div class="pipeline-row">
            ${['new','reviewed','contacted','replied','proposal','won','lost'].map(stage=>`<button class="stage-btn ${lead.pipeline_status===stage?'active':''}" data-stage="${stage}">${stage}</button>`).join('')}
          </div>
        </div>
        <button id="dnc" class="danger-btn">Do not contact</button>
      </div>`;

    document.querySelectorAll('[data-stage]').forEach(button=>button.addEventListener('click',()=>setStage(lead.id,button.dataset.stage)));
    $('dnc').addEventListener('click',()=>doNotContact(lead.id));
  }

  if(tab==='audit'){
    node.innerHTML=`
      <div class="tab-panel">
        <div class="detail-section">
          <h4>WEBSITE CHECK</h4>
          <div class="audit-card">
            <div class="audit-actions">
              <div>
                <strong>${lead.website?esc(lead.website):'No website in source'}</strong>
                <div class="muted" style="font-size:10px;margin-top:4px">Checks public page signals only.</div>
              </div>
              ${lead.website?'<button id="auditBtn" class="secondary-btn">Run audit</button>':''}
            </div>
            <div id="auditResult"></div>
          </div>
        </div>
      </div>`;

    if($('auditBtn')) $('auditBtn').addEventListener('click',()=>auditLead(lead.id));
  }

  if(tab==='message'){
    node.innerHTML=`
      <div class="tab-panel">
        <div class="detail-section">
          <div class="message-controls">
            <h4>PERSONALIZED DRAFT</h4>
            <div class="lang-row">
              <button class="lang-btn" data-lang="en">EN</button>
              <button class="lang-btn" data-lang="tr">TR</button>
              <button class="lang-btn" data-lang="de">DE</button>
            </div>
          </div>
          <div id="message" class="message-box">Choose a language. Nothing is sent automatically.</div>
          <div class="message-actions">
            <button id="copyMessage" class="secondary-btn" disabled>Copy message</button>
            ${lead.email?'<a class="secondary-btn" id="openEmail" href="#">Open email</a>':''}
          </div>
        </div>
      </div>`;

    document.querySelectorAll('[data-lang]').forEach(button=>{
      button.addEventListener('click',()=>loadMessage(lead,button.dataset.lang,button));
    });
    $('copyMessage').addEventListener('click',copyMessage);
  }
}

async function auditLead(id){
  const btn=$('auditBtn');
  btn.disabled=true;
  btn.textContent='Checking…';

  try{
    const data=await api(`/api/leads/${id}/audit`,{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:'{}'
    });
    const audit=data.audit;

    $('auditResult').innerHTML=audit.reachable
      ? `<div class="audit-flags">${(audit.quality_flags||[]).length
          ? (audit.quality_flags||[]).map(flag=>`<span class="flag">${esc(flag)}</span>`).join('')
          : '<span class="contact-pill">No major heuristic flags</span>'}</div>
         <p class="muted" style="font-size:10px">${audit.response_ms??'—'} ms · ${esc(audit.website_status||'unknown')}</p>`
      : `<p class="muted">Could not reach website: ${esc(audit.error||'Unknown error')}</p>`;

    toast('Website check complete.');
    await load();
  }catch(error){
    toast(error.message,true);
  }finally{
    btn.disabled=false;
    btn.textContent='Run audit';
  }
}

async function loadMessage(lead,lang,button){
  document.querySelectorAll('.lang-btn').forEach(item=>item.classList.remove('active'));
  button.classList.add('active');

  const data=await api(`/api/message?id=${lead.id}&lang=${lang}`);
  draftText=data.message||'';
  $('message').textContent=draftText;
  $('copyMessage').disabled=!draftText;

  if($('openEmail')){
    $('openEmail').href=`mailto:${encodeURIComponent(lead.email||'')}?body=${encodeURIComponent(draftText)}`;
  }
}

async function copyMessage(){
  if(!draftText) return;
  await navigator.clipboard.writeText(draftText);
  toast('Message copied.');
}

async function setStage(id,status){
  const data=await api(`/api/leads/${id}/status`,{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:JSON.stringify({status})
  });
  await load();
  renderDetail(data.lead);
  toast(`Moved to ${status}.`);
}

async function doNotContact(id){
  if(!confirm('Mark this lead do-not-contact and hide it?')) return;
  await api(`/api/leads/${id}/dnc`,{
    method:'POST',
    headers:{'Content-Type':'application/json'},
    body:'{}'
  });
  closeDrawer();
  await load();
  toast('Lead hidden and marked do-not-contact.');
}

function bind(){
  $('discover').addEventListener('click',discover);
  $('export').addEventListener('click',()=>{location.href='/api/export.csv?'+filters()});
  $('min_score').addEventListener('change',load);
  $('pipeline_status').addEventListener('change',load);

  document.querySelectorAll('.filter-chip').forEach(button=>{
    button.addEventListener('click',()=>{
      document.querySelectorAll('.filter-chip').forEach(item=>item.classList.remove('active'));
      button.classList.add('active');
      websiteFilter=button.dataset.filter;
      load();
    });
  });

  document.querySelectorAll('[data-close-drawer]').forEach(node=>node.addEventListener('click',closeDrawer));

  document.addEventListener('keydown',event=>{
    if(event.key==='Escape') closeDrawer();
    if(event.key==='Enter' && (event.target===$('city') || event.target===$('country'))) discover();
  });
}

async function init(){
  const cats=await api('/api/categories');
  $('category').innerHTML=
    '<option value="">Choose industry…</option>'+
    cats.items.map(category=>`<option value="${esc(category)}">${esc(labels[category]||category)}</option>`).join('');

  const saved=JSON.parse(localStorage.getItem('leadHunterSearch')||'{}');
  ['country','city','category','radius_km'].forEach(key=>{
    if(saved[key]!=null && $(key)) $(key).value=saved[key];
  });

  bind();
  await load();
}

init().catch(error=>toast(error.message,true));
