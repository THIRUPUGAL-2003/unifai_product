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
import io
import zipfile
import shutil
import hashlib
import struct
import time
import re


def _read_crx_manifest_version(path):
    """Read the extension version from the signed CRX payload for update.xml."""
    try:
        with open(path, "rb") as f:
            raw = f.read()
        if raw[:4] != b"Cr24" or len(raw) < 12:
            return ""
        format_version, header_length = struct.unpack_from("<II", raw, 4)
        if format_version != 3:
            return ""
        payload_start = 12 + header_length
        with zipfile.ZipFile(io.BytesIO(raw[payload_start:])) as zf:
            manifest = json.loads(zf.read("manifest.json").decode("utf-8"))
        return str(manifest.get("version") or "").strip()
    except Exception:
        return ""

def pack_all():
    script_dir = os.path.abspath(os.path.dirname(__file__))
    bg_dir = os.path.dirname(script_dir)
    ext_src = os.path.join(bg_dir, "extension")
    rel_dir = os.path.join(bg_dir, "release")
    staging_ext = os.path.join(bg_dir, "installer", "staging", "extension")
    common_ext = os.path.join(os.environ.get("ProgramData", r"C:\ProgramData"), "Gateway", "Guard", "guard_extension")
    user_ext = os.path.join(os.environ.get("LOCALAPPDATA", ""), "Gateway", "Guard", "guard_extension")

    # Release signing key is a local ignored secret, never an extension source asset.
    os.environ.setdefault(
        "GATEWAY_GUARD_EXTENSION_PEM",
        os.path.join(bg_dir, ".secrets", "guard_extension.pem"),
    )

    os.makedirs(rel_dir, exist_ok=True)
    if os.path.isdir(staging_ext):
        shutil.rmtree(staging_ext)
    os.makedirs(staging_ext, exist_ok=True)
    try:
        os.makedirs(common_ext, exist_ok=True)
    except Exception:
        pass
    try:
        if user_ext:
            os.makedirs(user_ext, exist_ok=True)
    except Exception:
        pass

    # Use the version and public key that match the CRX generated below.
    manifest_path = os.path.join(ext_src, "manifest.json")
    with open(manifest_path, "r", encoding="utf-8") as f:
        mdata = json.load(f)
    agent_dir = os.path.join(bg_dir, "agent")
    if agent_dir not in sys.path:
        sys.path.insert(0, agent_dir)
    from agent_extension import (
        EXTENSION_VERSION,
        _ensure_pem,
        _public_key_b64,
        is_signed_firefox_xpi,
    )
    import agent_config

    backend_url = str(
        getattr(agent_config, "GATEWAY_BACKEND_URL", "")
        or getattr(agent_config, "DEFAULT_BACKEND", "")
    ).strip()
    server_host = ""
    if backend_url:
        from urllib.parse import urlsplit

        parsed_backend = urlsplit(backend_url if "://" in backend_url else "//" + backend_url)
        server_host = parsed_backend.netloc.rsplit("@", 1)[-1]

    mdata["version"] = EXTENSION_VERSION
    mdata["key"] = _public_key_b64(_ensure_pem())
    version = EXTENSION_VERSION

    # 1. WebStore Zip (manifest, content.js, page_hook.js, endpoint.js, popup, background, icons)
    store_zip = os.path.join(rel_dir, "yp-raksha.zip")
    with zipfile.ZipFile(store_zip, "w", compression=zipfile.ZIP_DEFLATED) as zf:
        for fname in os.listdir(ext_src):
            if fname.endswith(".pem") or fname.endswith(".crx") or fname.endswith(".zip") or fname.endswith(".xpi"):
                continue
            fpath = os.path.join(ext_src, fname)
            if os.path.isfile(fpath):
                if fname == "manifest.json":
                    manifest_payload = (json.dumps(mdata, indent=2) + "\n").encode("utf-8")
                    zf.writestr(fname, manifest_payload)
                    with open(os.path.join(staging_ext, fname), "wb") as mf:
                        mf.write(manifest_payload)
                elif fname == "endpoint.js":
                    endpoint_path = os.path.join(ext_src, fname)
                    with open(endpoint_path, "r", encoding="utf-8") as endpoint_file:
                        endpoint_payload = endpoint_file.read()
                    endpoint_payload, count = re.subn(
                        r"(?m)^var GATEWAY_SERVER_HOST\s*=.*;$",
                        "var GATEWAY_SERVER_HOST = " + json.dumps(server_host) + ";",
                        endpoint_payload,
                    )
                    if count != 1:
                        raise RuntimeError("extension/endpoint.js must contain one GATEWAY_SERVER_HOST setting")
                    endpoint_bytes = endpoint_payload.encode("utf-8")
                    zf.writestr(fname, endpoint_bytes)
                    with open(os.path.join(staging_ext, fname), "wb") as ef:
                        ef.write(endpoint_bytes)
                else:
                    zf.write(fpath, fname)
                    shutil.copy2(fpath, os.path.join(staging_ext, fname))
                try:
                    shutil.copy2(os.path.join(staging_ext, fname), os.path.join(common_ext, fname))
                except Exception:
                    pass
                try:
                    if user_ext:
                        shutil.copy2(os.path.join(staging_ext, fname), os.path.join(user_ext, fname))
                except Exception:
                    pass
    # Legacy alias for WebStore zip
    shutil.copy2(store_zip, os.path.join(rel_dir, "YP_Raksha_WebStore.zip"))

    # Keep an explicitly unsigned developer XPI; only publish .xpi after Mozilla signing.
    unsigned_xpi = os.path.join(rel_dir, "yp-raksha-unsigned.xpi")
    shutil.copy2(store_zip, unsigned_xpi)
    xpi_path = os.path.join(rel_dir, "yp-raksha.xpi")
    signed_source = os.environ.get("GATEWAY_GUARD_SIGNED_XPI", "").strip()
    if signed_source:
        if not os.path.isfile(signed_source) or not is_signed_firefox_xpi(signed_source):
            raise RuntimeError("GATEWAY_GUARD_SIGNED_XPI must point to a valid Mozilla-signed XPI.")
        with zipfile.ZipFile(signed_source, "r") as signed_zip:
            signed_manifest = json.loads(signed_zip.read("manifest.json").decode("utf-8"))
            if str(signed_manifest.get("version") or "") != str(version):
                raise RuntimeError("The Mozilla-signed XPI is stale; sign the current extension package.")
            for source_name in os.listdir(staging_ext):
                source_path = os.path.join(staging_ext, source_name)
                if os.path.isfile(source_path) and source_name != "yp-raksha.xpi":
                    if source_name not in signed_zip.namelist():
                        raise RuntimeError(f"The Mozilla-signed XPI does not match current source: {source_name}")
                    with open(source_path, "rb") as sf:
                        expected = sf.read()
                    if signed_zip.read(source_name) != expected:
                        raise RuntimeError(f"The Mozilla-signed XPI does not match current source: {source_name}")
        shutil.copy2(signed_source, xpi_path)
        shutil.copy2(signed_source, os.path.join(staging_ext, "yp-raksha.xpi"))
        if user_ext:
            try:
                shutil.copy2(signed_source, os.path.join(os.path.dirname(user_ext), "yp-raksha.xpi"))
            except Exception:
                pass
    else:
        if os.path.isfile(xpi_path):
            os.remove(xpi_path)
        print("[pack] Firefox install XPI omitted: provision a Mozilla-signed XPI through GATEWAY_GUARD_SIGNED_XPI.")

    # 3. Signed Chromium CRX3 (yp-raksha.crx & legacy guard_extension.crx)
    try:
        from agent_extension import pack_extension, _crx_path, get_extension_id
        pac_p = getattr(agent_config, "PAC_HTTP_PORT", 18195)
        crx_file = pack_extension(f"http://127.0.0.1:{pac_p}")
        if crx_file and os.path.isfile(crx_file):
            release_crx = os.path.join(rel_dir, "yp-raksha.crx")
            for attempt in range(5):
                try:
                    shutil.copy2(crx_file, release_crx)
                    break
                except OSError:
                    if attempt == 4:
                        raise
                    time.sleep(attempt + 1)
            ext_id = get_extension_id()
            packed_version = _read_crx_manifest_version(release_crx) or version
            update_xml = (
                '<?xml version=\'1.0\' encoding=\'UTF-8\'?>\n'
                '<gupdate xmlns=\'http://www.google.com/update2/response\' protocol=\'2.0\'>\n'
                f'  <app appid=\'{ext_id}\'>\n'
                f'    <updatecheck codebase=\'http://127.0.0.1:{pac_p}/extension/yp-raksha.crx\' version=\'{packed_version}\' />\n'
                '  </app>\n'
                '</gupdate>\n'
            )
            with open(os.path.join(rel_dir, "update.xml"), "w", encoding="utf-8") as f:
                f.write(update_xml)
        else:
            raise RuntimeError("Chrome did not produce a fresh signed CRX")
    except Exception as e:
        raise RuntimeError(f"Chromium extension signing failed: {e}") from e

    print(f"[pack] YP-Raksha v{version} successfully packed with unified naming:")
    print(f"  - Chromium (.crx): {os.path.join(rel_dir, 'yp-raksha.crx')} ({os.path.getsize(os.path.join(rel_dir, 'yp-raksha.crx'))} bytes)")
    if os.path.isfile(xpi_path):
        print(f"  - Firefox  (.xpi, Mozilla-signed): {xpi_path} ({os.path.getsize(xpi_path)} bytes)")
    else:
        print(f"  - Firefox  (unsigned developer package only): {unsigned_xpi}")
    print(f"  - Store    (.zip): {store_zip} ({os.path.getsize(store_zip)} bytes)")

if __name__ == "__main__":
    pack_all()
