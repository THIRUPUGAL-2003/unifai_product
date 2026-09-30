"""Rebuild & Publish → installed Guards run the server's Guard code (agent + proxy)."""

from __future__ import annotations

import hashlib
import io
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path

AGENT_DIR = Path(__file__).resolve().parents[1]
PROXY_DIR = AGENT_DIR.parent / "proxy"
_TMP = tempfile.mkdtemp(prefix="unifai-bundle-test-")
os.environ["LOCALAPPDATA"] = _TMP
os.environ["HOME"] = _TMP
os.environ.pop("UNIFAI_GUARD_CODE_DIR", None)
os.environ.pop("UNIFAI_GUARD_CODE_SHA", None)
sys.path.insert(0, str(AGENT_DIR))

import agent_proxy_bundle as pb  # noqa: E402
import guard_bootstrap as gb  # noqa: E402


def source_files(mutate=None) -> dict[str, bytes]:
    files = {"browser_ai_proxy.py": (PROXY_DIR / "browser_ai_proxy.py").read_bytes()}
    for p in sorted((PROXY_DIR / "unifai_proxy_parts").iterdir()):
        if p.suffix == ".py" or p.name == "MANIFEST.txt":
            files[f"unifai_proxy_parts/{p.name}"] = p.read_bytes()
    for p in sorted(AGENT_DIR.glob("*.py")):
        if p.name != "guard_bootstrap.py":
            files[f"agent/{p.name}"] = p.read_bytes()
    if mutate:
        mutate(files)
    return files


def zipped(files: dict[str, bytes]) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as zf:
        for name in sorted(files):
            info = zipfile.ZipInfo(name, date_time=(2020, 1, 1, 0, 0, 0))
            zf.writestr(info, files[name])
    return buf.getvalue()


def info_for(data: bytes, versions=None) -> dict:
    return {
        "sha256": hashlib.sha256(data).hexdigest(),
        "published_at": "2026-09-28T00:00:00Z",
        "guard_versions": versions if versions is not None else [pb.AGENT_VERSION],
    }


def stage(files: dict[str, bytes], version: str) -> str:
    """Write a bundle exactly as apply_bundle leaves it; returns its sha."""
    sha = hashlib.sha256(zipped(files)).hexdigest()
    code = os.path.join(gb.bundle_root(), sha[:16])
    for name, body in files.items():
        path = os.path.join(code, *name.split("/"))
        os.makedirs(os.path.dirname(path), exist_ok=True)
        Path(path).write_bytes(body)
    gb._write_json(os.path.join(gb.bundle_root(), "active.json"), {
        "sha256": sha,
        "guard_version": version,
        "files": {n: hashlib.sha256(b).hexdigest() for n, b in files.items()},
    })
    return sha


def reset_store() -> None:
    shutil.rmtree(gb.bundle_root(), ignore_errors=True)
    os.environ.pop("UNIFAI_GUARD_CODE_DIR", None)
    os.environ.pop("UNIFAI_GUARD_CODE_SHA", None)


