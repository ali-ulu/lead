from __future__ import annotations

import os
import shutil
import subprocess
import venv
from pathlib import Path

ROOT = Path(__file__).resolve().parent
VENV = ROOT / ".venv"

def venv_python() -> Path:
    return VENV / ("Scripts/python.exe" if os.name == "nt" else "bin/python")

def main() -> int:
    if not venv_python().exists():
        print("[LeadScout] Creating local .venv ...")
        venv.EnvBuilder(with_pip=True).create(VENV)

    py = venv_python()
    print("[LeadScout] Checking Python dependencies ...")
    subprocess.check_call([str(py), "-m", "pip", "install", "-q", "-e", str(ROOT)])

    npm = shutil.which("npm")
    lighthouse_dir = ROOT / "node_modules" / "lighthouse"
    if npm and not lighthouse_dir.exists():
        print("[LeadScout] Installing optional Lighthouse audit tools ...")
        try:
            subprocess.check_call(
                [npm, "install", "--silent", "--no-audit", "--no-fund"],
                cwd=str(ROOT),
            )
        except Exception as exc:
            print(f"[LeadScout] Lighthouse install skipped: {exc}")

    return subprocess.call(
        [str(py), str(ROOT / "LeadScout.py")],
        cwd=str(ROOT),
        env=os.environ.copy(),
    )

if __name__ == "__main__":
    raise SystemExit(main())
