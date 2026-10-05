# LeadScout — Geliştirme Roadmap'i

Bu dosya, uygulamayı adım adım daha güvenilir, daha hızlı ve daha üretken
yapmak için sıralı bir plandır. Fazlar sırayla uygulanır; her fazın sonunda
testler yeşil olmalı ve değişiklik PR ile `main`'e inmelidir.

Durum işaretleri: `[ ]` yapılacak · `[~]` devam ediyor · `[x]` tamamlandı.

Kapsam dışı bırakılan şey: gerçek ücretli API anahtarı gerektiren işler
(varsayılan akış anahtarsız çalışmaya devam eder).

---

## Faz 0 — Sağlık kontrolü ve sessiz hataların önlenmesi (en yüksek öncelik)

**Amaç:** "Şehirde işletme yok" ile "sağlayıcıya erişemedim" durumunu
birbirinden ayırmak. Afyon koşusunda `duckdb` kurulu olmadığı için Overture
sessizce 0 döndü ve uygulama bunu "veri yok" gibi gösterdi.

- [x] `doctor` komutu ekle: `python -m lead_hunter.doctor` (ve `leadscout-doctor`).
  - Python sürümü, `duckdb` / `mcp` / `cryptography` / `lighthouse` varlığı.
  - Veritabanı yolu, yazılabilirlik, tablo listesi, lead sayısı.
  - Aktif keşif sağlayıcıları (osm, overture) ve web arama zinciri.
  - Hangi env anahtarları eksik (isim ver, değer gösterme).
  - İnsan-okur çıktı + `--json` makine-okur çıktı; sorun varsa çıkış kodu ≠ 0.
- [x] Keşif sonucunda "sessiz 0" koruması: sağlayıcı devre dışıysa / `partial`
  ise sonuca `degraded: true` ve okunur bir uyarı ekle; bilinmeyen sağlayıcı adı
  da uyarı üretir.
- [x] `/api/health` çıktısına canlı sağlayıcı durumu (`provider_status`),
  `degraded` ve `problems` ekle.
