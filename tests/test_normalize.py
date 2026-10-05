import unittest

from lead_hunter import normalize


class PhoneNormalizationTests(unittest.TestCase):
    def test_turkish_national_forms_converge(self):
        expected = "+905551112233"
        for raw in (
            "+90 555 111 22 33",
            "0555 111 22 33",
            "05551112233",
            "555 111 22 33",
            "(0555) 111-22-33",
            "0090 555 111 22 33",
        ):
            self.assertEqual(normalize.normalize_phone(raw, "TR"), expected, raw)

    def test_phone_key_drops_country_and_trunk_prefix(self):
        for raw in ("+90 555 111 22 33", "0555 111 22 33", "5551112233"):
            self.assertEqual(normalize.phone_key(raw, "TR"), "5551112233", raw)

    def test_non_turkish_keeps_digits(self):
        self.assertEqual(normalize.normalize_phone("030 1234567", "DE"), "0301234567")

    def test_international_prefix_and_plus_trusted(self):
        self.assertEqual(normalize.normalize_phone("+1 (202) 555-0143", "US"), "+12025550143")
        self.assertEqual(normalize.normalize_phone("0012025550143", "US"), "+12025550143")

    def test_empty_and_letters_return_none(self):
        self.assertIsNone(normalize.normalize_phone(None))
        self.assertIsNone(normalize.normalize_phone(""))
        self.assertIsNone(normalize.normalize_phone("no phone"))


class WebsiteNormalizationTests(unittest.TestCase):
    def test_strips_scheme_www_trailing_slash_and_tracking(self):
        self.assertEqual(
            normalize.normalize_website("http://www.Example.com/"),
            "http://example.com",
        )
        self.assertEqual(
            normalize.normalize_website("https://Example.com/path/?utm_source=x&utm_medium=y#frag"),
            "https://example.com/path",
        )

    def test_keeps_meaningful_query(self):
        self.assertEqual(
            normalize.normalize_website("example.com/page?id=7&utm_campaign=spring"),
            "https://example.com/page?id=7",
        )

    def test_plain_host_gets_https(self):
        self.assertEqual(normalize.normalize_website("acme.example"), "https://acme.example")

    def test_domain_helper(self):
        self.assertEqual(normalize.normalize_domain("www.Acme.example/path"), "acme.example")
        self.assertEqual(normalize.normalize_domain("https://acme.example"), "acme.example")
        self.assertEqual(normalize.normalize_domain(""), "")

    def test_none_and_empty(self):
        self.assertIsNone(normalize.normalize_website(None))
        self.assertIsNone(normalize.normalize_website(""))


class NameAndCategoryTests(unittest.TestCase):
    def test_name_folds_turkish_diacritics(self):
        self.assertEqual(normalize.normalize_name("Çiçekçi Şükrü"), "cicekcisukru")

    def test_similarity_matches_near_names(self):
        self.assertGreaterEqual(normalize.name_similarity("Acme Dental", "Acme Dent"), 0.86)
        self.assertGreaterEqual(normalize.name_similarity("Sarıtaş Döner", "Sarıtaş Döner Salonu"), 0.86)

    def test_similarity_rejects_different_names(self):
        self.assertLess(normalize.name_similarity("Acme Dental", "Bella Pizza"), 0.86)

    def test_category_aliases_merge_barber_and_hairdresser(self):
        self.assertEqual(normalize.category_key("hairdresser"), "barber")
        self.assertEqual(normalize.category_key("Berber"), "barber")
        self.assertEqual(normalize.category_key("kuaför"), "barber")
        self.assertEqual(normalize.category_key("Dişçi"), "dentist")
        self.assertEqual(normalize.category_key("eczane"), "pharmacy")

    def test_unknown_category_is_returned_normalized(self):
        self.assertEqual(normalize.category_key("Pet Shop"), "petshop")


if __name__ == "__main__":
    unittest.main()
