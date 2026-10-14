import os, shutil, sys

sys.path.insert(0, r'd:\unifai_project\apps\browser-guard\agent')
import agent_extension

src = r'd:\unifai_project\apps\browser-guard\extension'
dst1 = r'C:\Users\sakth\AppData\Local\Programs\Gateway\Guard\extension'
dst2 = r'C:\Users\sakth\AppData\Local\Gateway\Guard\guard_extension'

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
