const $ = id => document.getElementById(id);
const esc = v => String(v ?? '').replace(/[&<>"']/g, m => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[m]));
let selectedId = null;
let currentLead = null;
let leads = [];
let websiteFilter = '';
let socialOnly = false;
let draftText = '';
let currentLang = localStorage.getItem('leadscoutLang') || localStorage.getItem('nishanLang') || 'en';
let lastAudit = null;
let activeIds = JSON.parse(localStorage.getItem('leadscoutActiveIds') || localStorage.getItem('nishanActiveIds') || '[]');
let searchHistory = JSON.parse(localStorage.getItem('leadscoutHistory') || localStorage.getItem('nishanHistory') || '[]');

const I18N = {
  en:{
    brandTagline:'Local business scout',language:'Language',exportXlsx:'Excel',exportCsv:'CSV',clearHistory:'Clear history',recentSearches:'Recent searches',confirmClearHistory:'Clear saved searches and all locally cached leads/pipeline data?',historyCleared:'Search history and local lead cache cleared.',noActiveSearch:'Run or reopen a search first.',eyebrow:'FIND THE GAP',
    heroTitle:'Find businesses that are ready for a better web presence.',
    heroBody:'Search any market, spot missing or weak websites, collect contact and social channels, then move the best opportunities into outreach.',
    searchKicker:'SEARCH MARKET',searchTitle:'Where should LeadScout look?',searchHelp:'Choose a city, industry and radius. No API key required.',
    country:'Country',cityArea:'City / area',industry:'Industry',radius:'Radius',findLeads:'Find leads',
    resultsKicker:'OPPORTUNITIES',resultsTitle:'Best leads first.',all:'All',noSite:'No site found',weakSite:'Weak site',hasSocial:'Has social',
    score:'Score',allStages:'All stages',stageNew:'New',stageReviewed:'Reviewed',stageContacted:'Contacted',stageReplied:'Replied',
    stageProposal:'Proposal',stageWon:'Won',stageLost:'Lost',business:'Business',webPresence:'Web presence',channels:'Channels',stage:'Stage',
    emptyTitle:'No leads yet.',emptyBody:'Start with a city and industry above.',privacyTitle:'Local-first by default.',
    privacyBody:'Lead data stays in your local SQLite database. Messages are drafted, never auto-sent.',leadKicker:'LEAD PROFILE',
    chooseIndustry:'Choose industry…',searching:'Searching live open data…',searchFail:'Search failed. Try again or use the city name in English.',
    found:'{n} businesses found',shown:'{n} shown',hot:'{n} score 60+',missing:'{n} no site found',weak:'{n} weak site',social:'{n} with social',
    websiteNotFound:'No site found',websiteWeak:'Weak',websiteHealthy:'Healthy',websiteUnchecked:'Not checked',noContact:'No contact',
    phone:'Phone',email:'Email',socialLabel:'Social',website:'Website',map:'Map',overview:'Overview',websiteTab:'Website',outreach:'Outreach',
    contactWeb:'CONTACT & WEB',status:'Status',whyScore:'WHY THIS SCORE',pipeline:'PIPELINE',doNotContact:'Do not contact',
    websiteCheck:'WEBSITE CHECK',publicSignals:'Checks public page signals only.',runAudit:'Run audit',checking:'Checking…',
    socialChannels:'SOCIAL CHANNELS',noSocial:'No social account found in the available sources yet.',personalizedDraft:'PERSONALIZED DRAFT',
    chooseLanguage:'Choose a language. Nothing is sent automatically.',copyMessage:'Copy message',openEmail:'Open email',
    messageCopied:'Message copied.',auditComplete:'Website check complete.',moved:'Moved to {stage}.',dncDone:'Lead hidden and marked do-not-contact.',
    confirmDnc:'Mark this lead do-not-contact and hide it?',unknown:'Unknown',notFound:'Not found',noEvidence:'No score evidence yet.',
    noDirectLinks:'No direct contact links in source',noFlags:'No major heuristic flags',websiteUnreachable:'Could not reach website',
    searchRequired:'City / area and industry are required.',added:'{n} businesses added.'
  },
  tr:{
    brandTagline:'Yerel işletme avcısı',language:'Dil',exportXlsx:'Excel',exportCsv:'CSV',clearHistory:'Geçmişi temizle',recentSearches:'Son aramalar',confirmClearHistory:'Kayıtlı aramaları ve yerel lead/pipeline verilerini temizlemek istiyor musun?',historyCleared:'Arama geçmişi ve yerel lead cache temizlendi.',noActiveSearch:'Önce bir arama yap veya geçmişten bir arama aç.',eyebrow:'FIRSATI BUL',
    heroTitle:'Daha iyi bir web varlığına hazır işletmeleri bul.',
    heroBody:'İstediğin pazarı tara, sitesi olmayan veya zayıf olan işletmeleri ayır, iletişim ve sosyal kanalları topla, en iyi fırsatları satış akışına taşı.',
    searchKicker:'PAZAR ARA',searchTitle:'LeadScout nerede arasın?',searchHelp:'Şehir, sektör ve yarıçap seç. API anahtarı gerekmez.',
    country:'Ülke',cityArea:'Şehir / bölge',industry:'Sektör',radius:'Yarıçap',findLeads:'Lead bul',
    resultsKicker:'FIRSATLAR',resultsTitle:'En iyi leadler önce.',all:'Tümü',noSite:'Site bulunamadı',weakSite:'Zayıf site',hasSocial:'Sosyal hesabı var',
    score:'Skor',allStages:'Tüm aşamalar',stageNew:'Yeni',stageReviewed:'İncelendi',stageContacted:'İletişim',stageReplied:'Cevap',
    stageProposal:'Teklif',stageWon:'Kazanıldı',stageLost:'Kaybedildi',business:'İşletme',webPresence:'Web varlığı',channels:'Kanallar',stage:'Aşama',
    emptyTitle:'Henüz lead yok.',emptyBody:'Yukarıdan şehir ve sektör seçerek başla.',privacyTitle:'Varsayılan olarak local-first.',
    privacyBody:'Lead verileri yerel SQLite veritabanında kalır. Mesajlar sadece taslak hazırlanır, otomatik gönderilmez.',leadKicker:'LEAD PROFİLİ',
    chooseIndustry:'Sektör seç…',searching:'Canlı açık veri taranıyor…',searchFail:'Arama başarısız. Tekrar dene veya şehir adını İngilizce yaz.',
    found:'{n} işletme bulundu',shown:'{n} gösteriliyor',hot:'{n} skor 60+',missing:'{n} site bulunamadı',weak:'{n} zayıf site',social:'{n} sosyal hesabı var',
    websiteNotFound:'Site bulunamadı',websiteWeak:'Zayıf',websiteHealthy:'Sağlıklı',websiteUnchecked:'Kontrol edilmedi',noContact:'İletişim yok',
    phone:'Telefon',email:'E-posta',socialLabel:'Sosyal',website:'Web sitesi',map:'Harita',overview:'Genel',websiteTab:'Web sitesi',outreach:'Mesaj',
    contactWeb:'İLETİŞİM & WEB',status:'Durum',whyScore:'BU SKOR NEDEN?',pipeline:'SATIŞ AKIŞI',doNotContact:'İletişim kurma',
    websiteCheck:'WEB SİTESİ KONTROLÜ',publicSignals:'Yalnızca herkese açık sayfa sinyalleri kontrol edilir.',runAudit:'Siteyi tara',checking:'Kontrol ediliyor…',
    socialChannels:'SOSYAL KANALLAR',noSocial:'Mevcut kaynaklarda henüz sosyal hesap bulunamadı.',personalizedDraft:'KİŞİSELLEŞTİRİLMİŞ TASLAK',
    chooseLanguage:'Bir dil seç. Hiçbir şey otomatik gönderilmez.',copyMessage:'Mesajı kopyala',openEmail:'E-postayı aç',
    messageCopied:'Mesaj kopyalandı.',auditComplete:'Web sitesi kontrolü tamamlandı.',moved:'{stage} aşamasına taşındı.',dncDone:'Lead gizlendi ve iletişim dışı işaretlendi.',
    confirmDnc:'Bu leadi iletişim dışı işaretleyip gizlemek istiyor musun?',unknown:'Bilinmiyor',notFound:'Bulunamadı',noEvidence:'Henüz skor kanıtı yok.',
    noDirectLinks:'Kaynakta doğrudan iletişim bağlantısı yok',noFlags:'Belirgin sorun sinyali bulunmadı',websiteUnreachable:'Web sitesine ulaşılamadı',
    searchRequired:'Şehir / bölge ve sektör gerekli.',added:'{n} işletme eklendi.'
  },
  ur:{
    brandTagline:'مقامی کاروبار تلاش کریں',language:'زبان',exportXlsx:'Excel',exportCsv:'CSV',clearHistory:'ہسٹری صاف کریں',recentSearches:'حالیہ تلاشیں',confirmClearHistory:'محفوظ تلاشیں اور مقامی لیڈ/پائپ لائن ڈیٹا صاف کریں؟',historyCleared:'تلاش کی ہسٹری اور مقامی لیڈ کیش صاف ہوگئی۔',noActiveSearch:'پہلے نئی تلاش کریں یا حالیہ تلاش کھولیں۔',eyebrow:'موقع تلاش کریں',
    heroTitle:'ایسے کاروبار تلاش کریں جنہیں بہتر ویب موجودگی کی ضرورت ہے۔',
    heroBody:'کسی بھی مارکیٹ میں تلاش کریں، کمزور یا غائب ویب سائٹس دیکھیں، رابطہ اور سوشل چینلز جمع کریں، پھر بہترین مواقع کو آؤٹ ریچ میں لے جائیں۔',
    searchKicker:'مارکیٹ تلاش کریں',searchTitle:'LeadScout کہاں تلاش کرے؟',searchHelp:'شہر، شعبہ اور دائرہ منتخب کریں۔ API key کی ضرورت نہیں۔',
    country:'ملک',cityArea:'شہر / علاقہ',industry:'کاروباری شعبہ',radius:'دائرہ',findLeads:'لیڈز تلاش کریں',
    resultsKicker:'مواقع',resultsTitle:'بہترین لیڈز پہلے۔',all:'سب',noSite:'ویب سائٹ نہیں ملی',weakSite:'کمزور سائٹ',hasSocial:'سوشل موجود',
    score:'اسکور',allStages:'تمام مراحل',stageNew:'نیا',stageReviewed:'جائزہ لیا',stageContacted:'رابطہ کیا',stageReplied:'جواب آیا',
    stageProposal:'پیشکش',stageWon:'کامیاب',stageLost:'ناکام',business:'کاروبار',webPresence:'ویب موجودگی',channels:'رابطے',stage:'مرحلہ',
    emptyTitle:'ابھی کوئی لیڈ نہیں۔',emptyBody:'اوپر شہر اور شعبہ منتخب کر کے شروع کریں۔',privacyTitle:'ڈیٹا مقامی رہتا ہے۔',
    privacyBody:'لیڈ ڈیٹا آپ کے مقامی SQLite ڈیٹابیس میں رہتا ہے۔ پیغامات صرف ڈرافٹ ہوتے ہیں، خودکار نہیں بھیجے جاتے۔',leadKicker:'لیڈ پروفائل',
    chooseIndustry:'شعبہ منتخب کریں…',searching:'لائیو اوپن ڈیٹا تلاش ہو رہا ہے…',searchFail:'تلاش ناکام ہوئی۔ دوبارہ کوشش کریں یا شہر کا نام انگریزی میں لکھیں۔',
    found:'{n} کاروبار ملے',shown:'{n} دکھائے گئے',hot:'{n} اسکور 60+',missing:'{n} ویب سائٹ نہیں ملی',weak:'{n} کمزور سائٹ',social:'{n} سوشل اکاؤنٹس',
    websiteNotFound:'ویب سائٹ نہیں ملی',websiteWeak:'کمزور',websiteHealthy:'اچھی',websiteUnchecked:'چیک نہیں کیا',noContact:'رابطہ نہیں',
    phone:'فون',email:'ای میل',socialLabel:'سوشل',website:'ویب سائٹ',map:'نقشہ',overview:'خلاصہ',websiteTab:'ویب سائٹ',outreach:'پیغام',
    contactWeb:'رابطہ اور ویب',status:'حالت',whyScore:'یہ اسکور کیوں؟',pipeline:'سیلز مرحلہ',doNotContact:'رابطہ نہ کریں',
    websiteCheck:'ویب سائٹ چیک',publicSignals:'صرف عوامی ویب پیج سگنلز چیک کیے جاتے ہیں۔',runAudit:'سائٹ چیک کریں',checking:'چیک ہو رہا ہے…',
    socialChannels:'سوشل چینلز',noSocial:'دستیاب ذرائع میں ابھی کوئی سوشل اکاؤنٹ نہیں ملا۔',personalizedDraft:'ذاتی پیغام',
    chooseLanguage:'زبان منتخب کریں۔ کچھ بھی خودکار نہیں بھیجا جاتا۔',copyMessage:'پیغام کاپی کریں',openEmail:'ای میل کھولیں',
    messageCopied:'پیغام کاپی ہوگیا۔',auditComplete:'ویب سائٹ چیک مکمل۔',moved:'{stage} میں منتقل ہوگیا۔',dncDone:'لیڈ چھپا دی گئی اور رابطہ نہ کریں میں شامل ہوگئی۔',
    confirmDnc:'اس لیڈ کو رابطہ نہ کریں میں شامل کر کے چھپانا ہے؟',unknown:'نامعلوم',notFound:'نہیں ملا',noEvidence:'ابھی اسکور کی وجہ موجود نہیں۔',
    noDirectLinks:'ذریعے میں براہ راست رابطہ لنک نہیں',noFlags:'کوئی بڑا مسئلہ نظر نہیں آیا',websiteUnreachable:'ویب سائٹ تک رسائی نہیں ہوئی',
    searchRequired:'شہر / علاقہ اور شعبہ ضروری ہیں۔',added:'{n} کاروبار شامل ہوئے۔'
  },
  sd:{
    brandTagline:'مقامي ڪاروبار ڳوليو',language:'ٻولي',exportXlsx:'Excel',exportCsv:'CSV',clearHistory:'تاريخ صاف ڪريو',recentSearches:'تازيون ڳولائون',confirmClearHistory:'محفوظ ڳولائون ۽ مقامي ليڊ/پائپ لائن ڊيٽا صاف ڪجي؟',historyCleared:'ڳولا تاريخ ۽ مقامي ليڊ ڪيش صاف ٿي وئي.',noActiveSearch:'پهرين ڳولا ڪريو يا تازو ڳولا کوليو.',eyebrow:'موقعو ڳوليو',
    heroTitle:'اهي ڪاروبار ڳوليو جن کي بهتر ويب موجودگي جي ضرورت آهي.',
    heroBody:'ڪنهن به مارڪيٽ ۾ ڳوليو، ڪمزور يا نه مليل ويب سائيٽون ڏسو، رابطا ۽ سوشل چينل گڏ ڪريو ۽ بهتر موقعن کي آوٽ ريچ ڏانهن وٺي وڃو.',
    searchKicker:'مارڪيٽ ڳوليو',searchTitle:'LeadScout ڪٿي ڳولي؟',searchHelp:'شهر، ڪاروباري شعبي ۽ دائري کي چونڊيو. API key جي ضرورت ناهي.',
    country:'ملڪ',cityArea:'شهر / علائقو',industry:'ڪاروباري شعبو',radius:'دائرو',findLeads:'ليڊ ڳوليو',
    resultsKicker:'موقعا',resultsTitle:'بهترين ليڊ پهرين.',all:'سڀ',noSite:'ويب سائيٽ نه ملي',weakSite:'ڪمزور سائيٽ',hasSocial:'سوشل موجود',
    score:'اسڪور',allStages:'سڀ مرحلا',stageNew:'نئون',stageReviewed:'جائزو ورتو',stageContacted:'رابطو ڪيو',stageReplied:'جواب آيو',
    stageProposal:'آڇ',stageWon:'ڪامياب',stageLost:'ناڪام',business:'ڪاروبار',webPresence:'ويب موجودگي',channels:'رابطا',stage:'مرحلو',
    emptyTitle:'اڃا ليڊ ناهي.',emptyBody:'مٿان شهر ۽ شعبو چونڊي شروع ڪريو.',privacyTitle:'ڊيٽا مقامي رهي ٿي.',
    privacyBody:'ليڊ ڊيٽا توهان جي مقامي SQLite ڊيٽابيس ۾ رهي ٿي. پيغام صرف ڊرافٽ ٿين ٿا، پاڻمرادو نٿا موڪليا وڃن.',leadKicker:'ليڊ پروفائل',
    chooseIndustry:'شعبو چونڊيو…',searching:'لائيو اوپن ڊيٽا ڳوليو پيو وڃي…',searchFail:'ڳولا ناڪام ٿي. ٻيهر ڪوشش ڪريو يا شهر جو نالو انگريزي ۾ لکو.',
    found:'{n} ڪاروبار مليا',shown:'{n} ڏيکاريل',hot:'{n} اسڪور 60+',missing:'{n} سائيٽ نه ملي',weak:'{n} ڪمزور سائيٽ',social:'{n} سوشل اڪائونٽ',
    websiteNotFound:'ويب سائيٽ نه ملي',websiteWeak:'ڪمزور',websiteHealthy:'سٺي',websiteUnchecked:'چيڪ نه ٿيل',noContact:'رابطو ناهي',
    phone:'فون',email:'اي ميل',socialLabel:'سوشل',website:'ويب سائيٽ',map:'نقشو',overview:'خلاصو',websiteTab:'ويب سائيٽ',outreach:'پيغام',
    contactWeb:'رابطو ۽ ويب',status:'حالت',whyScore:'هي اسڪور ڇو؟',pipeline:'سيلز مرحلو',doNotContact:'رابطو نه ڪريو',
    websiteCheck:'ويب سائيٽ چيڪ',publicSignals:'صرف عوامي ويب پيج سگنل چيڪ ٿين ٿا.',runAudit:'سائيٽ چيڪ ڪريو',checking:'چيڪ ٿي رهيو آهي…',
    socialChannels:'سوشل چينل',noSocial:'دستياب ذريعن ۾ اڃا سوشل اڪائونٽ نه مليو.',personalizedDraft:'ذاتي پيغام',
    chooseLanguage:'ٻولي چونڊيو. ڪجھ به پاڻمرادو نه موڪليو ويندو.',copyMessage:'پيغام ڪاپي ڪريو',openEmail:'اي ميل کوليو',
    messageCopied:'پيغام ڪاپي ٿيو.',auditComplete:'ويب سائيٽ چيڪ مڪمل.',moved:'{stage} ڏانهن منتقل ٿيو.',dncDone:'ليڊ لڪائي وئي ۽ رابطو نه ڪريو ۾ شامل ڪئي وئي.',
    confirmDnc:'هن ليڊ کي رابطو نه ڪريو ۾ شامل ڪري لڪايو؟',unknown:'اڻڄاتل',notFound:'نه مليو',noEvidence:'اڃا اسڪور جي وضاحت ناهي.',
    noDirectLinks:'ذريعي ۾ سڌو رابطو لنڪ ناهي',noFlags:'ڪو وڏو مسئلو نظر نه آيو',websiteUnreachable:'ويب سائيٽ تائين رسائي نه ٿي',
    searchRequired:'شهر / علائقو ۽ شعبو ضروري آهن.',added:'{n} ڪاروبار شامل ٿيا.'
  }
};

const CATEGORY_LABELS = {
  dentist:'Dentist',clinic:'Clinic',doctor:'Doctor',physiotherapy:'Physiotherapy',veterinary:'Veterinary',
  restaurant:'Restaurant',cafe:'Cafe',hotel:'Hotel / Guest House',beauty:'Beauty Salon',hairdresser:'Hairdresser',
  barber:'Barber',spa:'Spa',real_estate:'Real Estate',accountant:'Accountant',lawyer:'Lawyer',insurance:'Insurance',
  travel_agency:'Travel Agency',car_repair:'Car Repair',car_dealer:'Car Dealer',electrician:'Electrician',plumber:'Plumber',
  photographer:'Photographer',architect:'Architect',gym:'Gym / Fitness',bakery:'Bakery',florist:'Florist'
};

const STAGE_KEYS = {
  new:'stageNew',reviewed:'stageReviewed',contacted:'stageContacted',replied:'stageReplied',
  proposal:'stageProposal',won:'stageWon',lost:'stageLost'
};

function t(key,vars={}){
  let value=(I18N[currentLang]&&I18N[currentLang][key])||I18N.en[key]||key;
  Object.entries(vars).forEach(([name,replacement])=>{value=value.replaceAll(`{${name}}`,String(replacement));});
  return value;
}

function applyLanguage(lang){
  currentLang=I18N[lang]?lang:'en';
  localStorage.setItem('leadscoutLang',currentLang);
  document.documentElement.lang=currentLang;
  document.documentElement.dir=['ur','sd'].includes(currentLang)?'rtl':'ltr';
  $('language').value=currentLang;
  document.querySelectorAll('[data-i18n]').forEach(node=>{
    const key=node.dataset.i18n;
    if(I18N[currentLang][key]||I18N.en[key]) node.textContent=t(key);
  });
  if($('country')) $('country').placeholder=currentLang==='ur'?'پاکستان':currentLang==='sd'?'پاڪستان':currentLang==='tr'?'Pakistan':'Pakistan';
  if($('city')) $('city').placeholder=currentLang==='tr'?'Karaçi':'Karachi';
  populateCategories();
  renderRecentSearches();
  renderTable();
  if(currentLead) renderDetail(currentLead);
}

function toast(message,bad=false){
  const node=$('toast');
  node.textContent=message;
  node.className='toast show'+(bad?' bad':'');
  clearTimeout(toast.timer);
  toast.timer=setTimeout(()=>node.className='toast',3200);
}

async function api(url,options={}){
  const response=await fetch(url,options);
  let data={};
  try{data=await response.json()}catch{}
  if(!response.ok) throw new Error(data.error||`Request failed (${response.status})`);
  return data;
}

function filters(){
  const params=new URLSearchParams();
  if(activeIds.length) params.set('ids',activeIds.join(','));
  if(websiteFilter) params.set('website_status',websiteFilter);
  if(socialOnly) params.set('has_social','1');
  const score=$('min_score').value.trim();
  if(score&&score!=='0') params.set('min_score',score);
  const stage=$('pipeline_status').value;
  if(stage) params.set('pipeline_status',stage);
  return params;
}

async function load(){
  if(!activeIds.length){
    leads=[];
    renderTable();
    return;
  }
  const data=await api('/api/leads?'+filters());
  leads=data.items||[];
  renderTable();
}

function websiteLabel(status){
  if(status==='missing') return [t('websiteNotFound'),'missing'];
  if(status==='weak') return [t('websiteWeak'),'weak'];
  if(status==='healthy') return [t('websiteHealthy'),'healthy'];
  return [t('websiteUnchecked'),'unknown'];
}

function socialCount(lead){
  return Object.keys(lead.social_links||{}).length;
}

function renderTable(){
  if(!$('leads')) return;
  const missing=leads.filter(x=>x.website_status==='missing').length;
  const weak=leads.filter(x=>x.website_status==='weak').length;
  const hot=leads.filter(x=>x.lead_score>=60).length;
  const socials=leads.filter(x=>socialCount(x)>0).length;

  $('count').textContent=t('found',{n:leads.length});
  $('summary').innerHTML=
    `<span class="summary-item"><strong>${leads.length}</strong> ${esc(t('shown',{n:''}).trim())}</span>
     <span class="summary-item hot"><strong>${hot}</strong> ${esc(t('hot',{n:''}).trim())}</span>
     <span class="summary-item"><strong>${missing}</strong> ${esc(t('missing',{n:''}).trim())}</span>
     <span class="summary-item"><strong>${weak}</strong> ${esc(t('weak',{n:''}).trim())}</span>
     <span class="summary-item"><strong>${socials}</strong> ${esc(t('social',{n:''}).trim())}</span>`;

  $('emptyResults').classList.toggle('hidden',leads.length>0);

  $('leads').innerHTML=leads.map(lead=>{
    const [webText,webClass]=websiteLabel(lead.website_status);
    const channels=[];
    if(lead.phone) channels.push(t('phone'));
    if(lead.email) channels.push(t('email'));
    if(socialCount(lead)) channels.push(`${t('socialLabel')} ${socialCount(lead)}`);
    return `<tr data-id="${lead.id}" tabindex="0" role="button" aria-label="${esc(lead.name)}">
      <td class="business-cell">
        <strong>${esc(lead.name)}</strong>
        <span>${esc(lead.city||t('unknown'))}${lead.country?`, ${esc(lead.country)}`:''} · ${esc(CATEGORY_LABELS[lead.category]||lead.category||'')}</span>
      </td>
      <td><span class="website-pill ${webClass}">${esc(webText)}</span></td>
      <td><div class="contact-set">${channels.length?channels.map((c,i)=>`<span class="contact-pill ${i===channels.length-1&&socialCount(lead)?'social':''}">${esc(c)}</span>`).join(''):`<span class="contact-pill none">${esc(t('noContact'))}</span>`}</div></td>
      <td><span class="stage-pill">${esc(t(STAGE_KEYS[lead.pipeline_status]||lead.pipeline_status))}</span></td>
      <td class="score-number ${lead.lead_score>=60?'hot':''}">${lead.lead_score}</td>
    </tr>`;
  }).join('');

  document.querySelectorAll('#leads tr').forEach(row=>{
    row.addEventListener('click',()=>openLead(Number(row.dataset.id)));
    row.addEventListener('keydown',event=>{
      if(event.key==='Enter'||event.key===' '){
        event.preventDefault();
        openLead(Number(row.dataset.id));
      }
    });
  });
}

function saveHistory(){
  localStorage.setItem('leadscoutHistory',JSON.stringify(searchHistory));
  localStorage.setItem('leadscoutActiveIds',JSON.stringify(activeIds));
}

function renderRecentSearches(){
  const node=$('recentSearches');
  if(!node) return;
  node.classList.toggle('hidden',searchHistory.length===0);
  if(!searchHistory.length){ node.innerHTML=''; return; }
  node.innerHTML=`<span class="recent-label">${esc(t('recentSearches'))}</span>`+
    searchHistory.map((item,index)=>`<button class="recent-chip" data-history-index="${index}">${esc(item.city)} · ${esc(CATEGORY_LABELS[item.category]||item.category)} · ${item.radius_km} km</button>`).join('');
  node.querySelectorAll('[data-history-index]').forEach(button=>button.addEventListener('click',()=>{
    const item=searchHistory[Number(button.dataset.historyIndex)];
    if(!item) return;
    $('country').value=item.country||'';
    $('city').value=item.city||'';
    $('category').value=item.category||'';
    $('radius_km').value=String(item.radius_km||20);
    activeIds=Array.isArray(item.ids)?item.ids:[];
    localStorage.setItem('leadscoutSearch',JSON.stringify({
      country:item.country||'',city:item.city||'',category:item.category||'',radius_km:item.radius_km||20
    }));
    saveHistory();
    $('areaLabel').textContent=item.display_name||`${item.city}, ${item.country}`;
    load();
  }));
}

function rememberSearch(search,data){
  const key=`${search.country.toLowerCase()}|${search.city.toLowerCase()}|${search.category}|${search.radius_km}`;
  const entry={
    key,
    country:search.country,
    city:search.city,
    category:search.category,
    radius_km:search.radius_km,
    ids:Array.isArray(data.ids)?data.ids:[],
    display_name:data.area?.display_name||`${search.city}, ${search.country}`,
    at:Date.now()
  };
  searchHistory=[entry,...searchHistory.filter(item=>item.key!==key)].slice(0,6);
  activeIds=entry.ids;
  saveHistory();
  renderRecentSearches();
}

async function clearHistory(){
  if(!confirm(t('confirmClearHistory'))) return;
  try{
    await api('/api/clear',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});
    activeIds=[];
    searchHistory=[];
    leads=[];
    selectedId=null;
    currentLead=null;
    draftText='';
    lastAudit=null;
    ['leadscoutSearch','leadscoutHistory','leadscoutActiveIds','nishanSearch','nishanHistory','nishanActiveIds','nishanLang','leadHunterSearch'].forEach(key=>localStorage.removeItem(key));
    $('country').value='';
    $('city').value='';
    $('category').value='';
    $('radius_km').value='20';
    $('areaLabel').textContent=t('searchHelp');
    closeDrawer();
    renderRecentSearches();
    renderTable();
    toast(t('historyCleared'));
  }catch(error){
    toast(error.message,true);
  }
}

async function discover(){
  const country=$('country').value.trim();
  const city=$('city').value.trim();
  const category=$('category').value;
  const radius_km=Number($('radius_km').value||20);
  if(!city||!category){toast(t('searchRequired'),true);return;}

  localStorage.setItem('leadscoutSearch',JSON.stringify({country,city,category,radius_km}));
  const btn=$('discover');
  btn.disabled=true;
  btn.classList.add('busy');
  $('areaLabel').textContent=t('searching');

  try{
    const data=await api('/api/discover',{
      method:'POST',
      headers:{'Content-Type':'application/json'},
      body:JSON.stringify({country,city,category,radius_km})
    });
    $('areaLabel').textContent=`${data.area.display_name} · ${t('found',{n:data.count})}`;
    rememberSearch({country,city,category,radius_km},data);
    toast(t('added',{n:data.count}));
    await load();
  }catch(error){
    $('areaLabel').textContent=t('searchFail');
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

function socialLinksHtml(lead){
  const socials=lead.social_links||{};
  const names={instagram:'Instagram',facebook:'Facebook',linkedin:'LinkedIn',x:'X',youtube:'YouTube',tiktok:'TikTok',telegram:'Telegram',whatsapp:'WhatsApp'};
  const items=Object.entries(socials).filter(([,url])=>url);
  if(!items.length) return `<div class="social-block"><h4>${esc(t('socialChannels'))}</h4><span class="muted" style="font-size:10px">${esc(t('noSocial'))}</span></div>`;
  return `<div class="social-block"><h4>${esc(t('socialChannels'))}</h4><div class="social-links">${items.map(([platform,url])=>`<a target="_blank" rel="noopener" href="${esc(safeUrl(url))}">${esc(names[platform]||platform)} ↗</a>`).join('')}</div></div>`;
}

function linkButtons(lead){
  const out=[];
  if(lead.website) out.push(`<a target="_blank" rel="noopener" href="${esc(safeUrl(lead.website))}">${esc(t('website'))} ↗</a>`);
  if(lead.email) out.push(`<a href="mailto:${encodeURIComponent(lead.email)}">${esc(t('email'))}</a>`);
  if(lead.phone) out.push(`<a href="tel:${esc(lead.phone)}">${esc(t('phone'))}</a>`);
  if(lead.latitude&&lead.longitude) out.push(`<a target="_blank" rel="noopener" href="https://www.openstreetmap.org/?mlat=${encodeURIComponent(lead.latitude)}&mlon=${encodeURIComponent(lead.longitude)}">${esc(t('map'))} ↗</a>`);
  return out.join('')||`<span>${esc(t('noDirectLinks'))}</span>`;
}

function openDrawer(){
  $('drawer').setAttribute('aria-hidden','false');
  document.body.style.overflow='hidden';
}

function closeDrawer(){
  $('drawer').setAttribute('aria-hidden','true');
  document.body.style.overflow='';
  selectedId=null;
  currentLead=null;
  draftText='';
  lastAudit=null;
}

async function openLead(id){
  selectedId=id;
  draftText='';
  lastAudit=null;
  currentLead=await api(`/api/leads/${id}`);
  renderDetail(currentLead);
  openDrawer();
}

function scoreReasons(lead){
  return (lead.score_reasons||[]).map(reason=>`<div class="reason">${esc(reason)}</div>`).join('')||`<p class="muted">${esc(t('noEvidence'))}</p>`;
}

function renderDetail(lead){
  currentLead=lead;
  $('drawerTitle').textContent=lead.name;
  $('detail').innerHTML=`
    <div class="lead-hero">
      <div>
        <h3>${esc(lead.name)}</h3>
        <div class="location">${esc(lead.city||'')}${lead.country?`, ${esc(lead.country)}`:''} · ${esc(CATEGORY_LABELS[lead.category]||lead.category||'')}</div>
      </div>
      <div class="lead-score">${lead.lead_score}</div>
    </div>
    <div class="quick-links">${linkButtons(lead)}</div>
    ${socialLinksHtml(lead)}
    <div class="detail-tabs">
      <button class="detail-tab active" data-tab="overview">${esc(t('overview'))}</button>
      <button class="detail-tab" data-tab="audit">${esc(t('websiteTab'))}</button>
      <button class="detail-tab" data-tab="message">${esc(t('outreach'))}</button>
    </div>
    <div id="tabContent"></div>`;

  document.querySelectorAll('.detail-tab').forEach(button=>button.addEventListener('click',()=>switchTab(button.dataset.tab,lead)));
  switchTab('overview',lead);
}

function switchTab(tab,lead){
  document.querySelectorAll('.detail-tab').forEach(button=>button.classList.toggle('active',button.dataset.tab===tab));
  const node=$('tabContent');

  if(tab==='overview'){
    node.innerHTML=`
      <div class="tab-panel">
        <div class="detail-section">
          <h4>${esc(t('contactWeb'))}</h4>
          <div class="kv">
            <span>${esc(t('website'))}</span><strong>${esc(lead.website||t('notFound'))}</strong>
            <span>${esc(t('phone'))}</span><strong>${esc(lead.phone||t('unknown'))}</strong>
            <span>${esc(t('email'))}</span><strong>${esc(lead.email||t('unknown'))}</strong>
            <span>${esc(t('status'))}</span><strong>${esc(websiteLabel(lead.website_status)[0])}</strong>
          </div>
        </div>
        <div class="detail-section"><h4>${esc(t('whyScore'))}</h4>${scoreReasons(lead)}</div>
        <div class="detail-section">
          <h4>${esc(t('pipeline'))}</h4>
          <div class="pipeline-row">${['new','reviewed','contacted','replied','proposal','won','lost'].map(stage=>`<button class="stage-btn ${lead.pipeline_status===stage?'active':''}" data-stage="${stage}">${esc(t(STAGE_KEYS[stage]))}</button>`).join('')}</div>
        </div>
        <button id="dnc" class="danger-btn">${esc(t('doNotContact'))}</button>
      </div>`;
    document.querySelectorAll('[data-stage]').forEach(button=>button.addEventListener('click',()=>setStage(lead.id,button.dataset.stage)));
    $('dnc').addEventListener('click',()=>doNotContact(lead.id));
  }

  if(tab==='audit'){
    node.innerHTML=`
      <div class="tab-panel">
        <div class="detail-section">
          <h4>${esc(t('websiteCheck'))}</h4>
          <div class="audit-card">
            <div class="audit-actions">
              <div>
                <strong>${lead.website?esc(lead.website):esc(t('websiteNotFound'))}</strong>
                <div class="muted" style="font-size:10px;margin-top:4px">${esc(t('publicSignals'))}</div>
              </div>
              ${lead.website?`<button id="auditBtn" class="secondary-btn">${esc(t('runAudit'))}</button>`:''}
            </div>
            <div id="auditResult">${auditResultHtml()}</div>
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
            <h4>${esc(t('personalizedDraft'))}</h4>
            <div class="lang-row">
              <button class="lang-btn" data-lang="en">EN</button>
              <button class="lang-btn" data-lang="tr">TR</button>
              <button class="lang-btn" data-lang="ur">اردو</button>
              <button class="lang-btn" data-lang="sd">سنڌي</button>
            </div>
          </div>
          <div id="message" class="message-box">${esc(t('chooseLanguage'))}</div>
          <div class="message-actions">
            <button id="copyMessage" class="secondary-btn" disabled>${esc(t('copyMessage'))}</button>
            ${lead.email?`<a class="secondary-btn" id="openEmail" href="#">${esc(t('openEmail'))}</a>`:''}
          </div>
        </div>
      </div>`;
    document.querySelectorAll('[data-lang]').forEach(button=>button.addEventListener('click',()=>loadMessage(lead,button.dataset.lang,button)));
    $('copyMessage').addEventListener('click',copyMessage);
  }
}

function auditResultHtml(){
  if(!lastAudit) return '';
  if(!lastAudit.reachable) return `<p class="muted">${esc(t('websiteUnreachable'))}: ${esc(lastAudit.error||t('unknown'))}</p>`;
  const flags=(lastAudit.quality_flags||[]).length
    ? (lastAudit.quality_flags||[]).map(flag=>`<span class="flag">${esc(flag)}</span>`).join('')
    : `<span class="contact-pill">${esc(t('noFlags'))}</span>`;
  return `<div class="audit-flags">${flags}</div><p class="muted" style="font-size:10px">${lastAudit.response_ms??'—'} ms · ${esc(lastAudit.website_status||'unknown')}</p>`;
}

async function auditLead(id){
  const btn=$('auditBtn');
  btn.disabled=true;
  btn.textContent=t('checking');
  try{
    const data=await api(`/api/leads/${id}/audit`,{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});
    lastAudit=data.audit;
    currentLead=data.lead;
    toast(t('auditComplete'));
    await load();
    renderDetail(currentLead);
    document.querySelector('[data-tab="audit"]').click();
  }catch(error){
    toast(error.message,true);
  }
}

async function loadMessage(lead,lang,button){
  document.querySelectorAll('.lang-btn').forEach(item=>item.classList.remove('active'));
  button.classList.add('active');
  const data=await api(`/api/message?id=${lead.id}&lang=${lang}`);
  draftText=data.message||'';
  $('message').textContent=draftText;
  $('message').dir=['ur','sd'].includes(lang)?'rtl':'ltr';
  $('copyMessage').disabled=!draftText;
  if($('openEmail')) $('openEmail').href=`mailto:${encodeURIComponent(lead.email||'')}?body=${encodeURIComponent(draftText)}`;
}

async function copyMessage(){
  if(!draftText) return;
  await navigator.clipboard.writeText(draftText);
  toast(t('messageCopied'));
}

async function setStage(id,status){
  const data=await api(`/api/leads/${id}/status`,{
    method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({status})
  });
  currentLead=data.lead;
  await load();
  renderDetail(currentLead);
  toast(t('moved',{stage:t(STAGE_KEYS[status])}));
}

async function doNotContact(id){
  if(!confirm(t('confirmDnc'))) return;
  await api(`/api/leads/${id}/dnc`,{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});
  closeDrawer();
  await load();
  toast(t('dncDone'));
}

let categories=[];

function populateCategories(){
  if(!$('category')) return;
  const selected=$('category').value;
  $('category').innerHTML=`<option value="">${esc(t('chooseIndustry'))}</option>`+
    categories.map(category=>`<option value="${esc(category)}">${esc(CATEGORY_LABELS[category]||category)}</option>`).join('');
  $('category').value=selected;
}

function bind(){
  $('discover').addEventListener('click',discover);
  $('clearHistory').addEventListener('click',clearHistory);
  $('export').addEventListener('click',()=>{
    if(!activeIds.length){toast(t('noActiveSearch'),true);return;}
    location.href='/api/export.csv?'+filters();
  });
  $('min_score').addEventListener('change',load);
  $('pipeline_status').addEventListener('change',load);
  $('language').addEventListener('change',event=>applyLanguage(event.target.value));

  document.querySelectorAll('.filter-chip[data-filter]').forEach(button=>{
    button.addEventListener('click',()=>{
      document.querySelectorAll('.filter-chip[data-filter]').forEach(item=>item.classList.remove('active'));
      button.classList.add('active');
      websiteFilter=button.dataset.filter;
      load();
    });
  });

  $('socialFilter').addEventListener('click',()=>{
    socialOnly=!socialOnly;
    $('socialFilter').classList.toggle('active',socialOnly);
    load();
  });

  document.querySelectorAll('[data-close-drawer]').forEach(node=>node.addEventListener('click',closeDrawer));

  document.addEventListener('keydown',event=>{
    if(event.key==='Escape') closeDrawer();
    if(event.key==='Enter'&&(event.target===$('city')||event.target===$('country'))) discover();
  });
}

async function init(){
  const data=await api('/api/categories');
  categories=data.items||[];
  applyLanguage(currentLang);

  const saved=JSON.parse(localStorage.getItem('leadscoutSearch')||localStorage.getItem('nishanSearch')||localStorage.getItem('leadHunterSearch')||'{}');
  ['country','city','category','radius_km'].forEach(key=>{
    if(saved[key]!=null&&$(key)) $(key).value=saved[key];
  });

  renderRecentSearches();
  bind();
  await load();
}

init().catch(error=>toast(error.message,true));
