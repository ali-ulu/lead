// Package normalize provides canonical forms for phone numbers, websites,
// names and categories so duplicates collapse before comparison and storage.
package normalize

import (
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var trRegions = map[string]bool{"tr": true, "tur": true, "turkey": true, "türkiye": true, "turkiye": true}

var nonDigit = regexp.MustCompile(`\D`)

var trackingParams = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true, "utm_term": true,
	"utm_content": true, "gclid": true, "fbclid": true, "yclid": true, "mc_cid": true,
	"mc_eid": true, "igshid": true, "ref": true, "ref_src": true,
}

// NormalizePhone returns an E.164-ish phone string, or "" when there are no digits.
func NormalizePhone(value string, region string) string {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ""
	}
	hasPlus := strings.HasPrefix(raw, "+")
	digits := nonDigit.ReplaceAllString(raw, "")
	if digits == "" {
		return ""
	}
	if hasPlus {
		return "+" + digits
	}
	if strings.HasPrefix(digits, "00") {
		return "+" + digits[2:]
	}
	if trRegions[strings.ToLower(strings.TrimSpace(region))] {
		if len(digits) == 12 && strings.HasPrefix(digits, "90") {
			return "+" + digits
		}
		if len(digits) == 11 && strings.HasPrefix(digits, "0") {
			return "+90" + digits[1:]
		}
		if len(digits) == 10 && strings.HasPrefix(digits, "5") {
			return "+90" + digits
		}
	}
	return digits
}

// PhoneKey returns a digits-only key for equality checks.
func PhoneKey(value string, region string) string {
	normalized := NormalizePhone(value, region)
	if normalized == "" {
		return ""
	}
	digits := nonDigit.ReplaceAllString(normalized, "")
	if trRegions[strings.ToLower(strings.TrimSpace(region))] && len(digits) == 12 && strings.HasPrefix(digits, "90") {
		return digits[2:]
	}
	return digits
}

// NormalizeDomain returns the bare host, lowercased, without a leading www.
func NormalizeDomain(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	text := strings.TrimSpace(value)
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	u, err := url.Parse(text)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	return strings.TrimPrefix(host, "www.")
}

var trFold = strings.NewReplacer(
	"ı", "i", "İ", "i", "ş", "s", "ğ", "g", "ü", "u",
	"ö", "o", "ç", "c", "â", "a", "î", "i", "û", "u",
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// NormalizeName lowercases alphanumerics and folds Turkish diacritics to ASCII.
func NormalizeName(value string) string {
	if value == "" {
		return ""
	}
	text := strings.ToLower(strings.TrimSpace(value))
	text = trFold.Replace(text)
	return nonAlnum.ReplaceAllString(text, "")
}

// NormalizeWebsite keeps the scheme, drops www., tracking params and trailing slash.
func NormalizeWebsite(value string) string {
	if value == "" {
		return ""
	}
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	if !strings.Contains(text, "://") {
		text = "https://" + text
	}
	u, err := url.Parse(text)
	if err != nil {
		return strings.TrimSpace(value)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return strings.TrimSpace(value)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" {
		return strings.TrimSpace(value)
	}
	host = strings.TrimPrefix(host, "www.")

	path := u.Path
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimRight(path, "/")
	}
	if path == "/" {
		path = ""
	}

	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		if !trackingParams[strings.ToLower(k)] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var pairs []string
	for _, k := range keys {
		for _, v := range q[k] {
			pairs = append(pairs, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	query := strings.Join(pairs, "&")

	netloc := host
	if u.Port() != "" && u.Port() != "80" && u.Port() != "443" {
		netloc = host + ":" + u.Port()
	}
	out := u.Scheme + "://" + netloc + path
	if query != "" {
		out += "?" + query
	}
	return out
}

func trigrams(value string) map[string]bool {
	padded := "  " + value + " "
	out := map[string]bool{}
	for i := 0; i+3 <= len(padded); i++ {
		out[padded[i:i+3]] = true
	}
	return out
}

var genericTokens = map[string]bool{
	"salon": true, "salonu": true, "salonlar": true, "restaurant": true, "restoran": true,
	"lokanta": true, "cafe": true, "kafe": true, "hotel": true, "otel": true, "market": true,
	"magaza": true, "store": true, "shop": true, "center": true, "merkez": true, "merkezi": true,
	"clinic": true, "klinik": true, "poliklinik": true, "ltd": true, "sti": true, "limited": true,
	"as": true, "inc": true, "co": true, "company": true, "the": true,
}

var wordSplit = regexp.MustCompile(`[^0-9A-Za-zÀ-ÿĞğİıŞşÇçÖöÜüÂâÎîÛû]+`)

func coreName(value string) string {
	if value == "" {
		return ""
	}
	words := wordSplit.Split(value, -1)
	var kept []string
	for _, w := range words {
		if w != "" && !genericTokens[NormalizeName(w)] {
			kept = append(kept, w)
		}
	}
	core := NormalizeName(strings.Join(kept, " "))
	if core == "" {
		return NormalizeName(value)
	}
	return core
}

// NameSimilarity is a 0..1 blend of character ratio and trigram Jaccard.
func NameSimilarity(a, b string) float64 {
	na, nb := coreName(a), coreName(b)
	if na == "" || nb == "" {
		return 0
	}
	if na == nb {
		return 1
	}
	ratio := sequenceRatio(na, nb)
	ta, tb := trigrams(na), trigrams(nb)
	inter := 0
	union := map[string]bool{}
	for k := range ta {
		union[k] = true
	}
	for k := range tb {
		union[k] = true
		if ta[k] {
			inter++
		}
	}
	jaccard := 0.0
	if len(union) > 0 {
		jaccard = float64(inter) / float64(len(union))
	}
	if ratio > jaccard {
		return ratio
	}
	return jaccard
}

// sequenceRatio approximates difflib.SequenceMatcher.ratio using the number of
// matching characters from the longest matching blocks (Ratcliff/Obershelp).
func sequenceRatio(a, b string) float64 {
	if a == "" && b == "" {
		return 1
	}
	matches := countMatches([]rune(a), []rune(b))
	return 2 * float64(matches) / float64(len([]rune(a))+len([]rune(b)))
}

func countMatches(a, b []rune) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// find longest matching block
	bestI, bestJ, bestSize := 0, 0, 0
	// dp over suffixes
	prev := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
				if cur[j] > bestSize {
					bestSize = cur[j]
					bestI = i
					bestJ = j
				}
			}
		}
		prev = cur
	}
	if bestSize == 0 {
		return 0
	}
	return bestSize +
		countMatches(a[:bestI-bestSize], b[:bestJ-bestSize]) +
		countMatches(a[bestI:], b[bestJ:])
}

