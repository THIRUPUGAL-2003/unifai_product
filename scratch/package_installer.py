import os
import shutil
import subprocess
from pathlib import Path

ROOT = Path(r"d:\unifai_project\apps\browser-guard")
STAGING = ROOT / "installer" / "staging"
STAGING.mkdir(parents=True, exist_ok=True)

shutil.copy2(ROOT / "build" / "exe" / "Gateway_Guard.exe", STAGING / "Gateway_Guard.exe")
shutil.copy2(ROOT / "release" / "gateway_guard_config.json", STAGING / "gateway_guard_config.json")
shutil.copy2(ROOT / "installer" / "gateway_guard.ico", STAGING / "gateway_guard.ico")
if (ROOT / "installer" / "EMPLOYEE_README.txt").is_file():
    shutil.copy2(ROOT / "installer" / "EMPLOYEE_README.txt", STAGING / "EMPLOYEE_README.txt")
if (ROOT / "installer" / "Uninstall_Gateway_Guard.bat").is_file():
    shutil.copy2(ROOT / "installer" / "Uninstall_Gateway_Guard.bat", STAGING / "Uninstall_Gateway_Guard.bat")

local_app_data = os.environ.get("LOCALAPPDATA", "")
iscc = Path(local_app_data) / "Programs" / "Inno Setup 6" / "ISCC.exe"
if not iscc.is_file():
    iscc = Path(r"C:\Program Files (x86)\Inno Setup 6\ISCC.exe")
if not iscc.is_file():
    iscc = Path(r"C:\Program Files\Inno Setup 6\ISCC.exe")

print(f"Using ISCC at {iscc}")
iss_file = ROOT / "installer" / "Gateway_Guard.iss"
res = subprocess.run([str(iscc), str(iss_file)], cwd=str(ROOT), capture_output=True, text=True)
print("ISCC Return code:", res.returncode)
if res.returncode != 0:
    print("STDOUT:", res.stdout[-500:])
    print("STDERR:", res.stderr[-500:])
else:
    print("Success! Created Gateway_Guard_Setup.exe")
    setup_exe = ROOT / "release" / "Gateway_Guard_Setup.exe"
    print("Setup EXE size:", setup_exe.stat().st_size if setup_exe.is_file() else "Not found")
