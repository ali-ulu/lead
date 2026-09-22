@echo off
cd /d "%~dp0"
where py >nul 2>nul && (py bootstrap.py) || (python bootstrap.py)
pause
