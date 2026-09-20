from __future__ import annotations
from typing import Any

def build_message(lead: dict[str, Any], lang: str = "en") -> str:
    name=lead.get("name") or "there"
    missing=not lead.get("website") or lead.get("website_status")=="missing"
    mobile_bad=lead.get("mobile_ok") is False
    if lang=="tr":
        obs="İncelediğim kaynaklarda işletmeniz için bağımsız bir web sitesi göremedim." if missing else ("Mevcut sitenizin mobil kullanımında iyileştirme fırsatı gördüm." if mobile_bad else "Mevcut web sitenizde ziyaretçiyi iletişime yönlendiren akışı geliştirme fırsatı gördüm.")
        return f"Merhaba {name},\n\n{obs} Mobil odaklı, hızlı ve iletişim/randevu dönüşümünü öne çıkaran sade bir çözüm hazırlayabilirim. İsterseniz önce kısa bir örnek ekran ve öneri listesi paylaşabilirim.\n\nİyi çalışmalar"
    if lang=="de":
        obs="In den geprüften Quellen konnte ich keine eigenständige Website für Ihr Unternehmen finden." if missing else ("Bei der mobilen Nutzung Ihrer bestehenden Website sehe ich Verbesserungspotenzial." if mobile_bad else "Auf Ihrer Website sehe ich Potenzial, den Weg zur Kontaktaufnahme klarer zu gestalten.")
        return f"Guten Tag {name},\n\n{obs} Ich kann eine schnelle, mobil ausgerichtete Lösung mit klarer Kontakt- bzw. Terminführung vorbereiten. Gern schicke ich Ihnen zuerst einen kurzen Entwurf mit konkreten Verbesserungsvorschlägen.\n\nViele Grüße"
    obs="I couldn't find an independent website for your business in the sources I reviewed." if missing else ("I noticed an opportunity to improve the mobile experience on your current website." if mobile_bad else "I noticed an opportunity to make the path from website visit to contact clearer.")
    return f"Hi {name},\n\n{obs} I can put together a fast, mobile-first improvement focused on a clearer contact or booking path. If useful, I can first send a short mockup and a concise list of suggested changes.\n\nBest regards"
