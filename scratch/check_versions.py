import os
import re
import zipfile

bg = r'd:\unifai_project\apps\browser-guard'

def read_file(p):
    if os.path.isfile(p):
        with open(p, 'r', encoding='utf-8', errors='ignore') as f:
            return f.read().strip()
    return 'Missing'

# 1. Windows Release VERSION.txt
win_ver = read_file(os.path.join(bg, 'release', 'VERSION.txt'))

# 2. Agent config baked
agent_ver = 'Unknown'
ac = read_file(os.path.join(bg, 'agent', 'agent_config.py'))
m = re.search(r'AGENT_VERSION_BAKED\s*=\s*"([^"]+)"', ac)
if m:
    agent_ver = m.group(1)

# 3. Guard bootstrap baked
boot_ver = 'Unknown'
gb = read_file(os.path.join(bg, 'agent', 'guard_bootstrap.py'))
m = re.search(r'RUNTIME_VERSION_BAKED\s*=\s*"([^"]+)"', gb)
if m:
    boot_ver = m.group(1)

# 4. Inno Setup
iss_ver = 'Unknown'
iss = read_file(os.path.join(bg, 'installer', 'Gateway_Guard.iss'))
m = re.search(r'#define\s+MyAppVersion\s+"([^"]+)"', iss)
if m:
    iss_ver = m.group(1)

# 5. Windows ZIP VERSION.txt
win_zip_ver = 'Unknown'
win_zip_path = os.path.join(bg, 'release', 'Gateway_Guard_Windows.zip')
if os.path.isfile(win_zip_path):
    try:
        with zipfile.ZipFile(win_zip_path) as z:
            win_zip_ver = z.read('VERSION.txt').decode('utf-8').strip()
    except Exception as e:
        win_zip_ver = f'Error: {e}'

# 6. macOS Spec
spec_ver = 'Unknown'
spec = read_file(os.path.join(bg, 'Gateway_Guard.macos.spec'))
m = re.search(r'"CFBundleShortVersionString":\s*"([^"]+)"', spec)
if m:
    spec_ver = m.group(1)

# 7. macOS Info.plist
plist_ver = 'Unknown'
plist = read_file(os.path.join(bg, 'release', 'Gateway_Guard.app', 'Contents', 'Info.plist'))
m = re.search(r'CFBundleShortVersionString</key>\s*<string>([^<]+)</string>', plist)
if m:
    plist_ver = m.group(1)

# 8. macOS App Resources VERSION.txt
mac_res_ver = read_file(os.path.join(bg, 'release', 'Gateway_Guard.app', 'Contents', 'Resources', 'VERSION.txt'))

# 9. macOS ZIP VERSION.txt
mac_zip_ver = 'Unknown'
mac_zip_path = os.path.join(bg, 'release', 'Gateway_Guard_macOS.zip')
if os.path.isfile(mac_zip_path):
    try:
        with zipfile.ZipFile(mac_zip_path) as z:
            mac_zip_ver = z.read('VERSION.txt').decode('utf-8').strip()
    except Exception as e:
        mac_zip_ver = f'Error: {e}'

print('========================================')
print('   GATEWAY GUARD VERSION VERIFICATION   ')
print('========================================')
print(f'1. Windows Release VERSION.txt : {win_ver}')
print(f'2. Agent Config Baked Version  : {agent_ver}')
print(f'3. Guard Bootstrap Version     : {boot_ver}')
print(f'4. Inno Setup MyAppVersion     : {iss_ver}')
print(f'5. Windows ZIP (VERSION.txt)   : {win_zip_ver}')
print('----------------------------------------')
print(f'6. macOS Spec Version          : {spec_ver}')
print(f'7. macOS Info.plist Version    : {plist_ver}')
print(f'8. macOS App Resources Version : {mac_res_ver}')
print(f'9. macOS ZIP (VERSION.txt)     : {mac_zip_ver}')
print('========================================')

all_versions = [win_ver, agent_ver, boot_ver, iss_ver, win_zip_ver, spec_ver, plist_ver, mac_res_ver, mac_zip_ver]
all_match = len(set(all_versions)) == 1
if all_match:
    print(f'>>> ALL VERSIONS MATCH PERFECTLY: {win_ver} <<<')
else:
    print('>>> WARNING: VERSION MISMATCH DETECTED! <<<')
