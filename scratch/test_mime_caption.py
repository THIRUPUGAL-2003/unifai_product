import re


def _looks_like_file_syntax(text: str) -> bool:
    t = (text or "").strip()
    if not t:
        return False
    low = t.lower()
    if low.startswith(("content-type:", "content-disposition:", "content-transfer-encoding:", "mime-version:")):
        return True
    if "application/vnd." in low or "application/octet-stream" in low or "multipart/" in low:
        return True
    if re.match(r"^(?:application|image|audio|video|text)/[a-z0-9.+-]+$", low):
        return True
    if "xmlns" in low or "</" in t or (t.startswith("<") and (">" in t or ":" in t[:48])):
        return True
    if "$(" in t:
        return True
    if t.count(",") >= 3 and "\n" not in t:
        return True
    if "\\b" in t:
        return True
    return False


mime = "Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
assert _looks_like_file_syntax(mime) is True
assert _looks_like_file_syntax("books seed") is False
assert _looks_like_file_syntax("hiii") is False
assert _looks_like_file_syntax("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet") is True
print("ok")
