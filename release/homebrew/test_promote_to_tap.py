import unittest
from unittest.mock import patch
import promote_to_tap as p


class PromotionGuardTest(unittest.TestCase):
    def test_no_local_promotion_even_when_download_skipped(self):
        for skip in (False, True):
            with patch.object(p, "verify_published_artifact") as download:
                with self.assertRaisesRegex(p.PromotionError, "promotion is disabled"):
                    p.promote("absent", "absent", "1.2.3", skip_download=skip)
                download.assert_not_called()
