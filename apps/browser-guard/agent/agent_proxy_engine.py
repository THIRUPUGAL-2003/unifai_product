"""MitM proxy engine launcher for Gateway Guard."""

from __future__ import annotations

import os
import signal
import socket
import subprocess
import sys
import time

from agent_config import LISTEN_HOST, PROXY_ADDR, SERVER_MODE, proxy_listen_port
from agent_health import free_proxy_port, port_open

_current_worker: subprocess.Popen | None = None


def stop_proxy_worker() -> None:
    """Kill the MitM worker child (it outlives the parent's os._exit otherwise)."""
    proc = _current_worker
    if proc is None or proc.poll() is not None:
        return
    try:
        proc.terminate()
        proc.wait(timeout=3)
    except Exception:
        try:
            proc.kill()
        except Exception:
            pass


def _resolve_listen_host() -> str:
    # Endpoint mode stays on 127.0.0.1 so only this PC is intercepted.
    # Server/network mode binds 0.0.0.0 (or GATEWAY_LISTEN_HOST) for corp PAC.
    listen_host = (LISTEN_HOST or "127.0.0.1").strip() or "127.0.0.1"
    if not SERVER_MODE:
        # Force IPv4 localhost. On some Windows setups mitm binds [::]:port only;
        # PAC/Chrome use 127.0.0.1 → connect timeout → health fail-open DIRECT.
        listen_host = "127.0.0.1"
        try:
            cfg_host = (PROXY_ADDR or "").rsplit(":", 1)[0].strip()
            if cfg_host and cfg_host not in ("0.0.0.0", "*", "::"):
                listen_host = cfg_host
        except Exception:
            pass
    elif listen_host in ("*",):
        listen_host = "0.0.0.0"
    return listen_host


def _can_bind(host: str, port: int) -> bool:
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        s.bind((host, int(port)))
        return True
    except OSError:
        return False
    finally:
        try:
            s.close()
        except Exception:
            pass


def _force_ipv4_name_resolution() -> None:
    """Windows IPv6 is often broken (SYN_SENT hang). mitmproxy then never
    finishes CONNECT → Chrome ERR_TIMED_OUT / 502 for ChatGPT, Gemini, etc.
    Pin DNS to IPv4 in the MitM worker only.
    """
    if getattr(socket, "_gateway_ipv4_only", False):
        return
    orig = socket.getaddrinfo

    def getaddrinfo(host, port, family=0, type=0, proto=0, flags=0):  # noqa: A002
        if family == socket.AF_INET6:
            return orig(host, port, family, type, proto, flags)
        try:
            return orig(host, port, socket.AF_INET, type, proto, flags)
        except OSError:
            return orig(host, port, family, type, proto, flags)

    socket.getaddrinfo = getaddrinfo  # type: ignore[assignment]
    socket._gateway_ipv4_only = True  # type: ignore[attr-defined]


def _run_mitmdump_inline(addon_script: str, listen_host: str, port: int) -> None:
    """Run mitmdump in THIS process (used by --mitm-worker child)."""
    from mitmproxy.tools.main import mitmdump

    _force_ipv4_name_resolution()
    print("[Gateway Guard] MitM upstream DNS: IPv4 only (avoids broken IPv6 CONNECT hang).")
    ssl_insecure_val = (
        "true"
        if (os.environ.get("GATEWAY_SSL_INSECURE") or os.environ.get("GATEWAY_SSL_INSECURE") or "").lower()
        in ("1", "true", "yes")
        else "false"
    )
    upstream_cert_val = "false" if ssl_insecure_val == "true" else "true"
    args = [
        "--listen-host", listen_host,
        "-p", str(port),
        "-s", addon_script,
        "--set", "block_global=false",
        "--set", f"ssl_insecure={ssl_insecure_val}",
        "--set", f"upstream_cert={upstream_cert_val}",
    ]
    # mitmdump registers signal handlers; worker may be a child process (ok on main thread).
    _orig_signal = signal.signal

    def _thread_safe_signal(sig, handler):  # type: ignore[no-untyped-def]
        try:
            return _orig_signal(sig, handler)
        except ValueError:
            return None

    try:
        print(f"[Gateway Guard] MitM worker listening on {listen_host}:{port}...")
        signal.signal = _thread_safe_signal  # type: ignore[assignment]
        mitmdump(args)
    except SystemExit as e:
        print(f"[Gateway Guard WARNING] Proxy engine exited ({e})")
    except Exception as e:
        print(f"[Gateway Guard ERROR] Proxy engine stopped: {e}")
    finally:
        signal.signal = _orig_signal  # type: ignore[assignment]