class ApplyBundleTests(unittest.TestCase):
    def setUp(self) -> None:
        reset_store()
        pb.UNIFAI_BACKEND_URL = "https://unifai.example.com"
        pb._failed_at.clear()
        self.restarts: list[str] = []
        self.served = b""
        pb._download = self._fake_download

    def _fake_download(self, sha: str) -> bytes:
        if hashlib.sha256(self.served).hexdigest() != sha:
            raise ValueError("SHA-256 mismatch")
        return self.served

    def _restart(self, reason: str) -> None:
        self.restarts.append(reason)

    def test_agent_and_proxy_code_is_self_tested_then_guard_restarts_onto_it(self) -> None:
        self.served = zipped(source_files())
        info = info_for(self.served)
        self.assertTrue(pb.apply_bundle(info, self._restart))
        self.assertEqual(len(self.restarts), 1)
        code_dir, sha = gb.verified_code_dir(pb.AGENT_VERSION)
        self.assertEqual(sha, info["sha256"])
        self.assertTrue(os.path.isfile(os.path.join(code_dir, "agent", "unifai_agent.py")))
        self.assertFalse(pb.apply_bundle(info, self._restart), "same bundle must not re-apply")

    def test_agent_code_that_crashes_on_import_is_rejected(self) -> None:
        def break_agent(files):
            files["agent/agent_http.py"] += b"\nraise RuntimeError('agent boom')\n"

        self.served = zipped(source_files(break_agent))
        info = info_for(self.served)
        self.assertFalse(pb.apply_bundle(info, self._restart))
        self.assertEqual(self.restarts, [])
        self.assertIn(info["sha256"], pb._bad_shas())
        self.assertEqual(gb.verified_code_dir(pb.AGENT_VERSION), ("", ""))

    def test_proxy_code_that_crashes_on_load_is_rejected(self) -> None:
        def break_proxy(files):
            files["unifai_proxy_parts/responses_addon.py"] += b"\nraise RuntimeError('proxy boom')\n"

        self.served = zipped(source_files(break_proxy))
        self.assertFalse(pb.apply_bundle(info_for(self.served), self._restart))
        self.assertEqual(self.restarts, [])

    def test_bundle_for_other_guard_version_is_ignored(self) -> None:
        self.served = zipped(source_files())
        self.assertFalse(pb.apply_bundle(info_for(self.served, ["0.0.1"]), self._restart))

    def test_plain_http_backend_never_downloads_code(self) -> None:
        pb.UNIFAI_BACKEND_URL = "http://10.0.0.5:8080"
        self.served = zipped(source_files())
        self.assertFalse(pb.apply_bundle(info_for(self.served), self._restart))

    def test_validate_rejects_unexpected_or_broken_files(self) -> None:
        ok = {"browser_ai_proxy.py": b"x = 1\n", "unifai_proxy_parts/a.py": b"a = 1\n"}
        pb.validate_bundle(zipped(ok))
        pb.validate_bundle(zipped({**ok, "agent/unifai_agent.py": b"def main(): pass\n"}))
        for bad in (
            {**ok, "../evil.py": b"x"},
            {**ok, "unifai_proxy_parts/../../evil.py": b"x"},
            {**ok, "agent/../evil.py": b"x"},
            {**ok, "agent/sub/x.py": b"x"},
            {**ok, "agent/unifai_agent.py": b"x", "agent/guard_bootstrap.py": b"x"},
            {**ok, "agent/agent_http.py": b"x = 1\n"},
            {**ok, "agent_config.py": b"x"},
            {**ok, "unifai_proxy_parts/a.py": b"def (:\n"},
            {**ok, "unifai_proxy_parts/MANIFEST.txt": b"a.py\nmissing.py\n"},
            {"unifai_proxy_parts/a.py": b"a = 1\n"},
        ):
            with self.assertRaises(Exception):
                pb.validate_bundle(zipped(bad))


