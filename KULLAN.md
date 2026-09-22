# LeadScout 4.0 — Kullanım

## Başlat

Windows: `START_WINDOWS.bat`  
macOS: `START_MAC.command`  
Linux: `./start.sh`

Doğrudan:

```bash
python3 LeadScout.py
```

Arayüz: `http://127.0.0.1:8787`

## İnsan kullanımı

1. Ülke, şehir/bölge, sektör ve yarıçap seç.
2. **Lead bul**.
3. Site durumu, sosyal hesap, skor veya pipeline ile filtrele.
4. Bir leadi açıp iletişim kanallarını ve sosyal hesapları incele.
5. Web sitesi varsa site audit çalıştır.
6. EN / TR / Urduca / Sindhice mesaj taslağı oluştur.
7. Pipeline aşamasını güncelle.
8. Sonuçları **Excel XLSX** veya CSV olarak indir.

Uygulama tarafında artık 250 sonuç sınırı yoktur. Açık veri sağlayıcısının döndürdüğü sonuçlar alınır.

## Arama geçmişi

Son aramalar yerelde tutulur. Üstteki **Geçmişi temizle** düğmesi:
- kayıtlı aramaları,
- aktif arama ID'lerini,
- yerel lead cache'ini,
- pipeline verisini

temizler.

## Ajan kullanımı

REST / OpenAPI:
`http://127.0.0.1:8787/api/v1`

OpenAPI:
`http://127.0.0.1:8787/api/v1/openapi.json`

MCP:

```bash
python -m pip install "mcp>=2,<3"
python mcp_server.py
```

Streamable HTTP:

```bash
python mcp_server.py --transport streamable-http --host 127.0.0.1 --port 8790
```

Ayrıntılar: `docs/AGENTS.md`.

## Gereksinim

Core uygulama için Python 3.11+ yeterlidir. MCP kullanımı için opsiyonel `mcp>=2,<3` paketi gerekir.
