import unittest
from lead_hunter.scoring import calculate_score

class ScoreTests(unittest.TestCase):
    def test_missing_website_with_contacts(self):
        score,reasons=calculate_score({"website":None,"website_status":"missing","phone":"x","email":"x","social_url":"x","data_confidence":"high"})
        self.assertEqual(score,60)
        self.assertTrue(any("No known" in r for r in reasons))

    def test_weak_dentist(self):
        score,reasons=calculate_score({"website":"https://example.test","category":"dentist","performance_score":30,"seo_score":40,"mobile_ok":False,"has_cta":False,"has_booking":False,"has_https":True,"phone":"x","data_confidence":"medium"})
        self.assertEqual(score,60)
        self.assertGreaterEqual(len(reasons),6)

    def test_cap(self):
        score,_=calculate_score({"website":"http://x","category":"dentist","performance_score":1,"seo_score":1,"mobile_ok":False,"has_cta":False,"has_booking":False,"has_https":False,"phone":"x","email":"x","social_url":"x","data_confidence":"high"})
        self.assertLessEqual(score,100)

if __name__=="__main__":
    unittest.main()