// CategoryAliases maps a canonical key to its English + Turkish aliases.
var CategoryAliases = map[string][]string{
	"barber":       {"barber", "berber", "hairdresser", "kuaför", "kuafor", "erkek kuafor"},
	"beauty":       {"beauty", "güzellik", "guzellik", "beauty_salon", "estetik"},
	"dentist":      {"dentist", "dental_clinic", "dişçi", "disci", "dis hekimi", "diş hekimi"},
	"doctor":       {"doctor", "doctors", "doktor", "hekim"},
	"clinic":       {"clinic", "medical_clinic", "klinik", "poliklinik"},
	"restaurant":   {"restaurant", "restoran", "lokanta", "yemek"},
	"cafe":         {"cafe", "café", "kafe", "kahve"},
	"hotel":        {"hotel", "otel"},
	"pharmacy":     {"pharmacy", "eczane"},
	"veterinary":   {"veterinary", "veteriner"},
	"florist":      {"florist", "flower_shop", "çiçekçi", "cicekci"},
	"gym":          {"gym", "fitness", "spor salonu"},
	"lawyer":       {"lawyer", "avukat", "hukuk"},
	"accountant":   {"accountant", "muhasebe", "mali müşavir"},
	"real_estate":  {"real_estate", "emlak", "gayrimenkul"},
	"car_repair":   {"car_repair", "oto tamir", "tamirci", "servis"},
	"construction": {"construction", "home_services", "tadilat", "dekorasyon", "mantolama", "çatı", "cati", "izolasyon", "klima", "kombi", "inşaat", "insaat", "müteahhit", "muteahhit"},
	"pest_control": {"pest_control", "ilaçlama", "ilaclama", "haşere", "hasere"},
	"manufacturer": {"manufacturer", "imalat", "fabrika", "üretim", "uretim", "sanayi"},
	"guest_house":  {"guest_house", "pansiyon", "butik otel", "villa", "apart"},
}

var aliasToKey = buildAliasIndex()

func buildAliasIndex() map[string]string {
	out := map[string]string{}
	for key, aliases := range CategoryAliases {
		out[key] = key
		for _, alias := range aliases {
			out[NormalizeName(alias)] = key
		}
	}
	return out
}

// CategoryKey maps a category name (English or Turkish) to its canonical key.
func CategoryKey(category string) string {
	name := NormalizeName(category)
	if name == "" {
		return ""
	}
	if key, ok := aliasToKey[name]; ok {
		return key
	}
	return name
}

func itoa(i int) string { return strconv.Itoa(i) }
