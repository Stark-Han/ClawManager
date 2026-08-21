from __future__ import annotations

import os
import unittest
from unittest.mock import patch

import northbound_client


class NorthboundInstanceModeTest(unittest.TestCase):
    def test_defaults_to_lite(self) -> None:
        with patch.dict(os.environ, {}, clear=True):
            self.assertEqual(northbound_client.northbound_instance_mode(), "lite")

    def test_accepts_pro_and_builds_compatible_route(self) -> None:
        with patch.dict(os.environ, {"NORTHBOUND_INSTANCE_MODE": "pro"}, clear=True):
            self.assertEqual(northbound_client.northbound_instance_mode(), "pro")
        self.assertEqual(northbound_client.instance_collection_path("pro"), "/pro-instances")

    def test_rejects_unknown_mode(self) -> None:
        with patch.dict(os.environ, {"NORTHBOUND_INSTANCE_MODE": "windows"}, clear=True):
            with self.assertRaisesRegex(ValueError, "must be lite or pro"):
                northbound_client.northbound_instance_mode()


if __name__ == "__main__":
    unittest.main()
