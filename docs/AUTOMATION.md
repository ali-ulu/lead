# Otomasyon: gece koşusu ve değişim raporu

LeadScout'u her gece kendiliğinden çalıştırıp sabah bir değişim raporu ile
uyanabilirsiniz. Bu belge `scripts/nightly.py` ve `lead_hunter/diff.py`
davranışını ve bir cron kurulumunu anlatır.

## Ne yapar

`scripts/nightly.py` verilen her (şehir, kategori) çifti için:

1. `discover_businesses` ile arama yapar (her arama `search_runs` tablosuna kayıt
   bırakır).
2. Aynı çiftin **önceki** koşusuyla karşılaştırır (`lead_hunter.diff`).
3. Tüm koşuların lead'lerini tek bir XLSX'e aktarır.
4. `exports/` altına üç dosya yazar:
   - `leadscout-nightly.xlsx` — lead'ler (Özet + SektorKarar + Leads sayfaları)
   - `leadscout-nightly.json` — makine okunur rapor
   - `leadscout-nightly.txt` — insan okunur özet

## Değişim raporu (diff)

İki koşu karşılaştırıldığında şunlar raporlanır:

| Alan | Anlamı |
|------|--------|
| `new` / `new_count` | Bu koşuda ilk kez görülen işletmeler |
| `gone` / `gone_count` | Önceki koşuda olup bu koşuda görülmeyenler |
| `website_gained` | Web sitesi sonradan bulunan lead'ler |
| `website_lost` | Web sitesi kaybolan/ölü hale gelen lead'ler |
| `alerts` | Yüksek fırsat: `lead_score >= 70` **ve** web sitesi yok **ve** telefon var |

Web sitesi geçişlerini doğru ölçmek için `search_run_leads` tablosu her koşuda
lead'in `website_status` ve `lead_score` değerlerini **anlık görüntü** olarak
saklar. Böylece lead daha sonra güncellense bile "önce/sonra" karşılaştırması
doğru kalır.

## Kullanım

```bash
# Tek seferlik
python scripts/nightly.py --city Afyonkarahisar --country Turkey \
    --category restaurant --category dentist --radius-km 15

# JSON rapor olarak
python scripts/nightly.py --city Afyonkarahisar --country Turkey \
    --category restaurant --json

# Birden çok işi bir dosyadan oku
python scripts/nightly.py --config nightly.json
```

`nightly.json` biçimi:

```json
{
  "jobs": [
    {"city": "Afyonkarahisar", "country": "Turkey", "category": "restaurant", "radius_km": 15},
    {"city": "Afyonkarahisar", "country": "Turkey", "category": "dentist", "radius_km": 15}
  ]
}
```

İsteğe bağlı: `--cache-ttl 21600` ile yanıt önbelleğini açın (aşağıya bakın).
`--no-export` XLSX üretimini atlar. Komut, işlerden biri hata verirse `1`,
hepsi başarılıysa `0` döner.

## Cron örneği

Her gece 03:00'te çalıştırıp çıktıyı loglayın:

```cron
0 3 * * * cd /path/to/lead && LEADSCOUT_CACHE_TTL=21600 \
    /usr/bin/python3 scripts/nightly.py --config nightly.json \
    >> /var/log/leadscout-nightly.log 2>&1
```

GitHub Actions ile de çalıştırılabilir:

```yaml
on:
  schedule:
    - cron: "0 3 * * *"
jobs:
  nightly:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with: { python-version: "3.11" }
      - run: pip install -e .
      - run: python scripts/nightly.py --config nightly.json
      - uses: actions/upload-artifact@v4
        with:
          name: leadscout-nightly
          path: exports/
```

## Önbellek ve nazik istek (Faz 1)

Gece koşusu public uç noktaları (Nominatim, Overpass, Overture, web arama) çok
kez çağırır. `LEADSCOUT_CACHE_TTL` (saniye) ayarlıysa aynı yanıtlar diskten
döner; ikinci koşu neredeyse anındadır ve paylaşılan uç noktalar yorulmaz.
Ayrıntılar için `.env.example` ve README'ye bakın.
