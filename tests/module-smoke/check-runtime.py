#!/usr/bin/env python3
"""Test/build/start module releases with GOWORK=off and an isolated local proxy."""
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
import json
import os
import shutil
import signal
import socket
import subprocess
import tempfile
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]


def command(args, cwd, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=240)
    if result.returncode:
        raise RuntimeError(f"{' '.join(map(str, args))}\n{result.stdout}")
    return result.stdout.strip()


def main():
    with tempfile.TemporaryDirectory(prefix="ohmyserver-contract-") as directory:
        temp = Path(directory)
        proxy = temp / "proxy"
        packager = temp / "package-module"
        command(["go", "build", "-o", str(packager), "./tests/module-smoke/proxy"], ROOT)
        cached = command(["go", "env", "GOMODCACHE"], ROOT)
        env = dict(os.environ, GOWORK="off", GOMODCACHE=str(temp / "cache"),
                   GOPROXY=f"{proxy.as_uri()},file://{cached}/cache/download,https://proxy.golang.org",
                   GONOSUMDB="github.com/lsongdev/ohmyserver/*")
        for name, version in [("moduleweb", "v0.1.0"), ("terminal", "v0.0.0")]:
            command([str(packager), str(ROOT / "pkg" / name),
                     f"github.com/lsongdev/ohmyserver/pkg/{name}", version, str(proxy)], ROOT)
        modules = sorted(path for path in (ROOT / "modules").iterdir() if (path / "go.mod").exists())
        for module in modules:
            command([str(packager), str(module), f"github.com/lsongdev/ohmyserver/modules/{module.name}",
                     "v0.0.0", str(proxy)], ROOT)

        def check(module):
            name = module.name
            # Test an independent source checkout as well as consumers of its zip.
            source = temp / "sources" / name
            shutil.copytree(module, source)
            sums = source / "go.sum"
            if sums.exists():
                sums.write_text("".join(line for line in sums.read_text().splitlines(True)
                                        if not line.startswith("github.com/lsongdev/ohmyserver/")))
            if "replace " in (source / "go.mod").read_text():
                raise RuntimeError(f"{name}: module still requires local replace")
            command(["go", "test", "-mod=mod", "./..."], source, env)
            command(["go", "build", "-mod=mod", "./..."], source, env)
            binary = temp / f"cli-{name}"
            command(["go", "build", "-mod=mod", "-o", str(binary), f"./cmd/{name}"], source, env)
            state = temp / "state" / name
            state.mkdir(parents=True)
            with socket.socket() as listener:
                listener.bind(("127.0.0.1", 0))
                port = listener.getsockname()[1]
            args = [str(binary), "web", "serve", "--listen", f"127.0.0.1:{port}"]
            cli = (source / "cmd" / name / "main.go").read_text()
            if 'String("db"' in cli:
                args += ["--db", str(state / "module.db")]
            if name == "files":
                args += ["--root", str(state)]
            with (state / "process.log").open("w+") as log:
                process = subprocess.Popen(args, cwd=state, env=env, stdout=log, stderr=log)
                try:
                    deadline = time.monotonic() + 15
                    while True:
                        try:
                            with urllib.request.urlopen(f"http://127.0.0.1:{port}/manifest.json", timeout=1) as response:
                                manifest = json.load(response)
                            assert manifest["id"] == name
                            break
                        except Exception:
                            if process.poll() is not None or time.monotonic() >= deadline:
                                log.seek(0)
                                raise RuntimeError(f"{name}: CLI did not start\n{log.read()}")
                            time.sleep(0.1)
                    for path, expected in [("/", 'content="."'), ("/assets/index.js", "mountStandalone"),
                                           ("/assets/runtime/view.js", "createModuleView")]:
                        with urllib.request.urlopen(f"http://127.0.0.1:{port}{path}", timeout=3) as response:
                            assert expected in response.read().decode(), (name, path)
                finally:
                    if process.poll() is None:
                        process.send_signal(signal.SIGTERM)
                    try:
                        process.wait(timeout=15)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait()
                        raise RuntimeError(f"{name}: CLI shutdown timed out")
            host = ROOT / "tests" / "module-smoke" / f"{name}-host"
            if host.exists():
                dest = temp / "hosts" / name
                shutil.copytree(host, dest)
                # Snapshot versions are deliberately rebuilt by this check.
                (dest / "go.sum").unlink(missing_ok=True)
                command(["go", "test", "-mod=mod", "./..."], dest, env)
            return name

        errors = []
        with ThreadPoolExecutor(max_workers=3) as pool:
            for result in as_completed([pool.submit(check, module) for module in modules]):
                try:
                    print(f"PASS {result.result()}: isolated test/build, CLI, release host", flush=True)
                except Exception as error:
                    errors.append(str(error))
                    print(f"FAIL {error}", flush=True)
        if errors:
            raise SystemExit(f"{len(errors)} module checks failed")
        print(f"PASS all {len(modules)} modules", flush=True)


if __name__ == "__main__":
    main()
