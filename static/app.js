
const $ = (id) => document.getElementById(id);
const esc = (v) => String(v == null ? "" : v).replace(/[&<>"']/g, (m) => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[m]));

let lang = localStorage.getItem("leadscoutLang") || "en";
let categories = [];
let leads = [];
let currentLead = null;
let currentSearchId = localStorage.getItem("leadscoutActiveSearchId") || "";
let websiteFilter = "";
let socialOnly = false;
let draftText = "";
let metaConnections = [];
let history = JSON.parse(localStorage.getItem("leadscoutHistory") || "[]");

const T = {
  en:{find:"Find leads",searching:"Searching OSM + Overture…",results:"Best leads first.",all:"All",noSite:"No site found",weak:"Weak site",social:"Has social",empty:"No leads yet.",verify:"Verify",enrich:"Enrich contacts",audit:"Deep audit",overview:"Overview",website:"Website",outreach:"Outreach",crm:"CRM",score:"Score",phone:"Phone",email:"Email",map:"Map",copy:"Copy",send:"Send with Meta",recipient:"Recipient ID",metaRule:"Meta APIs only send to eligible conversations; they cannot cold-DM an arbitrary public profile.",engagement:"Engagement",follow:"Follow-up",note:"Note",save:"Save",activity:"Activity",partial:"Partial data: one or more providers timed out.",agentDone:"Agent finished. Excel: ",meta:"Meta connected",history:"Recent searches",clear:"Clear history",confirmClear:"Clear searches, leads and CRM history?",unknown:"Unknown"},
  tr:{find:"Lead bul",searching:"OSM + Overture taranıyor…",results:"En iyi leadler önce.",all:"Tümü",noSite:"Site bulunamadı",weak:"Zayıf site",social:"Sosyal hesabı var",empty:"Henüz lead yok.",verify:"Doğrula",enrich:"İletişimi zenginleştir",audit:"Derin audit",overview:"Genel",website:"Web sitesi",outreach:"Mesaj",crm:"CRM",score:"Skor",phone:"Telefon",email:"E-posta",map:"Harita",copy:"Kopyala",send:"Meta ile gönder",recipient:"Recipient ID",metaRule:"Meta API yalnızca uygun mevcut konuşmalara mesaj gönderebilir; rastgele bir herkese açık profile cold-DM atamaz.",engagement:"İletişim sonucu",follow:"Takip tarihi",note:"Not",save:"Kaydet",activity:"Aktivite",partial:"Kısmi veri: bir veya daha fazla sağlayıcı zaman aşımına uğradı.",agentDone:"Ajan tamamlandı. Excel: ",meta:"Meta bağlı",history:"Son aramalar",clear:"Geçmişi temizle",confirmClear:"Aramaları, leadleri ve CRM geçmişini temizle?",unknown:"Bilinmiyor"},
  ur:{find:"لیڈز تلاش کریں",searching:"OSM + Overture تلاش ہو رہا ہے…",results:"بہترین لیڈز پہلے۔",all:"سب",noSite:"ویب سائٹ نہیں ملی",weak:"کمزور سائٹ",social:"سوشل موجود",empty:"ابھی کوئی لیڈ نہیں۔",verify:"تصدیق",enrich:"رابطہ بہتر کریں",audit:"ویب آڈٹ",overview:"خلاصہ",website:"ویب سائٹ",outreach:"پیغام",crm:"CRM",score:"اسکور",phone:"فون",email:"ای میل",map:"نقشہ",copy:"کاپی",send:"Meta سے بھیجیں",recipient:"Recipient ID",metaRule:"Meta API صرف اہل موجودہ گفتگو میں پیغام بھیج سکتا ہے؛ کسی بھی عوامی پروفائل کو براہ راست cold-DM نہیں بھیج سکتا۔",engagement:"رابطہ حالت",follow:"فالو اپ",note:"نوٹ",save:"محفوظ",activity:"سرگرمی",partial:"کچھ فراہم کنندہ مکمل ڈیٹا نہیں دے سکے۔",agentDone:"ایجنٹ مکمل۔ Excel: ",meta:"Meta منسلک",history:"حالیہ تلاشیں",clear:"ہسٹری صاف کریں",confirmClear:"تمام مقامی تلاش اور CRM ڈیٹا صاف کریں؟",unknown:"نامعلوم"},
  sd:{find:"ليڊ ڳوليو",searching:"OSM + Overture ڳوليو پيو وڃي…",results:"بهترين ليڊ پهرين.",all:"سڀ",noSite:"ويب سائيٽ نه ملي",weak:"ڪمزور سائيٽ",social:"سوشل موجود",empty:"اڃا ليڊ ناهي.",verify:"تصديق",enrich:"رابطا بهتر ڪريو",audit:"ويب آڊٽ",overview:"خلاصو",website:"ويب سائيٽ",outreach:"پيغام",crm:"CRM",score:"اسڪور",phone:"فون",email:"اي ميل",map:"نقشو",copy:"ڪاپي",send:"Meta سان موڪليو",recipient:"Recipient ID",metaRule:"Meta API صرف اهل موجوده گفتگو ڏانهن پيغام موڪلي سگهي ٿي؛ ڪنهن به عوامي پروفائل کي سڌو cold-DM نٿي موڪلي سگهي.",engagement:"رابطو حالت",follow:"فالو اپ",note:"نوٽ",save:"محفوظ",activity:"سرگرمي",partial:"ڪجهه ذريعن مان مڪمل ڊيٽا نه ملي.",agentDone:"ايجنٽ مڪمل. Excel: ",meta:"Meta ڳنڍيل",history:"تازيون ڳولائون",clear:"تاريخ صاف ڪريو",confirmClear:"سڀ مقامي ڳولا ۽ CRM ڊيٽا صاف ڪجي؟",unknown:"اڻڄاتل"}
};

const categoryLabels = {
  dentist:"Dentist",clinic:"Clinic",doctor:"Doctor",physiotherapy:"Physiotherapy",veterinary:"Veterinary",
  restaurant:"Restaurant",cafe:"Cafe",hotel:"Hotel",beauty:"Beauty Salon",hairdresser:"Hairdresser",barber:"Barber",spa:"Spa",
  real_estate:"Real Estate",accountant:"Accountant",lawyer:"Lawyer",insurance:"Insurance",travel_agency:"Travel Agency",
  car_repair:"Car Repair",car_dealer:"Car Dealer",electrician:"Electrician",plumber:"Plumber",photographer:"Photographer",
  architect:"Architect",gym:"Gym",bakery:"Bakery",florist:"Florist"
};

function tr(key){ return (T[lang] && T[lang][key]) || T.en[key] || key; }
function toast(msg,bad){ const n=$("toast"); n.textContent=msg; n.className="toast show"+(bad?" bad":""); clearTimeout(toast._t); toast._t=setTimeout(()=>n.className="toast",3200); }
async function api(url,options){ const r=await fetch(url,options||{}); let d={}; try{d=await r.json()}catch{} if(!r.ok) throw new Error(d.error || ("HTTP "+r.status)); return d; }

function applyLanguage(){
  document.documentElement.lang=lang;
  document.documentElement.dir=(lang==="ur"||lang==="sd")?"rtl":"ltr";
  $("language").value=lang;
  $("discover").querySelector(".buttonText").textContent=tr("find");
  $("clearHistory").textContent=tr("clear");
  document.querySelector('[data-i18n="resultsTitle"]').textContent=tr("results");
  document.querySelector('[data-filter=""]').textContent=tr("all");
  document.querySelector('[data-filter="missing"]').textContent=tr("noSite");
  document.querySelector('[data-filter="weak"]').textContent=tr("weak");
  $("socialFilter").textContent=tr("social");
  renderHistory();
  renderTable();
  if(currentLead) renderLead(currentLead);
}

function filters(){
  const q=new URLSearchParams();
  if(currentSearchId) q.set("search_id",currentSearchId);
  if(websiteFilter) q.set("website_status",websiteFilter);
  if(socialOnly) q.set("has_social","1");
  const min=$("min_score").value; if(min && min!=="0") q.set("min_score",min);
  const stage=$("pipeline_status").value; if(stage) q.set("pipeline_status",stage);
  return q;
}

async function loadLeads(){
  if(!currentSearchId){ leads=[]; renderTable(); return; }
  const d=await api("/api/leads?"+filters().toString());
  leads=d.items||[];
  renderTable();
}

function webState(lead){
  if(lead.website_status==="missing") return [tr("noSite"),"missing"];
  if(lead.website_status==="weak") return [tr("weak"),"weak"];
  if(lead.website_status==="healthy") return ["Healthy","healthy"];
  return ["Not checked","unknown"];
}

function renderTable(){
  const body=$("leads");
  if(!body) return;
  $("count").textContent=leads.length+" leads";
  $("emptyResults").classList.toggle("hidden",leads.length>0);
  const hot=leads.filter(x=>x.lead_score>=60).length;
  const social=leads.filter(x=>Object.keys(x.social_links||{}).length>0).length;
  $("summary").innerHTML='<span class="summary-item"><strong>'+leads.length+'</strong> shown</span><span class="summary-item hot"><strong>'+hot+'</strong> score 60+</span><span class="summary-item"><strong>'+social+'</strong> social</span>';
  body.innerHTML=leads.map((lead)=>{
    const ws=webState(lead);
    const channels=[];
    if(lead.phone) channels.push(tr("phone"));
    if(lead.email) channels.push(tr("email"));
    const sc=Object.keys(lead.social_links||{}).length;
    if(sc) channels.push("Social "+sc);
    return '<tr data-id="'+lead.id+'" tabindex="0">'+
      '<td class="business-cell"><strong>'+esc(lead.name)+'</strong><span>'+esc(lead.city||"")+(lead.country?", "+esc(lead.country):"")+' · '+esc(categoryLabels[lead.category]||lead.category||"")+'</span></td>'+
      '<td><span class="website-pill '+ws[1]+'">'+esc(ws[0])+'</span></td>'+
      '<td><div class="contact-set">'+(channels.length?channels.map(x=>'<span class="contact-pill">'+esc(x)+'</span>').join(""):'<span class="contact-pill none">—</span>')+'</div></td>'+
      '<td><span class="stage-pill">'+esc(lead.pipeline_status||"new")+' / '+esc(lead.engagement_status||"not_contacted")+'</span></td>'+
      '<td class="score-number '+(lead.lead_score>=60?"hot":"")+'">'+lead.lead_score+'</td></tr>';
  }).join("");
  document.querySelectorAll("#leads tr").forEach((row)=>{
    const open=()=>openLead(Number(row.dataset.id));
    row.addEventListener("click",open);
    row.addEventListener("keydown",(e)=>{ if(e.key==="Enter"||e.key===" "){e.preventDefault();open();} });
  });
}

function rememberSearch(input,data){
  const item={country:input.country,city:input.city,category:input.category,radius_km:input.radius_km,search_id:data.search_id,display_name:data.area&&data.area.display_name||input.city,at:Date.now()};
  history=[item].concat(history.filter(x=>!(x.country===item.country&&x.city===item.city&&x.category===item.category&&x.radius_km===item.radius_km))).slice(0,8);
  localStorage.setItem("leadscoutHistory",JSON.stringify(history));
  currentSearchId=data.search_id; localStorage.setItem("leadscoutActiveSearchId",currentSearchId);
  renderHistory();
}

function renderHistory(){
  const n=$("recentSearches");
  if(!history.length){n.classList.add("hidden");n.innerHTML="";return;}
  n.classList.remove("hidden");
  n.innerHTML='<span class="recent-label">'+esc(tr("history"))+'</span>'+history.map((x,i)=>'<button class="recent-chip" data-i="'+i+'">'+esc(x.city)+' · '+esc(categoryLabels[x.category]||x.category)+'</button>').join("");
  n.querySelectorAll("[data-i]").forEach(b=>b.addEventListener("click",()=>{ const x=history[Number(b.dataset.i)]; if(!x)return; $("country").value=x.country||"";$("city").value=x.city||"";$("category").value=x.category||"";$("radius_km").value=String(x.radius_km||20);currentSearchId=x.search_id||"";localStorage.setItem("leadscoutActiveSearchId",currentSearchId);loadLeads(); }));
}

async function discover(){
  const input={country:$("country").value.trim(),city:$("city").value.trim(),category:$("category").value,radius_km:Number($("radius_km").value||20)};
  if(!input.city||!input.category){toast("City / area and industry required",true);return;}
  const btn=$("discover"); btn.disabled=true; btn.classList.add("busy"); $("areaLabel").textContent=tr("searching");
  try{
    const d=await api("/api/discover",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(input)});
    $("areaLabel").textContent=(d.area&&d.area.display_name||input.city)+" · "+d.count+" leads";
    rememberSearch(input,d);
    if(d.partial) toast(tr("partial"),true); else toast(d.count+" leads");
    await loadLeads();
  }catch(e){toast(e.message,true);}
  finally{btn.disabled=false;btn.classList.remove("busy");}
}

function safeUrl(v){ if(!v)return "#"; const s=String(v).trim(); return /^https?:\/\//i.test(s)?s:"https://"+s; }
function quickLinks(lead){
  const out=[];
  if(lead.website) out.push('<a target="_blank" rel="noopener" href="'+esc(safeUrl(lead.website))+'">'+esc(tr("website"))+' ↗</a>');
  if(lead.email) out.push('<a href="mailto:'+encodeURIComponent(lead.email)+'">'+esc(tr("email"))+'</a>');
  if(lead.phone) out.push('<a href="tel:'+esc(lead.phone)+'">'+esc(tr("phone"))+'</a>');
  if(lead.latitude&&lead.longitude) out.push('<a target="_blank" rel="noopener" href="https://www.openstreetmap.org/?mlat='+encodeURIComponent(lead.latitude)+'&mlon='+encodeURIComponent(lead.longitude)+'">'+esc(tr("map"))+' ↗</a>');
  return out.join("")||"<span>—</span>";
}
function socialHtml(lead){
  const names={instagram:"Instagram",facebook:"Facebook",linkedin:"LinkedIn",x:"X",youtube:"YouTube",tiktok:"TikTok",telegram:"Telegram",whatsapp:"WhatsApp"};
  const items=Object.entries(lead.social_links||{}).filter(x=>x[1]);
  if(!items.length)return "";
  return '<div class="social-block"><h4>SOCIAL</h4><div class="social-links">'+items.map(x=>'<a target="_blank" rel="noopener" href="'+esc(safeUrl(x[1]))+'">'+esc(names[x[0]]||x[0])+' ↗</a>').join("")+'</div></div>';
}

function openDrawer(){ $("drawer").setAttribute("aria-hidden","false"); document.body.style.overflow="hidden"; }
function closeDrawer(){ $("drawer").setAttribute("aria-hidden","true"); document.body.style.overflow=""; currentLead=null; draftText=""; }
async function openLead(id){ currentLead=await api("/api/leads/"+id); renderLead(currentLead); openDrawer(); }

function renderLead(lead){
  currentLead=lead;
  $("drawerTitle").textContent=lead.name;
  $("detail").innerHTML='<div class="lead-hero"><div><h3>'+esc(lead.name)+'</h3><div class="location">'+esc(lead.city||"")+(lead.country?", "+esc(lead.country):"")+'</div></div><div class="lead-score">'+lead.lead_score+'</div></div>'+
    '<div class="quick-links">'+quickLinks(lead)+'</div>'+socialHtml(lead)+
    '<div class="detail-tabs"><button class="detail-tab active" data-tab="overview">'+esc(tr("overview"))+'</button><button class="detail-tab" data-tab="audit">'+esc(tr("website"))+'</button><button class="detail-tab" data-tab="message">'+esc(tr("outreach"))+'</button><button class="detail-tab" data-tab="crm">'+esc(tr("crm"))+'</button></div><div id="tabContent"></div>';
  document.querySelectorAll(".detail-tab").forEach(b=>b.addEventListener("click",()=>showTab(b.dataset.tab)));
  showTab("overview");
}

function selectHtml(id,values,current){
  return '<select id="'+id+'">'+values.map(v=>'<option value="'+esc(v)+'" '+(v===current?"selected":"")+'>'+esc(v)+'</option>').join("")+'</select>';
}

function showTab(tab){
  if(!currentLead)return;
  document.querySelectorAll(".detail-tab").forEach(b=>b.classList.toggle("active",b.dataset.tab===tab));
  const lead=currentLead,n=$("tabContent");
  if(tab==="overview"){
    n.innerHTML='<div class="tab-panel"><div class="detail-section"><h4>CONTACT & INTELLIGENCE</h4><div class="kv">'+
      '<span>Website</span><strong>'+esc(lead.website||"Not found")+'</strong><span>'+esc(tr("phone"))+'</span><strong>'+esc(lead.phone||tr("unknown"))+'</strong><span>'+esc(tr("email"))+'</span><strong>'+esc(lead.email||tr("unknown"))+'</strong>'+
      '<span>Verification</span><strong>'+esc(lead.verification_status||"unverified")+'</strong><span>Contactability</span><strong>'+esc(lead.contactability_score||0)+'/100</strong><span>Commercial</span><strong>'+esc(lead.commercial_score||0)+'/100</strong></div>'+
      '<div class="message-actions" style="margin-top:12px"><button id="verifyBtn" class="secondary-btn">'+esc(tr("verify"))+'</button>'+(lead.website?'<button id="enrichBtn" class="secondary-btn">'+esc(tr("enrich"))+'</button>':"")+'</div></div>'+
      '<div class="detail-section"><h4>PIPELINE</h4><div class="pipeline-row">'+["new","reviewed","contacted","replied","proposal","won","lost"].map(s=>'<button class="stage-btn '+(lead.pipeline_status===s?"active":"")+'" data-stage="'+s+'">'+s+'</button>').join("")+'</div></div>'+
      '<button id="dnc" class="danger-btn">Do not contact</button></div>';
    $("verifyBtn").addEventListener("click",()=>verifyLead(lead.id));
    if($("enrichBtn"))$("enrichBtn").addEventListener("click",()=>enrichLead(lead.id));
    document.querySelectorAll("[data-stage]").forEach(b=>b.addEventListener("click",()=>setStage(lead.id,b.dataset.stage)));
    $("dnc").addEventListener("click",()=>doNotContact(lead.id));
  }
  if(tab==="audit"){
    n.innerHTML='<div class="tab-panel"><div class="audit-card"><strong>'+esc(lead.website||"No website")+'</strong><p class="muted">Lighthouse is used when installed; heuristic checks are always available.</p>'+(lead.website?'<button id="auditBtn" class="secondary-btn">'+esc(tr("audit"))+'</button>':"")+'<div id="auditResult"></div></div></div>';
    if($("auditBtn"))$("auditBtn").addEventListener("click",()=>auditLead(lead.id));
  }
  if(tab==="message"){
    n.innerHTML='<div class="tab-panel"><div class="lang-row">'+["en","tr","ur","sd"].map(x=>'<button class="lang-btn" data-lang="'+x+'">'+x.toUpperCase()+'</button>').join("")+'</div><div id="message" class="message-box">Choose language</div><div class="message-actions"><button id="copyMessage" class="secondary-btn" disabled>'+esc(tr("copy"))+'</button></div>'+
      (metaConnections.length?'<div class="audit-card" style="margin-top:14px"><p class="muted" style="font-size:10px">'+esc(tr("metaRule"))+'</p><div class="kv"><span>Provider</span><select id="metaProvider"><option value="instagram">Instagram</option><option value="facebook">Facebook</option></select><span>'+esc(tr("recipient"))+'</span><input id="metaRecipient" placeholder="Meta recipient / conversation id"></div><div id="metaEligibility" class="muted" style="font-size:10px;margin-top:8px"></div><button id="sendMetaBtn" class="primary-btn" style="margin-top:10px">'+esc(tr("send"))+'</button></div>':"")+'</div>';
    document.querySelectorAll("[data-lang]").forEach(b=>b.addEventListener("click",()=>draft(lead.id,b.dataset.lang)));
    $("copyMessage").addEventListener("click",async()=>{if(draftText){await navigator.clipboard.writeText(draftText);toast("Copied");}});
    if($("sendMetaBtn")){
      $("sendMetaBtn").addEventListener("click",()=>sendMeta(lead.id));
      const refreshEligibility=async()=>{
        try{
          const provider=$("metaProvider").value;
          const e=await api("/api/v1/leads/"+lead.id+"/messaging-eligibility?provider="+encodeURIComponent(provider));
          $("metaEligibility").textContent=(e.eligible?"Eligible · ":"Not linked · ")+e.rule;
          if(e.recipient_id&&!$("metaRecipient").value)$("metaRecipient").value=e.recipient_id;
        }catch(err){$("metaEligibility").textContent=err.message;}
      };
      $("metaProvider").addEventListener("change",refreshEligibility);
      refreshEligibility();
    }
  }
  if(tab==="crm"){
    n.innerHTML='<div class="tab-panel"><div class="detail-section"><h4>'+esc(tr("engagement"))+'</h4>'+selectHtml("engagementStatus",["not_contacted","drafted","sent","delivered","replied","rejected","bounced","no_response"],lead.engagement_status||"not_contacted")+'<button id="saveEngagement" class="secondary-btn" style="margin-top:8px">'+esc(tr("save"))+'</button></div>'+
      '<div class="detail-section"><h4>'+esc(tr("follow"))+'</h4><input id="followUpAt" type="datetime-local"><button id="saveFollow" class="secondary-btn" style="margin-top:8px">'+esc(tr("save"))+'</button></div>'+
      '<div class="detail-section"><h4>'+esc(tr("note"))+'</h4><textarea id="leadNote" rows="3" style="width:100%"></textarea><button id="saveNote" class="secondary-btn" style="margin-top:8px">'+esc(tr("save"))+'</button></div>'+
      '<div class="detail-section"><h4>'+esc(tr("activity"))+'</h4><div id="activityList">…</div></div></div>';
    $("saveEngagement").addEventListener("click",()=>saveEngagement(lead.id));
    $("saveFollow").addEventListener("click",()=>saveFollow(lead.id));
    $("saveNote").addEventListener("click",()=>saveNote(lead.id));
    loadActivities(lead.id);
  }
}

async function refreshCurrent(tab){ if(!currentLead)return; currentLead=await api("/api/leads/"+currentLead.id); await loadLeads(); renderLead(currentLead); if(tab)document.querySelector('[data-tab="'+tab+'"]').click(); }
async function verifyLead(id){try{const d=await api("/api/v1/leads/"+id+"/verify",{method:"POST",headers:{"Content-Type":"application/json"},body:"{}"});currentLead=d.lead;await refreshCurrent("overview");toast(d.verified?"Verified":"Verification checked");}catch(e){toast(e.message,true);}}
async function enrichLead(id){try{await api("/api/v1/leads/"+id+"/enrich",{method:"POST",headers:{"Content-Type":"application/json"},body:"{}"});await refreshCurrent("overview");toast("Enriched");}catch(e){toast(e.message,true);}}
async function auditLead(id){try{const d=await api("/api/v1/leads/"+id+"/audit",{method:"POST",headers:{"Content-Type":"application/json"},body:"{}"});currentLead=d.lead;renderLead(currentLead);document.querySelector('[data-tab="audit"]').click();const n=$("auditResult");if(n)n.innerHTML='<p>'+esc(d.audit.audit_engine||"heuristic")+' · perf '+esc(d.audit.performance_score||"—")+' · SEO '+esc(d.audit.seo_score||"—")+' · accessibility '+esc(d.audit.accessibility_score||"—")+'</p>';await loadLeads();}catch(e){toast(e.message,true);}}
async function setStage(id,status){try{const d=await api("/api/v1/leads/"+id+"/status",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({status})});currentLead=d.lead;await refreshCurrent("overview");}catch(e){toast(e.message,true);}}
async function doNotContact(id){if(!confirm("Mark do-not-contact?"))return;try{await api("/api/v1/leads/"+id+"/dnc",{method:"POST",headers:{"Content-Type":"application/json"},body:"{}"});closeDrawer();await loadLeads();}catch(e){toast(e.message,true);}}
async function draft(id,l){try{const d=await api("/api/v1/leads/"+id+"/message?lang="+encodeURIComponent(l));draftText=d.message||"";$("message").textContent=draftText;$("message").dir=(l==="ur"||l==="sd")?"rtl":"ltr";$("copyMessage").disabled=!draftText;}catch(e){toast(e.message,true);}}
async function sendMeta(id){if(!draftText){toast("Draft a message first",true);return;}try{await api("/api/v1/leads/"+id+"/send",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({provider:$("metaProvider").value,recipient_id:$("metaRecipient").value.trim(),text:draftText})});toast("Sent");await refreshCurrent("crm");}catch(e){toast(e.message,true);}}
async function loadActivities(id){try{const d=await api("/api/v1/leads/"+id+"/activities");const n=$("activityList");if(!n)return;n.innerHTML=(d.items||[]).map(x=>'<div class="reason"><strong>'+esc(x.kind)+'</strong> · '+esc(x.channel||x.status||"")+'<br><span class="muted">'+esc(x.body||"")+' '+esc(x.created_at||"")+'</span></div>').join("")||"<span class='muted'>No activity yet.</span>";}catch(e){toast(e.message,true);}}
async function saveEngagement(id){try{await api("/api/v1/leads/"+id+"/engagement",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({status:$("engagementStatus").value})});await refreshCurrent("crm");}catch(e){toast(e.message,true);}}
async function saveFollow(id){try{await api("/api/v1/leads/"+id+"/follow-up",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({when:$("followUpAt").value||null})});await refreshCurrent("crm");}catch(e){toast(e.message,true);}}
async function saveNote(id){const note=$("leadNote").value.trim();if(!note)return;try{await api("/api/v1/leads/"+id+"/notes",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({note})});$("leadNote").value="";loadActivities(id);}catch(e){toast(e.message,true);}}