def run_mitm_worker_main(argv: list[str] | None = None) -> int:
    """CLI entry: Gateway_Guard --mitm-worker <addon> <port> [listen_host]"""
    args = list(argv if argv is not None else sys.argv[1:])
    # args[0] == --mitm-worker
    addon = args[1] if len(args) >= 2 else ""
    default_port = proxy_listen_port()
    port = int(args[2]) if len(args) >= 3 else default_port
    listen_host = args[3] if len(args) >= 4 else _resolve_listen_host()
    if not addon or not os.path.exists(addon):
        print(f"[Gateway Guard ERROR] MitM worker missing addon: {addon!r}")
        return 2
    _run_mitmdump_inline(addon, listen_host, port)
    return 0


def run_proxy_server(addon_script: str, port: int | None = None) -> None:
    """Supervise mitmdump in a child process.

    Why child: when mitm fails to bind (Errno 48) or hits "Event loop is closed",
    in-process mitmdump can hang without releasing the main loop — PAC stays
    fail-open DIRECT forever and Prompt Logs stay 0 on every AI domain.
    A child can be killed and restarted cleanly.
    """
    if port is None:
        port = proxy_listen_port()
    listen_host = _resolve_listen_host()  # captured here, reused for health checks below

    # If we are already the worker child, run inline.
    if (os.environ.get("GATEWAY_MITM_WORKER") or os.environ.get("GATEWAY_MITM_WORKER")) == "1":
        _run_mitmdump_inline(addon_script, listen_host, port)
        return

    free_proxy_port(port)
    for _ in range(40):
        if _can_bind(listen_host, port):
            break
        free_proxy_port(port)
        time.sleep(0.25)
    else:
        raise RuntimeError(f"Proxy port {listen_host}:{port} still busy before launch")

    env = os.environ.copy()
    env["GATEWAY_MITM_WORKER"] = "1"
    env["GATEWAY_MITM_WORKER"] = "1"
    if getattr(sys, "frozen", False):
        cmd = [sys.executable, "--mitm-worker", addon_script, str(port), listen_host]
    else:
        agent_main = os.path.join(os.path.dirname(os.path.abspath(__file__)), "gateway_agent.py")
        cmd = [sys.executable, agent_main, "--mitm-worker", addon_script, str(port), listen_host]
    print(f"[Gateway Guard] Launching MitM Security Interceptor on {listen_host}:{port}...")
    global _current_worker
    proc = subprocess.Popen(
        cmd,
        env=env,
        stdout=None,
        stderr=None,
    )
    _current_worker = proc

    # Wait until listening — or child exits (bind failure / crash).
    listened = False
    for _ in range(60):
        if port_open("127.0.0.1", port) or (
            listen_host not in ("127.0.0.1", "0.0.0.0", "") and port_open(listen_host, port)
        ):
            listened = True
            break
        rc = proc.poll()
        if rc is not None:
            raise RuntimeError(f"MitM worker exited before listen (code={rc})")
        time.sleep(0.25)

    if not listened:
        print(f"[Gateway Guard ERROR] MitM worker did not bind {listen_host}:{port} — killing child")
        try:
            proc.terminate()
            proc.wait(timeout=3)
        except Exception:
            try:
                proc.kill()
            except Exception:
                pass
        free_proxy_port(port)
        raise RuntimeError(f"Proxy failed to listen on {listen_host}:{port}")

    print(f"[Gateway Guard] MitM worker ready pid={proc.pid} on {listen_host}:{port}")

    # Block until child exits (crash / sleep-wake / stop).
    def _proxy_is_open() -> bool:
        """Check if the proxy port is reachable on any relevant address."""
        if port_open("127.0.0.1", port):
            return True
        # Server-mode binds 0.0.0.0 — also check listen_host (may differ from 127.0.0.1).
        if listen_host not in ("127.0.0.1", "") and port_open(listen_host, port):
            return True
        return False

    try:
        while True:
            rc = proc.poll()
            if rc is not None:
                print(f"[Gateway Guard WARNING] MitM worker exited (code={rc})")
                break
            # If listen socket vanished but process still alive → hung event loop.
            if not _proxy_is_open():
                time.sleep(1.0)
                if not _proxy_is_open() and proc.poll() is None:
                    print(
                        "[Gateway Guard ERROR] Proxy port lost while worker alive "
                        "(likely Event loop hung) — killing worker for restart"
                    )
                    try:
                        proc.terminate()
                        proc.wait(timeout=3)
                    except Exception:
                        try:
                            proc.kill()
                        except Exception:
                            pass
                    break
            time.sleep(0.5)
    finally:
        if proc.poll() is None:
            try:
                proc.terminate()
                proc.wait(timeout=3)
            except Exception:
                try:
                    proc.kill()
                except Exception:
                    pass
        free_proxy_port(port)
