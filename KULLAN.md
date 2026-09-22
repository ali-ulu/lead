# LeadScout 5.1 — Kullanım

## Başlat

Windows: `START_WINDOWS.bat`  
macOS: `START_MAC.command`  
Linux: `./start.sh`

Launcher gerekli Python paketlerini yerel `.venv` içine kurar. npm varsa Lighthouse araçlarını da kurmayı dener.

Arayüz: `http://127.0.0.1:8787`

## Normal akış

1. Ülke, şehir/bölge, sektör ve yarıçap seç.
2. **Lead bul**.
3. LeadScout OSM + Overture verilerini birleştirir ve duplicate işletmeleri temizler.
4. Bir lead aç.
5. **Doğrula** ile özellikle "site bulunamadı" sonucunu ikinci kaynaktan kontrol et.
6. Sitesi varsa **İletişimi zenginleştir** ile e-posta, telefon, booking ve sosyal linkleri tara.
7. **Deep audit** ile Lighthouse/heuristic web kalite kontrolünü çalıştır.
8. Mesaj taslağı oluştur.
9. Pipeline ve ayrı **iletişim sonucu** alanını güncelle.
10. Not ve follow-up tarihi ekle.
11. Excel/CSV indir.

## CRM

Pipeline satış aşamasıdır:

`new → reviewed → contacted → replied → proposal → won/lost`

Engagement gerçekten ne olduğudur:

`not_contacted / drafted / sent / delivered / replied / rejected / bounced / no_response`

Böylece "mesaj attık mı, cevap geldi mi, reddetti mi?" kaybolmaz.

## Meta

Facebook ve Instagram ayrı OAuth bağlantıları desteklenir. Tokenlar yerelde şifreli tutulur.

Önemli: bir Instagram/Facebook profil URL'si veya kullanıcı adı tek başına mesaj göndermek için yeterli değildir. Resmî API'nin uygun recipient/conversation ID'si ve gerekli izinler gerekir. LeadScout bunu kontrol eder ve uygun değilse göndermez.

## Ajan

UI'daki **Agent Run** veya REST/MCP ile ajan:

`ara → doğrula → audit/enrich → sırala → mesaj taslağı hazırla → Excel oluştur`

akışını tek görevde yürütebilir.

Otonom gönderim varsayılan kapalıdır. MCP/API gönderim ve yıkıcı işlem izinleri ayrı environment flag'leri ile açılır.

Ayrıntı: `docs/AGENTS.md`.
