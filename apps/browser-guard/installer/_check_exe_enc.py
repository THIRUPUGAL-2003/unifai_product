import hashlib
from pathlib import Path
from PyInstaller.archive.readers import CArchiveReader

exe = Path(r"d:\unifai_project\apps\browser-guard\dist\Gateway_Guard.exe")
arch = CArchiveReader(str(exe))
names = list(arch.toc)
interesting = [
    n for n in names
    if "gateway_" in n.lower() or n.lower().endswith("browser_ai_proxy.py") or "guard_code" in n.lower()
]
print("interesting", interesting)
proxy = arch.extract("browser_ai_proxy.py")
print("stub", proxy[:80])
print("stub is loader", b"Live Proxy" in proxy)
print("stub decrypts", b"gateway_guard_code.enc" in proxy)
enc = arch.extract("gateway_guard_code.enc")
print("code enc", enc[:12], len(enc), hashlib.sha256(enc).hexdigest()[:16])
parts = arch.extract("gateway_proxy_parts\\gateway_proxy_parts.enc")
print("parts enc", parts[:12], len(parts))
raw = exe.read_bytes()
for needle in (b"SecretKeySeed", b"Enterprise Desktop Security Guard", b"def _load_parts"):
    print(needle, needle in raw)
print("agent py packed", any(n.endswith("gateway_agent.py") for n in names))
