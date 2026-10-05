#!/usr/bin/env python3
"""Scheduled scan: search selected city/category pairs, export and diff.

Run it from cron for a hands-off morning report:

    python scripts/nightly.py --city Afyonkarahisar --country Turkey \\
        --category restaurant --category dentist --out-dir exports

Each pair is searched, the result is exported to one XLSX, and the run is
compared with the previous scan of the same pair so new businesses, website
changes and fresh high-opportunity leads are visible.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path

from lead_hunter.db import initialize, latest_search_run, lead_ids_for_search, purge_stale_leads
from lead_hunter.diff import compare_runs, format_report
from lead_hunter.exporters import xlsx_bytes
from lead_hunter.providers.osm import CATEGORY_FILTERS
from lead_hunter import services as _services
from lead_hunter.services import query_leads

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_OUT = ROOT / "exports"


def _load_config(path: str) -> list[dict]:
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    jobs = data.get("jobs") if isinstance(data, dict) else data
    if not isinstance(jobs, list):
        raise ValueError("Config must be a list of jobs or an object with a 'jobs' list.")
    return jobs


def _jobs_from_args(args) -> list[dict]:
    if args.config:
        return _load_config(args.config)
    jobs = []
    for city in args.city:
        for category in args.category:
            jobs.append({
                "city": city,
                "country": args.country,
                "category": category,
                "radius_km": args.radius_km,
            })
    return jobs


def run_nightly(
    jobs: list[dict],
    *,
    out_dir: Path = DEFAULT_OUT,
    export: bool = True,
    min_score: int = 70,
    retention_days: int = 0,
) -> dict:
    if not jobs:
        raise ValueError("No jobs to run. Pass --city/--category or --config.")

    out_dir = Path(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    results = []

    for job in jobs:
        city = str(job.get("city") or "").strip()
        category = str(job.get("category") or "").strip()
        country = str(job.get("country") or "").strip()
        radius_km = int(job.get("radius_km") or 20)
        if not city or not category:
            results.append({"city": city, "category": category, "error": "city and category are required"})
            continue
        if category not in CATEGORY_FILTERS:
            results.append({"city": city, "category": category, "error": f"unsupported category: {category}"})
            continue

        try:
            search = _services.discover_businesses(city=city, country=country, category=category, radius_km=radius_km)
        except Exception as exc:
            results.append({"city": city, "category": category, "error": str(exc)})
            continue

        # Match the previous run on the exact geocoded market, not the raw
        # input: discovery stores the resolved city/country, so a partial or
        # differently-spelled query would otherwise diff against the wrong run.
        area = search.get("area") or {}
        previous = latest_search_run(
            city=area.get("city") or city,
            country=area.get("country") or country,
            category=search.get("category") or category,
            exclude_id=search["search_id"],
        )
        previous_id = previous["id"] if previous else ""

        diff = None
        if previous_id and previous_id != search["search_id"]:
            diff = compare_runs(previous_id, search["search_id"], min_score=min_score)

        results.append({
            "city": city,
            "category": category,
            "search_id": search["search_id"],
            "count": search["count"],
            "degraded": search.get("degraded", False),
            "warnings": search.get("warnings", []),
            "previous_search_id": previous_id,
            "diff": diff,
        })

    export_ids: list[int] = []
    for result in results:
        if result.get("search_id"):
            export_ids.extend(lead_ids_for_search(result["search_id"]))

    purged = 0
    if retention_days > 0:
        purged = purge_stale_leads(days=retention_days)

    export_path = ""
    if export and export_ids:
        leads = query_leads(ids=sorted(set(export_ids)))
        export_path = str(out_dir / "leadscout-nightly.xlsx")
        Path(export_path).write_bytes(xlsx_bytes(leads))

    report = {
        "ok": True,
        "job_count": len(results),
        "total_leads": len(set(export_ids)),
        "export_path": export_path,
        "alerts": sum((r.get("diff") or {}).get("alert_count", 0) for r in results),
        "degraded_jobs": sum(1 for r in results if r.get("degraded")),
        "purged": purged,
        "errors": [r for r in results if r.get("error")],
        "results": results,
    }

    report_path = out_dir / "leadscout-nightly.json"
    report_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    report["report_path"] = str(report_path)

    summary_path = out_dir / "leadscout-nightly.txt"
    summary_path.write_text(format_nightly(report), encoding="utf-8")
    report["summary_path"] = str(summary_path)
    return report


def format_nightly(report: dict) -> str:
    lines = [
        f"LeadScout nightly — {report['job_count']} job(s), {report['total_leads']} lead(s)",
        f"  alerts: {report['alerts']}  degraded jobs: {report['degraded_jobs']}",
        f"  export: {report['export_path'] or '(none)'}",
        "",
    ]
    for item in report["results"]:
        header = f"{item.get('city')} / {item.get('category')}"
        if item.get("error"):
            lines.append(f"- {header}: ERROR {item['error']}")
            continue
        lines.append(f"- {header}: {item['count']} lead(s){' [degraded]' if item.get('degraded') else ''}")
        if item.get("diff"):
            lines.append("  " + format_report(item["diff"]).replace("\n", "\n  "))
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="leadscout-nightly", description="Run a scheduled LeadScout scan.")
    parser.add_argument("--city", action="append", default=[], help="City or area (repeatable).")
    parser.add_argument("--country", default="", help="Country for the cities.")
    parser.add_argument("--category", action="append", default=[], help="Category (repeatable).")
    parser.add_argument("--radius-km", type=int, default=20)
    parser.add_argument("--config", default="", help="JSON file with a list of jobs.")
    parser.add_argument("--out-dir", default=str(DEFAULT_OUT))
    parser.add_argument("--no-export", action="store_true", help="Skip the XLSX export.")
    parser.add_argument("--cache-ttl", type=int, default=0, help="Enable the response cache for N seconds.")
    parser.add_argument("--retention-days", type=int, default=0,
                        help="Delete not-contacted leads older than N days (KVKK/GDPR retention).")
    parser.add_argument("--json", action="store_true", help="Print the report as JSON.")
    args = parser.parse_args(argv)

    if args.cache_ttl:
        os.environ.setdefault("LEADSCOUT_CACHE_TTL", str(args.cache_ttl))

    jobs = _jobs_from_args(args)
    if not jobs:
        parser.error("Provide --city/--category or --config.")

    initialize()
    report = run_nightly(jobs, out_dir=Path(args.out_dir), export=not args.no_export,
                         retention_days=args.retention_days)

    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2))
    else:
        print(format_nightly(report))
    return 1 if report["errors"] else 0


if __name__ == "__main__":
    raise SystemExit(main())