class BootstrapTests(unittest.TestCase):
    def setUp(self) -> None:
        reset_store()
        self.version = gb.runtime_version()

    def test_tampered_file_falls_back_to_builtin(self) -> None:
        sha = stage(source_files(), self.version)
        self.assertEqual(gb.verified_code_dir(self.version)[1], sha)
        path = os.path.join(gb.bundle_root(), sha[:16], "agent", "agent_http.py")
        with open(path, "ab") as f:
            f.write(b"\n# tampered\n")
        self.assertEqual(gb.verified_code_dir(self.version), ("", ""))

    def test_other_exe_version_uses_builtin(self) -> None:
        stage(source_files(), "0.0.1")
        self.assertEqual(gb.verified_code_dir(self.version), ("", ""))

    def test_boot_loop_disables_bundle_until_confirmed_healthy(self) -> None:
        sha = stage(source_files(), self.version)
        for _ in range(gb.MAX_BOOT_ATTEMPTS):
            self.assertTrue(gb._count_boot(sha))
        os.environ["UNIFAI_GUARD_CODE_DIR"] = "x"
        os.environ["UNIFAI_GUARD_CODE_SHA"] = sha
        pb.confirm_bundle_healthy()
        self.assertTrue(gb._count_boot(sha), "healthy confirmation resets the counter")
        for _ in range(gb.MAX_BOOT_ATTEMPTS - 1):
            gb._count_boot(sha)
        self.assertFalse(gb._count_boot(sha))
        self.assertEqual(gb.verified_code_dir(self.version), ("", ""))

    def test_finder_serves_bundle_modules_and_can_be_removed(self) -> None:
        code = Path(tempfile.mkdtemp(dir=_TMP))
        (code / "agent").mkdir()
        (code / "agent" / "unifai_fake_mod.py").write_text("WHERE = 'bundle'\n")
        finder = gb.install_code_dir(str(code), "f" * 64)
        import unifai_fake_mod  # noqa: F401

        self.assertEqual(sys.modules["unifai_fake_mod"].WHERE, "bundle")
        self.assertEqual(pb.running_bundle_sha(), "f" * 64)
        self.assertTrue(pb.active_addon("BUILTIN") == "BUILTIN", "no proxy entry in this fake bundle")
        gb.uninstall_code_dir(finder)
        self.assertNotIn("unifai_fake_mod", sys.modules)
        self.assertEqual(pb.running_bundle_sha(), "")

    def _run_bootstrap(self, *args: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [sys.executable, str(AGENT_DIR / "guard_bootstrap.py"), *args],
            capture_output=True, text=True, timeout=120, env=os.environ.copy(),
        )

    def test_exe_runs_published_agent_code(self) -> None:
        marker = os.path.join(_TMP, "ran_bundle.txt")

        def instrument(files):
            files["agent/unifai_agent.py"] += (
                "\n_orig_main = main\n"
                "def main():\n"
                f"    open({marker!r}, 'w').write(os.environ.get('UNIFAI_GUARD_CODE_SHA', ''))\n"
                "    return _orig_main()\n"
            ).encode()

        sha = stage(source_files(instrument), self.version)
        proc = self._run_bootstrap("--mitm-worker", os.path.join(_TMP, "missing_addon.py"))
        self.assertEqual(proc.returncode, 2, proc.stdout + proc.stderr)
        self.assertEqual(Path(marker).read_text(), sha)

    def test_exe_falls_back_to_builtin_when_published_code_fails_to_import(self) -> None:
        def break_it(files):
            files["agent/agent_http.py"] += b"\nraise RuntimeError('boom')\n"

        sha = stage(source_files(break_it), self.version)
        proc = self._run_bootstrap("--mitm-worker", os.path.join(_TMP, "missing_addon.py"))
        self.assertEqual(proc.returncode, 2, proc.stdout + proc.stderr)
        self.assertIn(sha, pb._bad_shas())


class RelaunchTest(unittest.TestCase):
    def test_relaunch_starts_fresh_exe_with_unescaped_quotes(self) -> None:
        from unittest import mock

        for name, fn in (("guard_bootstrap.relaunch", gb.relaunch), ("agent_proxy_bundle._relaunch_fresh", pb._relaunch_fresh)):
            with self.subTest(name), \
                    mock.patch.object(subprocess, "Popen") as popen, \
                    mock.patch.object(sys, "frozen", True, create=True), \
                    mock.patch.object(sys, "platform", "win32"):
                fn()
            args, kwargs = popen.call_args
            self.assertIsInstance(args[0], str, "a list gets its quotes escaped as \\\" and cmd.exe cannot start the EXE")
            self.assertIn(f'start "" "{sys.executable}"', args[0])
            self.assertEqual(kwargs["env"].get("PYINSTALLER_RESET_ENVIRONMENT"), "1")


def tearDownModule() -> None:
    shutil.rmtree(_TMP, ignore_errors=True)


if __name__ == "__main__":
    unittest.main()
