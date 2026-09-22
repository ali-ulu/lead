@echo off
cd /d "%~dp0"
where py >nul 2>nul && (py LeadScout.py) || (python LeadScout.py)
pause
