from __future__ import annotations

import os
import subprocess
import sys
import venv
from pathlib import Path

ROOT=Path(__file__).resolve().parent
VENV=ROOT/".venv"

def venv_python() -> Path:
    if os.name=="nt":
        return VENV/"Scripts"/"python.exe"
    return VENV/"bin"/"python"

def main() -> int:
    if not venv_python().exists():
        print("[LeadScout] Creating local .venv ...")
        venv.EnvBuilder(with_pip=True).create(VENV)
    py=venv_python()
    print("[LeadScout] Checking dependencies ...")
    subprocess.check_call([str(py),"-m","pip","install","-q","-e",str(ROOT)])
    env=os.environ.copy()
    return subprocess.call([str(py),str(ROOT/"LeadScout.py")],cwd=str(ROOT),env=env)

if __name__=="__main__":
    raise SystemExit(main())