- [ ] Sağlayıcı sağlığı: üst üste N hata veren sağlayıcıyı kısa süre devre dışı
  bırakan basit circuit breaker (Faz 1'e taşındı — önbellek ile birlikte).

**Dosyalar:** `lead_hunter/doctor.py` (yeni), `lead_hunter/services.py`,
`server.py`, `pyproject.toml` (`[project.scripts]`), `tests/test_doctor.py` (yeni),
`README.md`.

**Kabul kriteri:** `duckdb` yokken `doctor` net bir hata verir ve çıkış kodu ≠ 0;
keşif "0" sonucu "degraded" olarak işaretlenir; testler eklenir. ✅ (77 test yeşil)

---

## Faz 1 — Hız ve engel direnci

**Amaç:** Yavaş tile kuyruklarını (40–60 sn) kısaltmak ve seri sorgularda
rate-limit yemeyi azaltmak.

- [x] Önbellek katmanı (TTL'li, disk üstü): Nominatim, Overpass, Overture ve
  web arama yanıtları için. `LEADSCOUT_CACHE_TTL`, `LEADSCOUT_CACHE_DIR`.
- [x] Tile taramasını paralelleştir: `_search_tiled_around_detailed` içinde
  `ThreadPoolExecutor` ile tile'ları eşzamanlı çek (sınırlı eşzamanlılık).
- [x] Global rate limiter + circuit breaker: Nominatim için 1 istek/sn politika;
  sağlayıcı başına token-bucket; üst üste hata sonrası kısa devre dışı bırakma.
- [x] Yeniden başlatılabilir/artımlı tarama: kategori bazlı checkpoint (aynı
  girdi ikinci kez önbellekten döner; `search_runs` kayıtları koşu geçmişini tutar).

**Dosyalar:** `lead_hunter/providers/osm.py`, `lead_hunter/providers/nominatim.py`,
`lead_hunter/providers/overture.py`, `lead_hunter/providers/web_search.py`,
`lead_hunter/cache.py` (yeni), `lead_hunter/ratelimit.py` (yeni),
`tests/test_cache.py`.

**Kabul kriteri:** Aynı şehir ikinci taramada belirgin hızlanır; tile taraması
paralel; Nominatim hızı politika ile sınırlı; testler yeşil.

---

## Faz 2 — Otomasyon ("sabah uyandığımda hazır olsun")

**Amaç:** Ürünün asıl değeri ilk taramada değil, tekrarlı taramadaki farkta.

- [x] Zamanlanmış gece koşusu: seçili şehir/kategorileri tarar, XLSX + JSON/txt
  özet bırakır (`scripts/nightly.py`, cron örneği `docs/AUTOMATION.md`).
- [x] Değişim tespiti (diff): koşular arası "yeni işletme", "web sitesi açan",
  "sitesi ölen", "kapanan" farklarını raporla; `search_runs` üstüne kur.
  `search_run_leads` artık `website_status`/`lead_score` anlık görüntüsü tutar.
- [x] Fırsat alarmı: `lead_score >= 70` ve sitesi yok ve telefon var olan yeni
  lead'ler `alerts` altında işaretlenir.
- [ ] `partial` kalan kategorileri düşük hızda otomatik tamamlayan kuyruk.

**Dosyalar:** `scripts/nightly.py` (yeni), `lead_hunter/diff.py` (yeni),
`lead_hunter/db.py`, `docs/AUTOMATION.md` (yeni), `tests/test_diff.py`.

**Kabul kriteri:** İki koşu arasındaki fark makine-okur ve insan-okur üretilir;
gece koşusu tek komutla çalışır.

---

## Faz 3 — Veri kalitesi ve tekilleştirme

- [x] Telefon normalizasyonu (E.164): `+90`, `0…`, boşluklu/parantezli TR
  formatları `lead_hunter/normalize.py` içinde tek forma iner; `country`
  yoksa TR varsayılır.
- [x] Domain normalizasyonu: `www.`, http/https, izleme parametreleri,
  sondaki `/` temizlenir; tekilleştirme domain bazlı (`normalize_domain`).
- [x] Bulanık isim eşleştirme: OSM ve Overture kayıtlarını trigram/Levenshtein
  + ≤250 m coğrafi yakınlık; jenerik kelimeler (`salon`, `restoran`, `ltd` …)
  eşleştirmede yok sayılır.
- [x] Sektör taksonomisi: `barber`/`hairdresser` alias birleştirmesi; kategoriler
  Türkçe adlarla da eşleşir (`category_key`).

**Dosyalar:** `lead_hunter/merge.py`, `lead_hunter/normalize.py` (yeni),
`lead_hunter/services.py`, `tests/test_normalize.py` (yeni), `tests/test_merge.py` (yeni).

**Kabul kriteri:** Aynı işletme farklı formatlarda tek kayda iner; birim testleri.

**Not (geri doldurma):** Normalizasyon `merge_leads` çıktısına uygulandığı için
`source_id` kanonik hash'i değişir; Faz 3 öncesinde oluşmuş ayrık satırlar
otomatik birleşmez, mevcut veritabanı için yeniden tarama gerekir.

---

## Faz 4 — Güvenlik ve uyum

- [ ] SSRF korumasını genişlet: tüm zenginleştirme/denetim yollarında özel ağ
  (localhost, 169.254.x, 10.x, 172.16–31.x, 192.168.x) ve yönlendirme takibi
  engellensin (mevcut koruma gözden geçirilip kapatılsın).
- [ ] HTTP sunucusu: istek hız sınırı, gövde boyutu sınırı, CORS/Origin kontrolü,
  API token'ı tüm korumalı uçlarda.
- [ ] KVKK/GDPR: veri saklama süresi, silme/opt-out akışı, kaynak atıf
  (OSM/Overture lisansları) dokümanda ve kodda.
- [ ] Site denetiminde robots.txt ve nazik tarama (domain başına politeness,
  kimlikli User-Agent).

**Dosyalar:** `lead_hunter/security.py`, `server.py`, `docs/COMPLIANCE.md`,
`tests/test_security.py`.

**Kabul kriteri:** Güvenlik testleri (SSRF dahil) yeşil; uçlarda token zorunlu.

---

## Faz 5 — Rapor ve ürün özellikleri

- [ ] XLSX zenginleştirme: koşullu biçimlendirme, "Sıcak Lead'ler" ve
  "Sitesi Olmayan Yüksek Fırsat" ayrı sayfalar; CSV/PDF çıktısı.
- [ ] Harita görünümü: tek HTML + Leaflet, lead'leri filtreli göster.
- [ ] Çok dilli outreach: `draft_outreach` için tr/en/ur/sd/de şablonları,
  takip planı ve yanıt takibi.
- [ ] Şehir/bölge toplu tarama: bbox ızgarası ile il/ilçe kapsama, ilçe raporu.
- [ ] Skor açıklanabilirliği: `lead_score` ağırlıkları yapılandırılabilir,
  her skorun "neden"i arayüzde gösterilsin.

**Dosyalar:** `lead_hunter/exporters.py`, `lead_hunter/scoring.py`,
`lead_hunter/outreach.py`, `static/`, `tests/test_exporters.py`.

**Kabul kriteri:** Yeni sayfalar/çıktılar doğrulanır; mevcut testler bozulmaz.

---

## Faz 6 — Geliştirici deneyimi ve test altyapısı

- [ ] Kayıtlı fixture'larla sağlayıcı entegrasyon testleri (VCR benzeri):
  Overpass / Overture / DuckDuckGo / Google yanıtları ağ olmadan test edilsin.
- [ ] `mypy`/`pyright` + coverage eşiği CI'a eklensin.
- [ ] Merkezi config + startup doğrulaması: eksik/yanlış env için net hata;
  `LEADSCOUT_*` şeması tek yerde belgelensin.
- [ ] Yapısal log + metrik: sağlayıcı başına gecikme/hata oranı,
  `/api/v1/stats`, koşu geçmişi paneli.
- [ ] MCP genişletme: toplu işlem (batch search/enrich/export), uzun koşular
  için ilerleme bildirimi, lead listesi kaynağı.

**Dosyalar:** `tests/fixtures/` (yeni), `.github/workflows/ci.yml`,
`lead_hunter/config.py` (yeni), `mcp_server.py`, `docs/AGENTS.md`.

**Kabul kriteri:** CI'da tip kontrolü + coverage eşiği; fixture testleri ağsız geçer.

---

## Uygulama sırası ve ilerleme

| Faz | Konu | Durum |
|-----|------|-------|
| 0 | Sağlık kontrolü + sessiz hata önleme | `[x]` |
| 1 | Hız ve engel direnci | `[x]` |
| 2 | Otomasyon (gece koşusu + diff) | `[x]` |
| 3 | Veri kalitesi ve tekilleştirme | `[x]` |
| 4 | Güvenlik ve uyum | `[ ]` |
| 5 | Rapor ve ürün özellikleri | `[ ]` |
| 6 | Geliştirici deneyimi ve test altyapısı | `[ ]` |

Her faz: kendi dalı → testler yeşil → PR → `main`'e merge. Faz tamamlandığında
bu tablodaki ilgili satır `[x]` yapılır.
