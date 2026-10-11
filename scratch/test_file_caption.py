from pathlib import Path

ns = {}
root = Path(r"d:\unifai_project\apps\browser-guard\proxy\gateway_proxy_parts")
exec(compile((root / "config_caches_rules.py").read_text(encoding="utf-8"), "c", "exec"), ns)
exec(compile((root / "file_policy.py").read_text(encoding="utf-8"), "f", "exec"), ns)
samples = [
    "run-provider-harness-test: $(if $(HELP),,install-newman)",
    '<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/meta',
    "api.chat.example.com,Example AI,chat,TRUE,FALSE",
    r'Warn on API & Secret Keys,regex,"\b(?:api',
    "see this",
    "hiii",
]
for s in samples:
    print(s[:60], "syntax", ns["_looks_like_file_syntax"](s))
