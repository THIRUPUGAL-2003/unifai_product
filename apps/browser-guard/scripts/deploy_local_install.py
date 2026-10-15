import os, shutil, sys

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
sys.path.insert(0, os.path.join(ROOT, "agent"))
import agent_extension

src = os.path.join(ROOT, "extension")
local_app = os.environ.get("LOCALAPPDATA") or os.path.expanduser("~")
dst1 = os.path.join(local_app, "Programs", "Gateway", "Guard", "extension")
dst2 = os.path.join(local_app, "Gateway", "Guard", "guard_extension")

for dst in (dst1, dst2):
    os.makedirs(dst, exist_ok=True)
    for f in os.listdir(src):
        s = os.path.join(src, f)
        d = os.path.join(dst, f)
        if os.path.isfile(s):
            shutil.copy2(s, d)
            print(f'Copied {f} to {dst}')

base_url = 'http://127.0.0.1:18195'
for dst in (dst1, dst2):
    with open(os.path.join(dst, 'endpoint.js'), 'w', encoding='utf-8') as ef:
        ef.write(f'var GATEWAY_GUARD_BASE = "{base_url}";\n')

crx = agent_extension.pack_extension(base_url)
print('CRX packed at:', crx)
