#!/usr/bin/env python3
"""Image / audio / file uploads: real filename stays, generic 'screenshot' does not
overwrite, and Guard Rules still scan all three types."""

from __future__ import annotations

import json
import re
import sys
import time
import unittest
from pathlib import Path


PROXY_DIR = Path(__file__).resolve().parents[1]
PARTS_DIR = PROXY_DIR / "gateway_proxy_parts"


def _load_proxy_parts():
    names = [
        "config_caches_rules.py",
        "helpers_prompts.py",
        "uploads_detect.py",
        "file_policy.py",
        "extract_office_backend.py",
        "responses_inject.py",
    ]
    ns: dict = {"__name__": "browser_ai_proxy_test"}
    for name in names:
        path = PARTS_DIR / name
        code = path.read_text(encoding="utf-8")
        exec(compile(code, str(path), "exec"), ns)
    # Stop background config refresh from hitting the network during tests.
    ns["_bg_config_refresh_started"] = True
    ns["_rules_fetched_at"] = time.time()
    return ns


NS = _load_proxy_parts()


def _clear_upload_state() -> None:
    with NS["_FILE_ID_NAME_REGISTRY_LOCK"]:
        NS["_FILE_ID_NAME_REGISTRY"].clear()
    with NS["_DOMAIN_PENDING_NAMES_LOCK"]:
        NS["_DOMAIN_PENDING_NAMES"].clear()
    with NS["_CONTENT_HASH_NAME_LOCK"]:
        NS["_CONTENT_HASH_NAME_REGISTRY"].clear()
    with NS["_UPLOAD_FILE_CACHE_LOCK"]:
        NS["_UPLOAD_FILE_CACHE"].clear()
        NS["_UPLOAD_FILE_QUEUES"].clear()


def _install_ssn_rule() -> None:
    pat = r"\b\d{3}-\d{2}-\d{4}\b"
    NS["_cached_rules"] = [{
        "name": "SSN Rule",
        "pattern": pat,
        "regex": re.compile(pat, re.IGNORECASE),
        "action": "BLOCK",
        "severity": "HIGH",
        "warning_message": "SSN blocked",
    }]
    NS["_rules_fetched_at"] = time.time()
    NS["_rules_fetch_ok"] = True


def _png_bytes(pad: int = 240) -> bytes:
    return b"\x89PNG\r\n\x1a\n" + (b"\x00" * pad)


def _wav_bytes() -> bytes:
    # Minimal RIFF/WAVE header so _looks_like_audio is true.
    return b"RIFF" + (100).to_bytes(4, "little") + b"WAVE" + b"\x00" * 80


def _txt_bytes(text: str) -> bytes:
    return text.encode("utf-8")


def _chatgpt_send(*, file_id: str, generic_name: str, real_name: str = "", extra: dict | None = None) -> str:
    att = {
        "id": file_id,
        "name": generic_name,
        "mimeType": "image/png",
    }
    if real_name:
        att["file_name"] = real_name
    body = {
        "conversation_id": "conv-1",
        "action": "next",
        "messages": [{
            "author": {"role": "user"},
            "content": {"content_type": "multimodal_text", "parts": ["check this"]},
            "metadata": {"attachments": [att]},
        }],
    }
    if extra:
        body.update(extra)
    return json.dumps(body)


