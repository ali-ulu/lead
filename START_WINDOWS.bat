@echo off
cd /d "%~dp0"
where py >nul 2>nul && (py LeadHunter.py) || (python LeadHunter.py)
pause