async function runAgent(){
  const input={city:$("city").value.trim(),country:$("country").value.trim(),category:$("category").value,radius_km:Number($("radius_km").value||20),top_n:30,min_score:35,lang,send:false};
  if(!input.city||!input.category){toast("City / area and industry required",true);return;}
  const b=$("agentRun");b.disabled=true;b.textContent="Agent…";
  try{const d=await api("/api/v1/agent/run",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(input)});if(d.search&&d.search.search_id){currentSearchId=d.search.search_id;localStorage.setItem("leadscoutActiveSearchId",currentSearchId);await loadLeads();}toast(tr("agentDone")+String(d.export_path||""));}
  catch(e){toast(e.message,true);}finally{b.disabled=false;b.textContent="Agent Run";}
}

async function clearHistory(){
  if(!confirm(tr("confirmClear")))return;
  try{await api("/api/clear",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({confirm:true})});history=[];leads=[];currentSearchId="";localStorage.removeItem("leadscoutHistory");localStorage.removeItem("leadscoutActiveSearchId");renderHistory();renderTable();toast("Cleared");}catch(e){toast(e.message,true);}
}

function bind(){
  $("discover").addEventListener("click",discover);
  $("agentRun").addEventListener("click",runAgent);
  $("clearHistory").addEventListener("click",clearHistory);
  $("exportXlsx").addEventListener("click",()=>{if(currentSearchId)location.href="/api/export.xlsx?search_id="+encodeURIComponent(currentSearchId);});
  $("exportCsv").addEventListener("click",()=>{if(currentSearchId)location.href="/api/export.csv?search_id="+encodeURIComponent(currentSearchId);});
  $("min_score").addEventListener("change",loadLeads);
  $("pipeline_status").addEventListener("change",loadLeads);
  $("socialFilter").addEventListener("click",()=>{socialOnly=!socialOnly;$("socialFilter").classList.toggle("active",socialOnly);loadLeads();});
  document.querySelectorAll(".filter-chip[data-filter]").forEach(b=>b.addEventListener("click",()=>{document.querySelectorAll(".filter-chip[data-filter]").forEach(x=>x.classList.remove("active"));b.classList.add("active");websiteFilter=b.dataset.filter;loadLeads();}));
  $("language").addEventListener("change",(e)=>{lang=e.target.value;localStorage.setItem("leadscoutLang",lang);applyLanguage();});
  document.querySelectorAll("[data-close-drawer]").forEach(n=>n.addEventListener("click",closeDrawer));
  document.addEventListener("keydown",(e)=>{if(e.key==="Escape")closeDrawer();});
}

async function init(){
  try{
    const d=await api("/api/categories"); categories=d.items||[];
    $("category").innerHTML='<option value="">Choose industry…</option>'+categories.map(x=>'<option value="'+esc(x)+'">'+esc(categoryLabels[x]||x)+'</option>').join("");
    const meta=await api("/api/v1/oauth/connections");metaConnections=meta.items||[];
    const igCount=metaConnections.filter(x=>x.provider==="instagram").length;
    const fbCount=metaConnections.filter(x=>x.provider==="facebook").length;
    if($("instagramConnect"))$("instagramConnect").textContent="Instagram"+(igCount?" ("+igCount+")":"");
    if($("facebookConnect"))$("facebookConnect").textContent="Facebook"+(fbCount?" ("+fbCount+")":"");
  }catch(e){toast(e.message,true);}
  applyLanguage(); renderHistory(); bind(); await loadLeads();
}
init();
