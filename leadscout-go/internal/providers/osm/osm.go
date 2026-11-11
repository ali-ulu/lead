// Package osm discovers businesses through OpenStreetMap's Overpass API.
package osm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ali-ulu/lead/leadscout-go/internal/cache"
	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/ratelimit"
)

// OverpassURLs are tried in order; the tiled fallback rotates across them.
var OverpassURLs = []string{
	"https://overpass-api.de/api/interpreter",
	"https://overpass.kumi.systems/api/interpreter",
}

const userAgent = "LeadScout/4.0 (local business research tool)"

// CategoryFilters maps a category key to its OSM tag selectors.
var CategoryFilters = map[string][][2]string{
	"dentist":       {{"amenity", "dentist"}},
	"clinic":        {{"amenity", "clinic"}, {"healthcare", "clinic"}},
	"doctor":        {{"amenity", "doctors"}, {"healthcare", "doctor"}},
	"physiotherapy": {{"healthcare", "physiotherapist"}},
	"veterinary":    {{"amenity", "veterinary"}},
	"restaurant":    {{"amenity", "restaurant"}},
	"cafe":          {{"amenity", "cafe"}},
	"hotel":         {{"tourism", "hotel"}, {"tourism", "guest_house"}},
	"beauty":        {{"shop", "beauty"}},
	"hairdresser":   {{"shop", "hairdresser"}},
	"barber":        {{"shop", "hairdresser"}},
	"spa":           {{"leisure", "spa"}, {"shop", "beauty"}},
	"real_estate":   {{"office", "estate_agent"}},
	"accountant":    {{"office", "accountant"}},
	"lawyer":        {{"office", "lawyer"}},
	"insurance":     {{"office", "insurance"}},
	"travel_agency": {{"shop", "travel_agency"}},
	"car_repair":    {{"shop", "car_repair"}},
	"car_dealer":    {{"shop", "car"}},
	"electrician":   {{"craft", "electrician"}},
	"plumber":       {{"craft", "plumber"}},
	"photographer":  {{"craft", "photographer"}},
	"architect":     {{"office", "architect"}},
	"gym":           {{"leisure", "fitness_centre"}},
	"bakery":        {{"shop", "bakery"}},
	"florist":       {{"shop", "florist"}},
	"construction": {
		{"craft", "builder"}, {"craft", "plasterer"}, {"craft", "painter"},
		{"craft", "insulation"}, {"craft", "roofer"}, {"craft", "hvac"},
		{"craft", "heating_engineer"}, {"craft", "carpenter"},
		{"shop", "trade"}, {"office", "construction_company"},
	},
	"pest_control": {{"craft", "pest_control"}, {"shop", "pest_control"}},
	"manufacturer": {{"man_made", "works"}},
	"guest_house": {
		{"tourism", "guest_house"}, {"tourism", "apartment"},
		{"tourism", "chalet"}, {"tourism", "hostel"},
	},
}

type socialSpec struct {
	keys   []string
	prefix string
}

var socialTags = map[string]socialSpec{
	"instagram": {[]string{"contact:instagram", "instagram", "brand:instagram", "operator:instagram"}, "https://instagram.com/"},
	"facebook":  {[]string{"contact:facebook", "facebook", "brand:facebook", "operator:facebook"}, "https://facebook.com/"},
	"linkedin":  {[]string{"contact:linkedin", "linkedin", "brand:linkedin", "operator:linkedin"}, "https://linkedin.com/"},
	"x":         {[]string{"contact:twitter", "twitter", "contact:x", "x"}, "https://x.com/"},
	"youtube":   {[]string{"contact:youtube", "youtube", "brand:youtube"}, "https://youtube.com/"},
	"tiktok":    {[]string{"contact:tiktok", "tiktok"}, "https://tiktok.com/@"},
	"telegram":  {[]string{"contact:telegram", "telegram"}, "https://t.me/"},
	"whatsapp":  {[]string{"contact:whatsapp", "whatsapp"}, "https://wa.me/"},
}

