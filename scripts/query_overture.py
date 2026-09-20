#!/usr/bin/env python3
"""Overture adapter prototype.
Install optional dependency: python -m pip install duckdb
Set OVERTURE_PLACES_GLOB to the current official Places GeoParquet path or a local subset.
"""
import json, os, sys
try: import duckdb
except ImportError: raise SystemExit("duckdb is not installed: python -m pip install duckdb")

def main():
    if len(sys.argv)!=5: raise SystemExit("usage: query_overture.py SOUTH WEST NORTH EAST")
    south,west,north,east=map(float,sys.argv[1:]); source=os.environ.get("OVERTURE_PLACES_GLOB")
    if not source: raise SystemExit("Set OVERTURE_PLACES_GLOB from current Overture docs")
    con=duckdb.connect(); con.execute("INSTALL spatial; LOAD spatial;")
    rows=con.execute("SELECT * FROM read_parquet(?) WHERE bbox.xmin <= ? AND bbox.xmax >= ? AND bbox.ymin <= ? AND bbox.ymax >= ? LIMIT 500",[source,east,west,north,south]).fetchdf().to_dict("records")
    print(json.dumps(rows,ensure_ascii=False,default=str))
if __name__=="__main__": main()
