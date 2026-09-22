from __future__ import annotations
from typing import Any

def build_message(lead: dict[str, Any], lang: str = "en") -> str:
    name = lead.get("name") or "there"
    missing = not lead.get("website") or lead.get("website_status") == "missing"
    mobile_bad = lead.get("mobile_ok") is False

    if lang == "tr":
        obs = (
            "İncelediğim kaynaklarda işletmeniz için bağımsız bir web sitesi göremedim."
            if missing else
            "Mevcut sitenizin mobil kullanımında iyileştirme fırsatı gördüm."
            if mobile_bad else
            "Mevcut web sitenizde ziyaretçiyi iletişime yönlendiren akışı geliştirme fırsatı gördüm."
        )
        return (
            f"Merhaba {name},\n\n{obs} "
            "Mobil odaklı, hızlı ve iletişim/randevu dönüşümünü öne çıkaran sade bir çözüm hazırlayabilirim. "
            "İsterseniz önce kısa bir örnek ekran ve öneri listesi paylaşabilirim.\n\nİyi çalışmalar"
        )

    if lang == "ur":
        obs = (
            "میں نے جن ذرائع کا جائزہ لیا، ان میں آپ کے کاروبار کی کوئی الگ ویب سائٹ نظر نہیں آئی۔"
            if missing else
            "آپ کی موجودہ ویب سائٹ کے موبائل استعمال میں بہتری کی گنجائش نظر آئی۔"
            if mobile_bad else
            "آپ کی ویب سائٹ پر وزیٹر کو رابطے یا بکنگ تک لے جانے کا راستہ مزید واضح کیا جا سکتا ہے۔"
        )
        return (
            f"السلام علیکم {name}،\n\n{obs} "
            "میں آپ کے لیے ایک تیز، موبائل فرسٹ حل تیار کر سکتا ہوں جس میں رابطہ اور بکنگ واضح ہو۔ "
            "اگر مناسب لگے تو پہلے ایک مختصر نمونہ اور بہتری کی فہرست بھیج سکتا ہوں۔\n\nشکریہ"
        )

    if lang == "sd":
        obs = (
            "مون جن ذريعن جو جائزو ورتو، انهن ۾ توهان جي ڪاروبار لاءِ الڳ ويب سائيٽ نظر نه آئي."
            if missing else
            "توهان جي موجوده ويب سائيٽ جي موبائل استعمال ۾ بهتري جي گنجائش نظر آئي."
            if mobile_bad else
            "توهان جي ويب سائيٽ تي رابطو يا بڪنگ ڪرڻ جو رستو وڌيڪ صاف ڪري سگهجي ٿو."
        )
        return (
            f"السلام عليڪم {name}،\n\n{obs} "
            "مان توهان لاءِ تيز ۽ موبائل فرسٽ حل تيار ڪري سگهان ٿو، جنهن ۾ رابطو ۽ بڪنگ واضح هجي. "
            "جيڪڏهن مناسب لڳي ته پهرين مختصر نمونو ۽ بهتريءَ جي فهرست موڪلي سگهان ٿو.\n\nمهرباني"
        )

    if lang == "de":
        obs = (
            "In den geprüften Quellen konnte ich keine eigenständige Website für Ihr Unternehmen finden."
            if missing else
            "Bei der mobilen Nutzung Ihrer bestehenden Website sehe ich Verbesserungspotenzial."
            if mobile_bad else
            "Auf Ihrer Website sehe ich Potenzial, den Weg zur Kontaktaufnahme klarer zu gestalten."
        )
        return (
            f"Guten Tag {name},\n\n{obs} "
            "Ich kann eine schnelle, mobil ausgerichtete Lösung mit klarer Kontakt- bzw. Terminführung vorbereiten. "
            "Gern schicke ich Ihnen zuerst einen kurzen Entwurf mit konkreten Verbesserungsvorschlägen.\n\nViele Grüße"
        )

    obs = (
        "I couldn't find an independent website for your business in the sources I reviewed."
        if missing else
        "I noticed an opportunity to improve the mobile experience on your current website."
        if mobile_bad else
        "I noticed an opportunity to make the path from website visit to contact clearer."
    )
    return (
        f"Hi {name},\n\n{obs} "
        "I can put together a fast, mobile-first improvement focused on a clearer contact or booking path. "
        "If useful, I can first send a short mockup and a concise list of suggested changes.\n\nBest regards"
    )
