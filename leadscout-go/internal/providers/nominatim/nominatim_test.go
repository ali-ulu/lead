package nominatim

import "testing"

func TestPickPlacePrefersSettlementOverProvince(t *testing.T) {
	rows := []row{
		{DisplayName: "Afyonkarahisar, Turkey", AddressType: "province", Lat: "38.68", Lon: "30.64"},
		{DisplayName: "Afyonkarahisar Merkez, Turkey", AddressType: "city", Lat: "38.75", Lon: "30.53"},
	}
	if got := pickPlace(rows); got.AddressType != "city" {
		t.Errorf("got %q, want city", got.AddressType)
	}
}

func TestPickPlaceFallsBackToFirstRow(t *testing.T) {
	rows := []row{{DisplayName: "Nowhere", AddressType: "province", Lat: "1", Lon: "2"}}
	if got := pickPlace(rows); got.DisplayName != "Nowhere" {
		t.Errorf("got %q, want Nowhere", got.DisplayName)
	}
}

func TestPickPlacePrefersCityOverSuburb(t *testing.T) {
	rows := []row{
		{AddressType: "suburb", Lat: "1", Lon: "1"},
		{AddressType: "city", Lat: "2", Lon: "2"},
	}
	if got := pickPlace(rows); got.Lat != "2" {
		t.Errorf("got lat %q, want 2", got.Lat)
	}
}
