@echo off
cd /d "%~dp0"
where py >nul 2>nul && (py Nishan.py) || (python Nishan.py)
pause
