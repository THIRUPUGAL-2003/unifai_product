"""
Pack and sign YP-Raksha browser extension for all browsers:
1. Chromium Signed CRX3 (.crx)
2. Chrome Web Store package (.zip)
3. Mozilla Firefox add-on package (.xpi)
4. Chrome Omaha update manifest (update.xml)
5. Mirror to installer staging and ProgramData common extension folder
"""

import os
import sys
import json
import zipfile
import shutil
import hashlib

def pack_all():
    script_dir = os.path.abspath(os.path.dirname(__file__))
    bg_dir = os.path.dirname(script_dir)
    ext_src = os.path.join(bg_dir, "extension")
    rel_dir = os.path.join(bg_dir, "release")
    staging_ext = os.path.join(bg_dir, "installer", "staging", "extension")
    common_ext = os.path.join(os.environ.get("ProgramData", r"C:\ProgramData"), "Gateway", "Guard", "guard_extension")

    os.makedirs(rel_dir, exist_ok=True)
    os.makedirs(staging_ext, exist_ok=True)
    try:
        os.makedirs(common_ext, exist_ok=True)
    except Exception:
        pass

    # Read manifest version
    manifest_path = os.path.join(ext_src, "manifest.json")
    with open(manifest_path, "r", encoding="utf-8") as f:
        mdata = json.load(f)
    version = mdata.get("version", "1.1.28.4")

    # 1. WebStore Zip (manifest, content.js, page_hook.js, endpoint.js, icons)
    store_zip = os.path.join(rel_dir, "YP_Raksha_WebStore.zip")
    with zipfile.ZipFile(store_zip, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for fname in os.listdir(ext_src):
            if fname.endswith(".pem") or fname.endswith(".crx") or fname.endswith(".zip") or fname.endswith(".xpi"):
                continue
            fpath = os.path.join(ext_src, fname)
            if os.path.isfile(fpath):
                zf.write(fpath, fname)
                shutil.copy2(fpath, os.path.join(staging_ext, fname))
                try:
                    shutil.copy2(fpath, os.path.join(common_ext, fname))
                except Exception:
                    pass

    # 2. Firefox XPI (same files without pem)
    xpi_path = os.path.join(rel_dir, "yp-raksha.xpi")
    shutil.copy2(store_zip, xpi_path)

    # 3. Signed CRX3
    agent_dir = os.path.join(bg_dir, "agent")
    if agent_dir not in sys.path:
        sys.path.insert(0, agent_dir)
    try:
        from agent_extension import pack_extension, _crx_path, get_extension_id
        crx_file = pack_extension("http://127.0.0.1:18195")
        if crx_file and os.path.isfile(crx_file):
            shutil.copy2(crx_file, os.path.join(rel_dir, "guard_extension.crx"))
            ext_id = get_extension_id()
            update_xml = (
                '<?xml version=\'1.0\' encoding=\'UTF-8\'?>\n'
                '<gupdate xmlns=\'http://www.google.com/update2/response\' protocol=\'2.0\'>\n'
                f'  <app appid=\'{ext_id}\'>\n'
                f'    <updatecheck codebase=\'http://127.0.0.1:18195/extension/guard.crx\' version=\'{version}\' />\n'
                '  </app>\n'
                '</gupdate>\n'
            )
            with open(os.path.join(rel_dir, "update.xml"), "w", encoding="utf-8") as f:
                f.write(update_xml)
    except Exception as e:
        print(f"[pack] CRX generation notice: {e}")

    print(f"[pack] YP-Raksha v{version} successfully packed:")
    print(f"  - WebStore: {store_zip} ({os.path.getsize(store_zip)} bytes)")
    print(f"  - Firefox:  {xpi_path} ({os.path.getsize(xpi_path)} bytes)")
    if os.path.isfile(os.path.join(rel_dir, "guard_extension.crx")):
        print(f"  - CRX3:     {os.path.join(rel_dir, 'guard_extension.crx')} ({os.path.getsize(os.path.join(rel_dir, 'guard_extension.crx'))} bytes)")

if __name__ == "__main__":
    pack_all()