var socialOrder = []string{"instagram", "facebook", "linkedin", "x", "youtube", "tiktok", "whatsapp", "telegram"}

func tag(tags map[string]any, names ...string) string {
	for _, n := range names {
		if v, ok := tags[n]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func normalizeSocial(value, prefix string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	handle := strings.TrimLeft(value, "@/ ")
	if strings.HasSuffix(prefix, "wa.me/") {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, handle)
		handle = digits
	}
	return prefix + handle
}

// Socials extracts the social profile links from an OSM tag set.
func Socials(tags map[string]any) map[string]string {
	found := map[string]string{}
	for platform, spec := range socialTags {
		if value := tag(tags, spec.keys...); value != "" {
			if u := normalizeSocial(value, spec.prefix); u != "" {
				found[platform] = u
			}
		}
	}
	return found
}

// Social returns the primary social URL, if any.
func Social(tags map[string]any) string {
	socials := Socials(tags)
	for _, platform := range socialOrder {
		if v := socials[platform]; v != "" {
			return v
		}
	}
	return ""
}

// BuildQuery builds a bounding-box Overpass query.
func BuildQuery(south, west, north, east float64, category string, timeout int) (string, error) {
	filters, ok := CategoryFilters[category]
	if !ok {
		return "", fmt.Errorf("Unsupported category: %s", category)
	}
	bbox := fmt.Sprintf("%g,%g,%g,%g", south, west, north, east)
	var parts []string
	for _, f := range filters {
		parts = append(parts, fmt.Sprintf("nwr[\"%s\"=\"%s\"](%s);", f[0], f[1], bbox))
	}
	return fmt.Sprintf("[out:json][timeout:%d];(%s);out tags center qt;", timeout, strings.Join(parts, "")), nil
}

// BuildAroundQuery builds a radius Overpass query.
func BuildAroundQuery(lat, lon float64, radiusM int, category string, timeout int) (string, error) {
	filters, ok := CategoryFilters[category]
	if !ok {
		return "", fmt.Errorf("Unsupported category: %s", category)
	}
	radius := radiusM
	if radius < 1000 {
		radius = 1000
	}
	if radius > 100000 {
		radius = 100000
	}
	var parts []string
	for _, f := range filters {
		parts = append(parts, fmt.Sprintf("nwr[\"%s\"=\"%s\"](around:%d,%g,%g);", f[0], f[1], radius, lat, lon))
	}
	return fmt.Sprintf("[out:json][timeout:%d];(%s);out tags center qt;", timeout, strings.Join(parts, "")), nil
}

type overpassElement struct {
	Type   string   `json:"type"`
	ID     int64    `json:"id"`
	Lat    *float64 `json:"lat"`
	Lon    *float64 `json:"lon"`
	Center *struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"center"`
	Tags map[string]any `json:"tags"`
}

type overpassResponse struct {
	Elements []overpassElement `json:"elements"`
}

func fetchOne(endpoint, query string, timeout int) (*overpassResponse, error) {
	var cached overpassResponse
	key := []any{endpoint, query}
	if cache.Get("overpass", key, &cached) {
		return &cached, nil
	}
	call := func() (*overpassResponse, error) {
		form := url.Values{}
		form.Set("data", query)
		req, _ := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		client := &http.Client{Timeout: time.Duration(timeout+5) * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		// Python's urlopen raises on 4xx/5xx; mirror that so a gateway
		// timeout (502) surfaces as a retryable error instead of a
		// confusing JSON parse failure on the HTML error page.
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("overpass %s returned HTTP %d", endpoint, resp.StatusCode)
		}
		var data overpassResponse
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, err
		}
		return &data, nil
	}
	data, err := ratelimit.Call("overpass:"+endpoint, call, ratelimit.Options{Retries: 2})
	if err != nil {
		return nil, err
	}
	cache.Put("overpass", key, data)
	return data, nil
}

func fetch(query string, timeout int) (*overpassResponse, error) {
	var lastErr error
	for _, endpoint := range OverpassURLs {
		data, err := fetchOne(endpoint, query, timeout)
		if err == nil {
			return data, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("OpenStreetMap query failed: %v", lastErr)
}

// Normalize converts raw Overpass elements into leads.
func Normalize(data *overpassResponse, category, city, country string, limit int) []*model.Lead {
	var out []*model.Lead
	seen := map[string]bool{}
	for _, el := range data.Elements {
		tags := el.Tags
		if tags == nil {
			tags = map[string]any{}
		}
		name := tag(tags, "name", "brand")
		if name == "" {
			continue
		}
		sourceID := fmt.Sprintf("%s:%d", el.Type, el.ID)
		if seen[sourceID] {
			continue
		}
		seen[sourceID] = true
		lat := el.Lat
		lon := el.Lon
		if lat == nil && el.Center != nil {
			v := el.Center.Lat
			lat = &v
		}
		if lon == nil && el.Center != nil {
			v := el.Center.Lon
			lon = &v
		}
		website := tag(tags, "website", "contact:website", "url")
		socials := Socials(tags)
		primary := ""
		for _, platform := range socialOrder {
			if v := socials[platform]; v != "" {
				primary = v
				break
			}
		}
		lead := &model.Lead{
			Source:         "osm",
			SourceID:       sourceID,
			Name:           name,
			Country:        firstNonEmpty(tag(tags, "addr:country"), country),
			City:           firstNonEmpty(tag(tags, "addr:city", "addr:town", "addr:village", "addr:municipality"), city),
			Category:       category,
			Latitude:       lat,
			Longitude:      lon,
			Phone:          strPtr(tag(tags, "phone", "contact:phone", "mobile", "contact:mobile")),
			Email:          strPtr(tag(tags, "email", "contact:email")),
			SocialLinks:    socials,
			DataConfidence: "medium",
		}
		if website != "" {
			lead.Website = strPtr(website)
			lead.WebsiteStatus = "unknown"
		} else {
			lead.WebsiteStatus = "missing"
		}
		if primary != "" {
			lead.SocialURL = strPtr(primary)
		}
		out = append(out, lead)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// NormalizeFromElements is a test/utility entry point that normalizes raw
// Overpass elements without a network round-trip.
func NormalizeFromElements(elements []map[string]any, category, city, country string, limit int) []*model.Lead {
	resp := &overpassResponse{}
	for _, el := range elements {
		raw, _ := json.Marshal(el)
		var parsed overpassElement
		if err := json.Unmarshal(raw, &parsed); err == nil {
			resp.Elements = append(resp.Elements, parsed)
		}
	}
	return Normalize(resp, category, city, country, limit)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// SearchBBox queries a bounding box.
func SearchBBox(south, west, north, east float64, category, city, country string, timeout, limit int) ([]*model.Lead, error) {
	query, err := BuildQuery(south, west, north, east, category, timeout)
	if err != nil {
		return nil, err
	}
	data, err := fetch(query, timeout)
	if err != nil {
		return nil, err
	}
	return Normalize(data, category, city, country, limit), nil
}

// DistanceKm is the great-circle distance between two points.
func DistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthKm = 6371.0088
	p1 := rad(lat1)
	p2 := rad(lat2)
	dp := rad(lat2 - lat1)
	dl := rad(lon2 - lon1)
	a := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * earthKm * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func rad(d float64) float64 { return d * math.Pi / 180 }

// RadiusBBox converts a center and radius into a bounding box.
func RadiusBBox(lat, lon, radiusKm float64) (south, west, north, east float64) {
	latDelta := radiusKm / 111.32
	cosLat := math.Max(0.15, math.Abs(math.Cos(rad(lat))))
	lonDelta := radiusKm / (111.32 * cosLat)
	return lat - latDelta, lon - lonDelta, lat + latDelta, lon + lonDelta
}

func tileBoxes(south, west, north, east float64, grid int) [][4]float64 {
	latStep := (north - south) / float64(grid)
	lonStep := (east - west) / float64(grid)
	var boxes [][4]float64
	for row := 0; row < grid; row++ {
		for col := 0; col < grid; col++ {
			boxes = append(boxes, [4]float64{
				south + float64(row)*latStep,
				west + float64(col)*lonStep,
				south + float64(row+1)*latStep,
				west + float64(col+1)*lonStep,
			})
		}
	}
	return boxes
}

// FetchTileRows is the tiled fallback fetch (exported for tests).
func FetchTileRows(south, west, north, east float64, category, city, country string, timeout int) []*model.Lead {
	tileTimeout := timeout
	if tileTimeout > 10 {
		tileTimeout = 10
	}
	idx := int(math.Abs((south+west+north+east)*1000)) % len(OverpassURLs)
	query, err := BuildQuery(south, west, north, east, category, tileTimeout)
	if err != nil {
		return nil
	}
	data, err := fetchOne(OverpassURLs[idx], query, tileTimeout)
	if err != nil {
		return nil
	}
	return Normalize(data, category, city, country, 0)
}

// SearchAroundDetailed performs a radius search with tiled fallback.
func SearchAroundDetailed(lat, lon float64, radiusKm int, category, city, country string, timeout, limit int) (map[string]any, error) {
	if radiusKm < 1 {
		radiusKm = 1
	}
	if radiusKm > 100 {
		radiusKm = 100
	}
	probeTimeout := timeout
	if probeTimeout > 12 {
		probeTimeout = 12
	}
	query, err := BuildAroundQuery(lat, lon, radiusKm*1000, category, probeTimeout)
	if err != nil {
		return nil, err
	}
	if data, err := fetchOne(OverpassURLs[0], query, probeTimeout); err == nil {
		rows := Normalize(data, category, city, country, limit)
		return map[string]any{"rows": rows, "partial": false, "failed_tiles": 0}, nil
	}
	rows, partial, failed := searchTiledAroundDetailed(lat, lon, radiusKm, category, city, country, timeout, limit)
	warnings := []string{}
	if partial {
		warnings = append(warnings, fmt.Sprintf("OpenStreetMap returned partial data: %d tile(s) timed out.", failed))
	}
	return map[string]any{"rows": rows, "partial": partial, "failed_tiles": failed, "warnings": warnings}, nil
}

func searchTiledAroundDetailed(lat, lon float64, radiusKm int, category, city, country string, timeout, limit int) ([]*model.Lead, bool, int) {
	south, west, north, east := RadiusBBox(lat, lon, float64(radiusKm))
	merged := map[string]*model.Lead{}
	failedTiles := 0

	boxes := tileBoxes(south, west, north, east, 2)
	results := make([][]*model.Lead, len(boxes))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, box := range boxes {
		wg.Add(1)
		go func(i int, box [4]float64) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = FetchTileRows(box[0], box[1], box[2], box[3], category, city, country, timeout)
		}(i, box)
	}
	wg.Wait()

	for _, rows := range results {
		if len(rows) == 0 {
			failedTiles++
		}
		for _, row := range rows {
			if row.Latitude != nil && row.Longitude != nil {
				if DistanceKm(lat, lon, *row.Latitude, *row.Longitude) > float64(radiusKm) {
					continue
				}
			}
			merged[row.SourceID] = row
		}
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []*model.Lead
	for _, k := range keys {
		out = append(out, merged[k])
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, failedTiles > 0, failedTiles
}

// SearchAround performs a radius search and returns only the rows.
func SearchAround(lat, lon float64, radiusKm int, category, city, country string, timeout, limit int) ([]*model.Lead, error) {
	result, err := SearchAroundDetailed(lat, lon, radiusKm, category, city, country, timeout, limit)
	if err != nil {
		return nil, err
	}
	return result["rows"].([]*model.Lead), nil
}

var _ = errors.New
