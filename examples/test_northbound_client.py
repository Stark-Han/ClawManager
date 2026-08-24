from __future__ import annotations

import unittest

import northbound_client


class NorthboundInstanceCollectionTest(unittest.TestCase):
    def test_uses_one_collection_for_every_runtime(self) -> None:
        self.assertEqual(northbound_client.instance_collection_path(), "/lite-instances")


if __name__ == "__main__":
    unittest.main()
