// Package merge deduplicates the same business arriving from multiple
// providers, then normalizes phone, website and category before storage.
package merge

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ali-ulu/lead/leadscout-go/internal/model"
	"github.com/ali-ulu/lead/leadscout-go/internal/normalize"
)

const (
	NameSimilarity = 0.86
	NearbyMeters   = 250.0
)

func distanceM(a, b *model.Lead) float64 {
	if a.Latitude == nil || a.Longitude == nil || b.Latitude == nil || b.Longitude == nil {
		return 1e9
	}
	lat1, lon1 := *a.Latitude, *a.Longitude
	lat2, lon2 := *b.Latitude, *b.Longitude
	const r = 6371000.0
	p1, p2 := rad(lat1), rad(lat2)
	dp := rad(lat2 - lat1)
	dl := rad(lon2 - lon1)
	x := math.Sin(dp/2)*math.Sin(dp/2) + math.Cos(p1)*math.Cos(p2)*math.Sin(dl/2)*math.Sin(dl/2)
	return 2 * r * math.Atan2(math.Sqrt(x), math.Sqrt(1-x))
}

func rad(d float64) float64 { return d * math.Pi / 180 }

func region(a, b *model.Lead) string {
	if a.Country != "" {
		return a.Country
	}
	if b.Country != "" {
		return b.Country
	}
	return "TR"
}

func sameBusiness(a, b *model.Lead) bool {
	da, db := normalize.NormalizeDomain(str(a.Website)), normalize.NormalizeDomain(str(b.Website))
	if da != "" && db != "" && da == db {
		return true
	}
	reg := region(a, b)
	pa := normalize.PhoneKey(str(a.Phone), reg)
	pb := normalize.PhoneKey(str(b.Phone), reg)
	if pa != "" && pb != "" && len(pa) >= 7 && pa == pb {
		return true
	}
	if normalize.NameSimilarity(a.Name, b.Name) >= NameSimilarity && distanceM(a, b) <= NearbyMeters {
		return true
	}
	return false
}

func canonicalID(lead *model.Lead) string {
	domain := normalize.NormalizeDomain(str(lead.Website))
	phone := normalize.PhoneKey(str(lead.Phone), region(lead, lead))
	var key string
	switch {
	case domain != "":
		key = "domain:" + domain
	case phone != "":
		key = "phone:" + phone
	default:
		lat := 0.0
		lon := 0.0
		if lead.Latitude != nil {
			lat = *lead.Latitude
		}
		if lead.Longitude != nil {
			lon = *lead.Longitude
		}
		key = fmt.Sprintf("namegeo:%s:%.4f:%.4f", normalize.NormalizeName(lead.Name), lat, lon)
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:32]
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// MergeLeads combines provider result sets into deduplicated leads.
func MergeLeads(providerSets ...[]*model.Lead) []*model.Lead {
	var merged []*model.Lead
	for _, rows := range providerSets {
		for _, candidate := range rows {
			var target *model.Lead
			for _, x := range merged {
				if sameBusiness(x, candidate) {
					target = x
					break
				}
			}
			if target == nil {
				item := *candidate
				item.Source = "multi"
				item.SourceRefs = copyRefs(candidate.SourceRefs)
				if len(item.SourceRefs) == 0 {
					item.SourceRefs = map[string]string{candidate.Source: candidate.SourceID}
				}
				item.SocialLinks = copySocials(candidate.SocialLinks)
				item.VerificationNotes = append([]string{}, candidate.VerificationNotes...)
				merged = append(merged, &item)
				continue
			}

			if target.SourceRefs == nil {
				target.SourceRefs = map[string]string{}
			}
			for k, v := range candidate.SourceRefs {
				target.SourceRefs[k] = v
			}
			if len(candidate.SourceRefs) == 0 {
				target.SourceRefs[candidate.Source] = candidate.SourceID
			}
			var refKeys []string
			for k := range target.SourceRefs {
				refKeys = append(refKeys, k)
			}
			sort.Strings(refKeys)
			target.VerificationNotes = append(target.VerificationNotes, "Matched across "+strings.Join(refKeys, ", "))
			if len(target.SourceRefs) >= 2 {
				target.VerificationStatus = "cross_source"
			} else if target.VerificationStatus == "" {
				target.VerificationStatus = "unverified"
			}

			if target.Website == nil && candidate.Website != nil {
				target.Website = candidate.Website
			}
			if target.Phone == nil && candidate.Phone != nil {
				target.Phone = candidate.Phone
			}
			if target.Email == nil && candidate.Email != nil {
				target.Email = candidate.Email
			}
			if target.SocialURL == nil && candidate.SocialURL != nil {
				target.SocialURL = candidate.SocialURL
			}
			if target.SocialLinks == nil {
				target.SocialLinks = map[string]string{}
			}
			for k, v := range candidate.SocialLinks {
				target.SocialLinks[k] = v
			}
			if target.SocialURL == nil && len(target.SocialLinks) > 0 {
				for _, v := range sortedValues(target.SocialLinks) {
					target.SocialURL = model.Str(v)
					break
				}
			}
			if target.Website != nil && *target.Website != "" {
				target.WebsiteStatus = "unknown"
			}
			if candidate.DataConfidence == "high" {
				target.DataConfidence = "high"
			}
			if target.Latitude == nil && candidate.Latitude != nil {
				target.Latitude = candidate.Latitude
			}
			if target.Longitude == nil && candidate.Longitude != nil {
				target.Longitude = candidate.Longitude
			}
		}
	}

	for _, lead := range merged {
		reg := lead.Country
		if reg == "" {
			reg = "TR"
		}
		if lead.Phone != nil {
			if p := normalize.NormalizePhone(*lead.Phone, reg); p != "" {
				lead.Phone = model.Str(p)
			} else {
				lead.Phone = nil
			}
		}
		if lead.Website != nil {
			if w := normalize.NormalizeWebsite(*lead.Website); w != "" {
				lead.Website = model.Str(w)
			} else {
				lead.Website = nil
			}
		}
		if lead.Website != nil && *lead.Website != "" && (lead.WebsiteStatus == "" || lead.WebsiteStatus == "missing") {
			lead.WebsiteStatus = "unknown"
		}
		if key := normalize.CategoryKey(lead.Category); key != "" {
			lead.Category = key
		}
		lead.Source = "multi"
		lead.SourceID = canonicalID(lead)
		if lead.SourceRefs == nil {
			lead.SourceRefs = map[string]string{}
		}
	}
	return merged
}

func copyRefs(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copySocials(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sortedValues(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	vals := make([]string, 0, len(m))
	for _, k := range keys {
		vals = append(vals, m[k])
	}
	return vals
}
