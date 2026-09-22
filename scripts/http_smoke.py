#!/usr/bin/env python3
from __future__ import annotations

import io
import json
import urllib.request
import zipfile

BASE = "http://127.0.0.1:8787"


def get(path: str) -> bytes:
    with urllib.request.urlopen(BASE + path, timeout=10) as response:
        if response.status != 200:
            raise SystemExit(f"{path} returned {response.status}")
        return response.read()


def main() -> None:
    health = json.loads(get("/api/v1/health"))
    assert health["name"] == "LeadScout"
    assert health["version"] == "4.0.0"

    spec = json.loads(get("/api/v1/openapi.json"))
    assert "/api/v1/search" in spec["paths"]
    assert "/api/v1/export.xlsx" in spec["paths"]

    workbook = get("/api/v1/export.xlsx")
    with zipfile.ZipFile(io.BytesIO(workbook)) as zf:
        assert "xl/workbook.xml" in zf.namelist()
        assert "xl/worksheets/sheet1.xml" in zf.namelist()

    print("HTTP PASS: health, OpenAPI and XLSX download")


if __name__ == "__main__":
    main()