class UploadNameAndRuleTests(unittest.TestCase):
    def setUp(self) -> None:
        _clear_upload_state()
        _install_ssn_rule()

    def tearDown(self) -> None:
        _clear_upload_state()

    def test_generic_screenshot_is_fake_but_real_names_are_kept(self) -> None:
        fake = NS["_is_fake_upload_name"]
        real = NS["_is_real_user_upload_name"]
        for n in ("screenshot", "voice", "attachment", "blob", "null", "undefined"):
            self.assertTrue(fake(n), n)
            self.assertFalse(real(n), n)
        for n in (
            "screenshot.png", "Screenshot.JPG", "image.png",
            "audio.m4a", "recording.mp3", "pasted-image.png",
        ):
            self.assertFalse(fake(n), n)
            self.assertTrue(real(n), n)
        # Chat/product labels must NEVER become Prompt Log filenames.
        for n in ("Gateway", "Greeting", "New chat", "Gateway Manual Overview"):
            self.assertFalse(real(n), n)
        # ChatGPT page scripts must never become the Prompt Log filename.
        for n in ("analytics.js", "gtag.js", "chunk.js", "vendor.js", "hot-update.js"):
            self.assertFalse(real(n), n)
            self.assertFalse(NS["_name_fits_upload_bytes"](n, b"%PDF-1.4\n%%EOF\n" + (b"\x00" * 80)))
        for n in (
            "salary-slip.png",
            "Q3-report.pdf",
            "meeting-notes.m4a",
            "Screenshot 2024-09-21 at 2.33.00 AM.png",
            "IMG_4032.jpg",
            "Gateway Product (1)",
            "01-User-Manual(2).pdf",
        ):
            self.assertTrue(real(n), n)
            self.assertFalse(fake(n), n)

    def test_pdf_product_title_is_not_used_as_filename(self) -> None:
        # Minimal PDF with /Title (Gateway) — brand title must not become the log label.
        pdf = (
            b"%PDF-1.4\n"
            b"1 0 obj<< /Title (Gateway) >>endobj\n"
            b"trailer<< /Root 1 0 R >>\n"
            b"%%EOF\n"
        )
        got = NS["_name_from_pdf_metadata"](pdf)
        self.assertEqual(got, "")
        NS["cache_upload_file"](
            "chatgpt.com",
            file_name="",
            raw_bytes=pdf + (b"\x00" * 200),
            content_type="application/pdf",
            upload_reason="cdn upload",
            file_id="file-pdfTitle01",
        )
        entries = []
        for lst in NS["_UPLOAD_FILE_QUEUES"].values():
            entries.extend(lst)
        self.assertTrue(entries)
        name = (entries[-1].get("file_name") or "").lower()
        self.assertNotEqual(name, "gateway")
        self.assertNotEqual(name, "gateway.pdf")
        self.assertIn(name, {"document.pdf", "attachment"})

    def test_chatgpt_send_ignores_chat_title_gateway(self) -> None:
        domain = "chatgpt.com"
        fid = "file-docManual01"
        real_name = "01-User-Manual(2).pdf"
        handshake = json.dumps({
            "file_name": real_name,
            "file_size": 4096,
            "use_case": "my_files",
        })
        NS["remember_file_create_handshake"](
            handshake, domain, handshake.encode(), "/backend-api/files",
        )
        pdf = b"%PDF-1.4\n1 0 obj<< /Title (Gateway) >>endobj\n%%EOF\n" + (b"\x00" * 300)
        NS["cache_upload_file"](
            domain,
            file_name="",
            raw_bytes=pdf,
            content_type="application/pdf",
            upload_reason="cdn upload",
            file_id=fid,
        )
        send = json.dumps({
            "conversation_id": "conv-1",
            "title": "Gateway Manual Overview",
            "action": "next",
            "messages": [{
                "author": {"role": "user"},
                "content": {"content_type": "multimodal_text", "parts": ["hiiiiiii"]},
                "metadata": {
                    "attachments": [{
                        "id": fid,
                        "name": "Gateway",
                        "file_name": real_name,
                        "mimeType": "application/pdf",
                    }],
                },
            }],
        })
        NS["ingest_upload_filenames_from_body"](send, domain)
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        self.assertTrue(cached)
        self.assertEqual(cached[0]["file_name"], real_name)
        label = NS["_display_label_for_upload"](
            cached[0]["file_name"], cached[0]["raw_bytes"], "application/pdf",
        )
        self.assertEqual(label, real_name)
        names = NS["extract_all_attachment_filenames_from_send"](send)
        self.assertIn(real_name, names)
        self.assertNotIn("Gateway", names)

    def test_chatgpt_pdf_does_not_take_analytics_js_name(self) -> None:
        """Live bug: ChatGPT wire leaked analytics.js onto a PDF Prompt Log."""
        domain = "chatgpt.com"
        real_name = "04-Feature-Buttons.pdf"
        pdf = (
            b"%PDF-1.4\n1 0 obj<< /Title (Gateway) >>endobj\n"
            b"2 0 obj<< /Length 20 >>stream\nFEATURE-BTN\nendstream\nendobj\n"
            b"trailer<< /Root 1 0 R >>\n%%EOF\n"
        ) + (b"\x00" * 200)
        NS["cache_upload_file"](
            domain,
            file_name="",
            raw_bytes=pdf,
            content_type="application/pdf",
            upload_reason="cdn upload",
            file_id="file-feat01",
        )
        # Page/CDN JSON wrongly offers analytics.js — must not stamp the PDF.
        junk = json.dumps({"id": "file-feat01", "name": "analytics.js", "file_name": "analytics.js"})
        NS["ingest_upload_filenames_from_body"](junk, domain)
        NS["rename_recent_nameless_caches"](domain, "analytics.js", "file-feat01")
        handshake = json.dumps({
            "file_name": real_name,
            "file_size": 4096,
            "use_case": "my_files",
        })
        NS["remember_file_create_handshake"](
            handshake, domain, handshake.encode(), "/backend-api/files",
        )
        NS["rename_recent_nameless_caches"](domain, real_name, "file-feat01")
        send = json.dumps({
            "conversation_id": "conv-1",
            "action": "next",
            "messages": [{
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["hiiiiiii"]},
                "metadata": {
                    "attachments": [{
                        "id": "file-feat01",
                        "name": "analytics.js",
                        "file_name": real_name,
                    }],
                },
            }],
        })
        NS["ingest_upload_filenames_from_body"](send, domain)
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        self.assertTrue(cached)
        self.assertEqual(cached[0]["file_name"], real_name)
        label = NS["_display_label_for_upload"](
            cached[0]["file_name"], cached[0]["raw_bytes"], "application/pdf",
        )
        self.assertEqual(label, real_name)
        self.assertNotEqual(label.lower(), "analytics.js")

    def test_image_send_screenshot_does_not_overwrite_real_name(self) -> None:
        domain = "chatgpt.com"
        fid = "file-imgSalary01"
        real_name = "salary-slip.png"
        handshake = json.dumps({
            "file_name": real_name,
            "file_size": 4096,
            "use_case": "multimodal",
        })
        self.assertTrue(NS["remember_file_create_handshake"](
            handshake, domain, handshake.encode(), "/backend-api/files",
        ))
        NS["cache_upload_file"](
            domain,
            file_name="",
            raw_bytes=_png_bytes(),
            content_type="image/png",
            upload_reason="cdn upload",
            file_id=fid,
        )
        send = _chatgpt_send(file_id=fid, generic_name="screenshot")
        NS["ingest_upload_filenames_from_body"](send, domain)
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        self.assertTrue(cached, "image bytes must stay cached")
        self.assertEqual(cached[0]["file_name"], real_name)
        names = NS["extract_all_attachment_filenames_from_send"](send)
        self.assertNotIn("screenshot", [n.lower() for n in names])

    def test_image_json_prefers_file_name_over_screenshot(self) -> None:
        body = json.dumps({
            "id": "file-imgPrefer01",
            "name": "screenshot",
            "file_name": "invoice-scan.png",
            "mimeType": "image/png",
        })
        mapped = NS["extract_file_id_name_map"](body)
        self.assertEqual(mapped.get("file-imgPrefer01"), "invoice-scan.png")
        walked = NS["_walk_json_file_id_names"](json.loads(body))
        self.assertEqual(walked.get("file-imgPrefer01"), "invoice-scan.png")

    def test_audio_filename_and_transcript_rule_blocks(self) -> None:
        domain = "chatgpt.com"
        fid = "file-audMeet01"
        real_name = "meeting-notes.m4a"
        handshake = json.dumps({
            "file_name": real_name,
            "file_size": 2048,
            "use_case": "audio",
        })
        NS["remember_file_create_handshake"](
            handshake, domain, handshake.encode(), "/backend-api/files",
        )
        NS["cache_upload_file"](
            domain,
            file_name="",
            raw_bytes=_wav_bytes(),
            content_type="audio/wav",
            upload_reason="voice upload",
            file_id=fid,
        )
        send = json.dumps({
            "conversation_id": "conv-a",
            "action": "next",
            "messages": [{
                "author": {"role": "user"},
                "metadata": {
                    "attachments": [{"id": fid, "name": "audio", "file_name": real_name}],
                },
            }],
            "transcript": "Please review SSN 123-45-6789 from the call.",
        })
        NS["ingest_upload_filenames_from_body"](send, domain)
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        self.assertEqual(cached[0]["file_name"], real_name)
        self.assertTrue(NS["_looks_like_audio"](cached[0]["raw_bytes"], "audio/wav", real_name))

        scanned, hit, rule_name, action, *_rest = NS["_scan_upload_for_rules"](
            cached[0]["raw_bytes"],
            "audio/wav",
            send,
            real_name,
            cached[0],
            skip_backend=True,
        )
        self.assertIn("123-45-6789", scanned)
        self.assertTrue(hit)
        self.assertEqual(rule_name, "SSN Rule")
        self.assertEqual(action, "BLOCK")

    def test_file_filename_and_text_rule_blocks(self) -> None:
        domain = "chatgpt.com"
        fid = "file-docQ3rep01"
        real_name = "Q3-report.txt"
        handshake = json.dumps({
            "file_name": real_name,
            "file_size": 80,
            "use_case": "my_files",
        })
        NS["remember_file_create_handshake"](
            handshake, domain, handshake.encode(), "/backend-api/files",
        )
        payload = _txt_bytes("Employee SSN 987-65-4321 is confidential.\n")
        NS["cache_upload_file"](
            domain,
            file_name=real_name,
            raw_bytes=payload,
            content_type="text/plain",
            upload_reason="file upload",
            file_id=fid,
        )
        send = json.dumps({
            "conversation_id": "conv-f",
            "action": "next",
            "messages": [{
                "author": {"role": "user"},
                "metadata": {
                    "attachments": [{"id": fid, "name": real_name, "file_name": real_name}],
                },
            }],
        })
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        self.assertEqual(cached[0]["file_name"], real_name)

        scanned, hit, rule_name, action, *_rest = NS["_scan_upload_for_rules"](
            cached[0]["raw_bytes"],
            "text/plain",
            send,
            real_name,
            cached[0],
            skip_backend=True,
        )
        self.assertIn("987-65-4321", scanned)
        self.assertTrue(hit)
        self.assertEqual(rule_name, "SSN Rule")
        self.assertEqual(action, "BLOCK")

    def test_image_scan_path_runs_and_keeps_real_name(self) -> None:
        domain = "chatgpt.com"
        fid = "file-imgRule01"
        real_name = "badge-photo.png"
        handshake = json.dumps({
            "file_name": real_name,
            "file_size": 300,
            "use_case": "multimodal",
        })
        NS["remember_file_create_handshake"](
            handshake, domain, handshake.encode(), "/backend-api/files",
        )
        png = _png_bytes()
        NS["cache_upload_file"](
            domain,
            file_name="",
            raw_bytes=png,
            content_type="image/png",
            upload_reason="cdn upload",
            file_id=fid,
        )
        send = _chatgpt_send(file_id=fid, generic_name="screenshot")
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        self.assertEqual(cached[0]["file_name"], real_name)
        self.assertTrue(NS["_looks_like_image"](cached[0]["raw_bytes"], "image/png", real_name))

        # Image OCR may be empty in unit tests; caption/context still goes through rules.
        scanned, hit, rule_name, action, _excerpt, images, evaluated, _err = NS["_scan_upload_for_rules"](
            cached[0]["raw_bytes"],
            "image/png",
            send,
            real_name,
            cached[0],
            skip_backend=True,
            extra_context="ID card SSN 111-22-3333",
        )
        self.assertTrue(hit)
        self.assertEqual(rule_name, "SSN Rule")
        self.assertEqual(action, "BLOCK")
        self.assertTrue(evaluated)
        self.assertTrue(isinstance(images, list))

    def test_nameless_image_falls_back_to_type_label_not_screenshot(self) -> None:
        domain = "chatgpt.com"
        png = _png_bytes(400)
        NS["cache_upload_file"](
            domain,
            file_name="screenshot",
            raw_bytes=png,
            content_type="image/png",
            upload_reason="cdn upload",
            file_id="file-imgNone01",
        )
        q = NS["_UPLOAD_FILE_QUEUES"]
        entries = []
        for lst in q.values():
            entries.extend(lst)
        self.assertTrue(entries)
        name = (entries[-1].get("file_name") or "").lower()
        self.assertNotEqual(name, "screenshot")
        self.assertNotEqual(name, "screenshot.png")
        self.assertIn(name, {"image.png", "attachment"})


    def test_chatgpt_multi_pdf_keeps_each_real_name_like_claude(self) -> None:
        """Mac-correct behavior: each PDF keeps its real name, never brand title."""
        domain = "chatgpt.com"
        files = [
            ("file-m1", "01-User-Manual(2).pdf"),
            ("file-m2", "04-Feature-Buttons(1).pdf"),
            ("file-m3", "07-Worked-Examples(3).pdf"),
        ]
        for fid, real_name in files:
            handshake = json.dumps({
                "file_name": real_name,
                "file_size": 2048,
                "use_case": "my_files",
            })
            NS["remember_file_create_handshake"](
                handshake, domain, handshake.encode(), "/backend-api/files",
            )
            # Distinct PDF bodies (unique stream before %%EOF) — mirrors real multi-upload.
            body = (
                f"%PDF-1.4\n1 0 obj<< /Title (Gateway) /Subject ({real_name}) >>endobj\n"
                f"2 0 obj<< /Length 20 >>stream\n{fid}-CONTENT\nendstream\nendobj\n"
                f"trailer<< /Root 1 0 R >>\n%%EOF\n"
            ).encode("utf-8")
            NS["cache_upload_file"](
                domain,
                file_name="",
                raw_bytes=body,
                content_type="application/pdf",
                upload_reason="cdn upload",
                file_id=fid,
            )
        send = json.dumps({
            "conversation_id": "conv-multi",
            "title": "Gateway Manual Overview",
            "action": "next",
            "messages": [{
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["hiiiiiii"]},
                "metadata": {
                    "attachments": [
                        {"id": fid, "name": "Gateway", "file_name": real_name}
                        for fid, real_name in files
                    ],
                },
            }],
        })
        NS["ingest_upload_filenames_from_body"](send, domain)
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        names = [(e.get("file_name") or "") for e in cached]
        for _, real_name in files:
            self.assertIn(real_name, names)
        for n in names:
            self.assertNotEqual(n.lower(), "gateway")
            self.assertNotEqual(n.lower(), "gateway.pdf")
            self.assertNotEqual(n.lower(), "screenshot")
        label0 = NS["_display_label_for_upload"](
            cached[0]["file_name"], cached[0]["raw_bytes"], "application/pdf",
        )
        self.assertTrue(label0.endswith(".pdf"))
        self.assertNotIn(label0.lower(), {"gateway", "gateway.pdf", "screenshot"})

    def test_multi_file_predicts_all_even_if_only_one_id_key_cached(self) -> None:
        """Regression: finding 1 file by id must still predict siblings from the queue."""
        domain = "chatgpt.com"
        files = [
            ("file-only1", "alpha-report.pdf"),
            ("", "beta-notes.pdf"),           # CDN cache without file_id key
            ("", "gamma-sheet.pdf"),
        ]
        for fid, real_name in files:
            handshake = json.dumps({
                "file_name": real_name,
                "file_size": 2048,
                "use_case": "my_files",
            })
            NS["remember_file_create_handshake"](
                handshake, domain, handshake.encode(), "/backend-api/files",
            )
            body = (
                f"%PDF-1.4\n1 0 obj<< /Title (Gateway) >>endobj\n"
                f"2 0 obj<< /Length 24 >>stream\n{real_name}-BODY\nendstream\nendobj\n"
                f"trailer<< /Root 1 0 R >>\n%%EOF\n"
            ).encode("utf-8") + (b"\x00" * 80)
            NS["cache_upload_file"](
                domain,
                file_name="",
                raw_bytes=body,
                content_type="application/pdf",
                upload_reason="cdn upload",
                file_id=fid,
            )
        send = json.dumps({
            "conversation_id": "conv-partial",
            "action": "next",
            "messages": [{
                "author": {"role": "user"},
                "content": {"content_type": "text", "parts": ["check all"]},
                "metadata": {
                    "attachments": [
                        {"id": "file-only1", "name": "screenshot", "file_name": "alpha-report.pdf"},
                        {"id": "file-ghost2", "name": "screenshot", "file_name": "beta-notes.pdf"},
                        {"id": "file-ghost3", "name": "screenshot", "file_name": "gamma-sheet.pdf"},
                    ],
                },
            }],
        })
        # Mimic file_policy: id take then always top-up from queue.
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=False)
        more = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        seen = {str(e.get("cache_uid") or id(e)) for e in cached}
        for e in more:
            uid = str(e.get("cache_uid") or id(e))
            if uid not in seen:
                cached.append(e)
                seen.add(uid)
        cached = NS["_dedupe_cached_uploads_by_bytes"](cached)
        cached = NS["_trim_phantom_upload_caches"](cached, send, "check all")
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        self.assertGreaterEqual(len(cached), 3, f"expected 3 files to predict, got {len(cached)}")
        names = [(e.get("file_name") or "") for e in cached]
        for want in ("alpha-report.pdf", "beta-notes.pdf", "gamma-sheet.pdf"):
            self.assertIn(want, names)

    def test_second_file_does_not_reuse_first_file_name(self) -> None:
        """Already-uploaded File A name must not be shown again on File B."""
        domain = "chatgpt.com"
        name_a = "01-User-Manual.pdf"
        name_b = "02-Feature-Catalog.pdf"
        handshake_a = json.dumps({"file_name": name_a, "file_size": 2048, "use_case": "my_files"})
        NS["remember_file_create_handshake"](
            handshake_a, domain, handshake_a.encode(), "/backend-api/files",
        )
        pdf_a = (
            b"%PDF-1.4\n1 0 obj<< /Title (A) >>endobj\n"
            b"2 0 obj<< /Length 8 >>stream\nFILE-A\nendstream\nendobj\n"
            b"trailer<< /Root 1 0 R >>\n%%EOF\n"
        ) + (b"\x00" * 80)
        NS["cache_upload_file"](
            domain, file_name="", raw_bytes=pdf_a,
            content_type="application/pdf", upload_reason="cdn", file_id="file-a1",
        )
        # File A's name must not be re-queued / peeked onto File B.
        handshake_b = json.dumps({"file_name": name_b, "file_size": 2048, "use_case": "my_files"})
        NS["remember_file_create_handshake"](
            handshake_b, domain, handshake_b.encode(), "/backend-api/files",
        )
        NS["rename_recent_nameless_caches"](domain, name_a, "")
        pdf_b = (
            b"%PDF-1.4\n1 0 obj<< /Title (B) >>endobj\n"
            b"2 0 obj<< /Length 8 >>stream\nFILE-B\nendstream\nendobj\n"
            b"trailer<< /Root 1 0 R >>\n%%EOF\n"
        ) + (b"\x00" * 90)
        NS["cache_upload_file"](
            domain, file_name="", raw_bytes=pdf_b,
            content_type="application/pdf", upload_reason="cdn", file_id="file-b1",
        )
        send = json.dumps({
            "conversation_id": "c1",
            "action": "next",
            "messages": [{
                "author": {"role": "user"},
                "metadata": {"attachments": [
                    {"id": "file-a1", "file_name": name_a},
                    {"id": "file-b1", "file_name": name_b},
                ]},
            }],
        })
        cached = NS["take_all_cached_uploads_for_send"](domain, send, allow_latest=True)
        cached = NS["_bind_real_filenames_to_cached_uploads"](cached, send)
        names = [(e.get("file_name") or "") for e in cached]
        self.assertIn(name_a, names)
        self.assertIn(name_b, names)
        self.assertEqual(names.count(name_a), 1)

    def test_block_ui_shows_once_for_repeated_claude_injects(self) -> None:
        """Claude multi-request Send must paint the block bubble only once."""
        domain = "claude.ai"
        msg = "UPI ID not allowed -- UPI ID -- Bank"
        d1, k1 = NS["_block_ui_dedupe_key"](domain, msg)
        d2, k2 = NS["_block_ui_dedupe_key"]("www.claude.ai", msg)
        self.assertEqual(d1, d2)
        self.assertEqual(k1, k2)
        self.assertFalse(NS["is_duplicate_event"](d1, k1, ttl=15, mark=False))
        NS["mark_duplicate_event"](d1, k1)
        self.assertTrue(NS["is_duplicate_event"](d2, k2, ttl=15, mark=False))
        # Different rule text is a new bubble.
        d3, k3 = NS["_block_ui_dedupe_key"](domain, "Other rule blocked")
        self.assertFalse(NS["is_duplicate_event"](d3, k3, ttl=15, mark=False))

    def test_claude_block_sse_has_single_text_path(self) -> None:
        """Anthropic inject must not also emit legacy completion (double bubble)."""
        src = (PARTS_DIR / "responses_inject.py").read_text(encoding="utf-8")
        # Ensure the dual-emit completion event is gone from Claude path.
        self.assertIn("content_block_delta", src)
        # The Claude block builder should not append a second `event: completion` twin.
        # (Other platforms may still use completion — only Anthropic section must be clean.)
        start = src.find("# ── Claude / Anthropic chat APIs")
        end = src.find("# ── Microsoft Copilot", start)
        claude_block = src[start:end] if start >= 0 and end > start else ""
        self.assertTrue(claude_block)
        self.assertNotIn("event: completion", claude_block)
        self.assertIn("_silent_block_response", src)
        self.assertIn("_BLOCK_UI_DEDUPE_TTL", src)

    def test_mid_pattern_inline_flags_compile_like_backend(self) -> None:
        """Backend Go accepts 'a|(?i)b'; the Guard must enforce it too, not skip the rule."""
        compile_rx = NS["_compile_guard_regex"]
        rx = compile_rx(r"\b\d{10}\b|(?i)[A-Z]{5}[0-9]{4}[A-Z]")
        self.assertTrue(rx.search("4545454353"))
        self.assertTrue(rx.search("my pan abcde1234f"))
        self.assertFalse(rx.search("hello"))
        self.assertTrue(compile_rx(r"foo(?s).bar").search("foo\nbar"))
        self.assertTrue(compile_rx(r"(?P<n>\d{3})-(?:\d{2})").search("123-45"))
        self.assertTrue(compile_rx(r"\(?i\)").search("i)"))


if __name__ == "__main__":
    # Keep proxy dir importable if helpers expect local paths.
    sys.path.insert(0, str(PROXY_DIR))
    unittest.main(verbosity=2)
