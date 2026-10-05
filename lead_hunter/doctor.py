"""Health check for a local LeadScout install.

A missing optional dependency must be visible, not silent. The Afyon run
returned zero businesses because duckdb was not installed and Overture quietly
dropped out; this command exists so that failure mode is impossible to miss.
"""
from __future__ import annotations

import argparse
import importlib.util
import json
import shutil
import sqlite3
import sys
from pathlib import Path
from typing import Any

import lead_hunter.db as db
from lead_hunter.providers.osm import CATEGORY_FILTERS
from lead_hunter.providers.web_search import configured_providers

MODULES = (
    ("duckdb", "Overture discovery"),
    ("cryptography", "Encrypted local OAuth tokens"),
    ("mcp", "MCP agent surface"),
)

ENV_KEYS = (
    "LEADSCOUT_API_TOKEN",
    "LEADSCOUT_WEB_SEARCH",
    "GOOGLE_CSE_API_KEY",
    "GOOGLE_CSE_ID",
    "BRAVE_SEARCH_API_KEY",
    "SEARXNG_URL",
    "GOOGLE_PLACES_API_KEY",
)


def _module_status() -> list[dict[str, Any]]:
    checks = []
    for name, purpose in MODULES:
        found = importlib.util.find_spec(name) is not None
        checks.append({"name": name, "ok": found, "purpose": purpose})
    return checks


def _database_status() -> dict[str, Any]:
    path = Path(db.DB_PATH)
    info: dict[str, Any] = {"path": str(path), "exists": path.exists(), "tables": []}
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        db.initialize()
        with db.connect() as conn:
            info["tables"] = sorted(
                row["name"] for row in conn.execute(
                    "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name"
                ).fetchall()
            )
            info["lead_count"] = conn.execute("SELECT COUNT(*) AS n FROM leads").fetchone()["n"]
            info["user_version"] = conn.execute("PRAGMA user_version").fetchone()[0]
        info["writable"] = True
    except (sqlite3.Error, OSError) as exc:
        info["writable"] = False
        info["error"] = str(exc)
    return info


def _provider_status() -> dict[str, Any]:
    overture = importlib.util.find_spec("duckdb") is not None
    overture_reason = "" if overture else "duckdb is not installed; Overture discovery is disabled"
    web = configured_providers()
    return {
        "osm": {"ok": True, "detail": "public Overpass, no key required"},
        "overture": {"ok": overture, "detail": overture_reason or "duckdb available"},
        "web_search": {
            "ok": bool(web),
            "detail": "chain: " + ", ".join(web) if web else "disabled (LEADSCOUT_WEB_SEARCH=off)",
        },
        "google_places": {
            "ok": True,
            "detail": "optional reputation; key " + ("set" if _env("GOOGLE_PLACES_API_KEY") else "not set"),
        },
    }


def _env(name: str) -> bool:
    import os
    return bool(os.environ.get(name, "").strip())


def _config_status() -> dict[str, Any]:
    import os
    return {key: ("set" if os.environ.get(key, "").strip() else "not set") for key in ENV_KEYS}


def run_checks() -> dict[str, Any]:
    modules = _module_status()
    database = _database_status()
    providers = _provider_status()
    config = _config_status()
    node = shutil.which("node")

    problems: list[str] = []
    for item in modules:
        if not item["ok"]:
            problems.append(f"{item['name']} is missing ({item['purpose']}). Run: pip install -e .")
    if not database.get("writable"):
        problems.append(f"Database is not usable at {database['path']}: {database.get('error', 'unknown error')}")
    if not providers["overture"]["ok"]:
        problems.append(
            "Overture discovery is disabled: duckdb is missing, so discovery can silently "
            "return fewer results. Run: pip install -e ."
        )
    if not providers["web_search"]["ok"]:
        problems.append("Web verification is disabled by configuration (LEADSCOUT_WEB_SEARCH=off).")

    return {
        "ok": not problems,
        "python": {"version": sys.version.split()[0], "executable": sys.executable},
        "node": node or "not found (optional: website audits fall back to heuristics)",
        "modules": modules,
        "database": database,
        "providers": providers,
        "config": config,
        "categories": sorted(CATEGORY_FILTERS),
        "problems": problems,
    }


def format_report(report: dict[str, Any]) -> str:
    lines: list[str] = []
    mark = lambda ok: "OK " if ok else "!! "  # noqa: E731
    lines.append(f"LeadScout doctor — Python {report['python']['version']} ({report['python']['executable']})")
    lines.append(f"node: {report['node']}")
    lines.append("")
    lines.append("Modules:")
    for item in report["modules"]:
        lines.append(f"  [{mark(item['ok'])}] {item['name']:<13} {item['purpose']}")
    db_info = report["database"]
    lines.append("")
    lines.append("Database:")
    lines.append(f"  [{mark(db_info.get('writable'))}] {db_info['path']}")
    lines.append(f"      tables: {', '.join(db_info.get('tables') or []) or '(none)'}")
    lines.append(f"      leads: {db_info.get('lead_count', 0)}")
    lines.append("")
    lines.append("Providers:")
    for name, info in report["providers"].items():
        lines.append(f"  [{mark(info['ok'])}] {name:<14} {info['detail']}")
    lines.append("")
    lines.append("Config:")
    for name, state in report["config"].items():
        lines.append(f"  {name:<24} {state}")
    lines.append(f"  {'categories':<24} {len(report['categories'])}")
    if report["problems"]:
        lines.append("")
        lines.append("Problems:")
        for problem in report["problems"]:
            lines.append(f"  - {problem}")
    else:
        lines.append("")
        lines.append("No problems found.")
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="leadscout-doctor", description="Check a LeadScout install.")
    parser.add_argument("--json", action="store_true", help="Emit machine-readable JSON.")
    args = parser.parse_args(argv)

    report = run_checks()
    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2))
    else:
        print(format_report(report))
    return 0 if report["ok"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
