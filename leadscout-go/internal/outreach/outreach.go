// Package outreach builds cold-outreach drafts in the supported languages.
package outreach

import "github.com/ali-ulu/lead/leadscout-go/internal/model"

// BuildMessage returns a draft message for the lead in the given language.
func BuildMessage(lead *model.Lead, lang string) string {
	name := lead.Name
	if name == "" {
		name = "there"
	}
	missing := lead.Website == nil || *lead.Website == "" || lead.WebsiteStatus == "missing"
	mobileBad := lead.MobileOK != nil && !*lead.MobileOK

	switch lang {
	case "tr":
		obs := trObservation(missing, mobileBad)
		return "Merhaba " + name + ",\n\n" + obs +
			" Mobil odaklı, hızlı ve iletişim/randevu dönüşümünü öne çıkaran sade bir çözüm hazırlayabilirim. " +
			"İsterseniz önce kısa bir örnek ekran ve öneri listesi paylaşabilirim.\n\nİyi çalışmalar"
	case "ur":
		obs := urObservation(missing, mobileBad)
		return "السلام علیکم " + name + "،\n\n" + obs +
			" میں آپ کے لیے ایک تیز، موبائل فرسٹ حل تیار کر سکتا ہوں جس میں رابطہ اور بکنگ واضح ہو۔ " +
			"اگر مناسب لگے تو پہلے ایک مختصر نمونہ اور بہتری کی فہرست بھیج سکتا ہوں۔\n\nشکریہ"
	case "sd":
		obs := sdObservation(missing, mobileBad)
		return "السلام عليڪم " + name + "،\n\n" + obs +
			" مان توهان لاءِ تيز ۽ موبائل فرسٽ حل تيار ڪري سگهان ٿو، جنهن ۾ رابطو ۽ بڪنگ واضح هجي. " +
			"جيڪڏهن مناسب لڳي ته پهرين مختصر نمونو ۽ بهتريءَ جي فهرست موڪلي سگهان ٿو.\n\nمهرباني"
	case "de":
		obs := deObservation(missing, mobileBad)
		return "Guten Tag " + name + ",\n\n" + obs +
			" Ich kann eine schnelle, mobil ausgerichtete Lösung mit klarer Kontakt- bzw. Terminführung vorbereiten. " +
			"Gern schicke ich Ihnen zuerst einen kurzen Entwurf mit konkreten Verbesserungsvorschlägen.\n\nViele Grüße"
	}
	obs := enObservation(missing, mobileBad)
	return "Hi " + name + ",\n\n" + obs +
		" I can put together a fast, mobile-first improvement focused on a clearer contact or booking path. " +
		"If useful, I can first send a short mockup and a concise list of suggested changes.\n\nBest regards"
}

func trObservation(missing, mobileBad bool) string {
	if missing {
		return "İncelediğim kaynaklarda işletmeniz için bağımsız bir web sitesi göremedim."
	}
	if mobileBad {
		return "Mevcut sitenizin mobil kullanımında iyileştirme fırsatı gördüm."
	}
	return "Mevcut web sitenizde ziyaretçiyi iletişime yönlendiren akışı geliştirme fırsatı gördüm."
}

func urObservation(missing, mobileBad bool) string {
	if missing {
		return "میں نے جن ذرائع کا جائزہ لیا، ان میں آپ کے کاروبار کی کوئی الگ ویب سائٹ نظر نہیں آئی۔"
	}
	if mobileBad {
		return "آپ کی موجودہ ویب سائٹ کے موبائل استعمال میں بہتری کی گنجائش نظر آئی۔"
	}
	return "آپ کی ویب سائٹ پر وزیٹر کو رابطے یا بکنگ تک لے جانے کا راستہ مزید واضح کیا جا سکتا ہے۔"
}

func sdObservation(missing, mobileBad bool) string {
	if missing {
		return "مون جن ذريعن جو جائزو ورتو، انهن ۾ توهان جي ڪاروبار لاءِ الڳ ويب سائيٽ نظر نه آئي."
	}
	if mobileBad {
		return "توهان جي موجوده ويب سائيٽ جي موبائل استعمال ۾ بهتري جي گنجائش نظر آئي."
	}
	return "توهان جي ويب سائيٽ تي رابطو يا بڪنگ ڪرڻ جو رستو وڌيڪ صاف ڪري سگهجي ٿو."
}

func deObservation(missing, mobileBad bool) string {
	if missing {
		return "In den geprüften Quellen konnte ich keine eigenständige Website für Ihr Unternehmen finden."
	}
	if mobileBad {
		return "Bei der mobilen Nutzung Ihrer bestehenden Website sehe ich Verbesserungspotenzial."
	}
	return "Auf Ihrer Website sehe ich Potenzial, den Weg zur Kontaktaufnahme klarer zu gestalten."
}

func enObservation(missing, mobileBad bool) string {
	if missing {
		return "I couldn't find an independent website for your business in the sources I reviewed."
	}
	if mobileBad {
		return "I noticed an opportunity to improve the mobile experience on your current website."
	}
	return "I noticed an opportunity to make the path from website visit to contact clearer."
}
