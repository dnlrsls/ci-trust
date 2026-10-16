"""Preparation-only candidate. Never imports ESLint, starts Docker or loads target code.

Run only in the separately authorized hosted preparation job. Unit tests are offline.
The generated image remains unadmitted and on the documented Node security hold.
"""

import base64
from contextlib import contextmanager
import hashlib
import http.server
import io
import json
import os
from pathlib import Path, PurePosixPath
import posixpath
import re
import select
import socket
import subprocess
import tarfile
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

NODE_VERSION = "24.22.0"
NPM_VERSION = "11.19.1"
NODE_ARCHIVE = "node-v24.22.0-linux-x64.tar.xz"
NODE_SHA256 = "cc2f601cee82d9a5a233302fe307abc600727035fd7d2c3a3571755989e95008"
BASE = "https://gcr.io/v2/distroless/cc-debian12/"
BASE_MANIFEST = "sha256:777e96cf322c46bc32aca926c263624c4dc8d7cf37e2fa65ba2c7e697318ebbb"
MAX_BYTES = 512 << 20
MAX_FILES = 50000
SHA = re.compile(r"sha256:[a-f0-9]{64}\Z")

# Only owned vocabulary may reach hosted logs. Never stringify caught errors.
STAGES = frozenset({"host-inputs", "node-download", "node-extract", "owned-manifest",
                    "npm-setup", "npm-lock", "npm-ci", "base-image", "rootfs",
                    "base-manifest", "base-config", "base-platform", "base-layers",
                    "base-layer-download", "base-layer-decompress", "base-layer-diff",
                    "base-layer-import",
                    "image-validation", "artifact-write"})
CATEGORIES = frozenset({"http", "network", "timeout", "filesystem", "archive",
                        "validation", "process-failed", "origin-denied", "unexpected"})

# Fixed invariant codes only; no input-derived strings or exception text.
REASONS = frozenset({"download-identity", "platform", "layer-count", "layer-media-type",
                     "layer-diff-id", "json-duplicate-key", "path-unsafe", "path-invalid",
                     "link-invalid", "layer-duplicate-or-limit", "layer-privileged",
                     "layer-whiteout", "layer-ancestor", "layer-expanded-limit",
                     "layer-hardlink", "layer-special-entry", "layer-type-replacement",
                     "rootfs-limit", "rootfs-missing", "link-cycle"})


class ValidationFailure(ValueError):
    def __init__(self, reason):
        self.reason = reason if reason in REASONS else None
        super().__init__("owned validation failure")


class PreparationFailure(Exception):
    def __init__(self, stage, category, reason=None):
        self.stage = stage if stage in STAGES else "unknown"
        self.category = category if category in CATEGORIES else "unexpected"
        self.reason = reason if reason in REASONS else None
        super().__init__(self.stage + ":" + self.category)


def error_category(error):
    # HTTPError/URLError must precede OSError; TimeoutExpired can contain argv.
    if isinstance(error, urllib.error.HTTPError):
        return "http"
    if isinstance(error, (TimeoutError, subprocess.TimeoutExpired)):
        return "timeout"
    if isinstance(error, urllib.error.URLError):
        return "network"
    if isinstance(error, tarfile.TarError):
        return "archive"
    if isinstance(error, OSError):
        return "filesystem"
    if isinstance(error, (ValueError, KeyError, TypeError)):
        return "validation"
    return "unexpected"


@contextmanager
def diagnostic_stage(name):
    if name not in STAGES:
        raise PreparationFailure("unknown", "unexpected")
    try:
        yield
    except PreparationFailure:
        raise
    except ValidationFailure as error:
        raise PreparationFailure(name, "validation", error.reason) from None
    except Exception as error:
        raise PreparationFailure(name, error_category(error)) from None


def main():
    try:
        prepare()
    except Exception as error:
        # Revalidate even structured failures; no raw exception text or traceback.
        if isinstance(error, PreparationFailure):
            failure = PreparationFailure(error.stage, error.category, error.reason)
        else:
            failure = PreparationFailure("unknown", error_category(error))
        reason = "; reason=" + failure.reason if failure.reason else ""
        print("owned lint preparation failed; stage=" + failure.stage + "; category=" + failure.category + reason + "; no runtime admission or ESLint execution", file=os.sys.stderr)
        return 1
    return 0


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode()


def unique_json(data):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValidationFailure("json-duplicate-key")
            result[key] = value
        return result
    return json.loads(data, object_pairs_hook=pairs)


def safe_path(name):
    name = name.removeprefix("./").rstrip("/")
    if not name or name.startswith("/") or "\\" in name or any(p in ("", ".", "..") for p in name.split("/")):
        raise ValidationFailure("path-unsafe")
    if len(name) > 4096 or any(ord(c) < 32 or ord(c) == 127 for c in name):
        raise ValidationFailure("path-invalid")
    return name


def link_path(name, target):
    if not target or "\\" in target or any(ord(c) < 32 for c in target):
        raise ValidationFailure("link-invalid")
    # Absolute links refer to the image root, never the preparation host.
    joined = target.lstrip("/") if target.startswith("/") else posixpath.join(posixpath.dirname(name), target)
    normalized = posixpath.normpath(joined)
    return safe_path(normalized)


class Redirects(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        checked_url(newurl)
        return super().redirect_request(req, fp, code, msg, headers, newurl)


def checked_url(url):
    parsed = urllib.parse.urlsplit(url)
    if parsed.scheme != "https" or parsed.username or parsed.password or parsed.port not in (None, 443) or parsed.fragment:
        raise ValueError("invalid download URL")
    if parsed.hostname not in {"nodejs.org", "gcr.io", "storage.googleapis.com"}:
        raise ValueError("download origin denied")


def download(url, expected):
    checked_url(url)
    if not SHA.fullmatch(expected):
        raise ValueError("missing artifact digest")
    # No proxy, cookies, ambient authenticated client or inherited headers.
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), Redirects())
    request = urllib.request.Request(url, headers={"Accept": "application/vnd.oci.image.manifest.v1+json"})
    with opener.open(request, timeout=45) as response:
        data = response.read(MAX_BYTES + 1)
    if len(data) > MAX_BYTES or digest(data) != expected:
        raise ValidationFailure("download-identity")
    return data


def overlay(tree, data):
    seen, total = set(), 0
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:*") as archive:
        for member in archive:
            if member.name in (".", "./") and member.isdir():
                continue
            name = safe_path(member.name)
            if name in seen or len(seen) >= MAX_FILES:
                raise ValidationFailure("layer-duplicate-or-limit")
            seen.add(name)
            if member.mode & 0o6000 or member.pax_headers.get("SCHILY.xattr.security.capability"):
                raise ValidationFailure("layer-privileged")
            if posixpath.basename(name).startswith(".wh."):
                # Pinned distroless input currently needs no whiteout support.
                # Reject rather than implement ambiguous deletion semantics.
                raise ValidationFailure("layer-whiteout")
            for parent in PurePosixPath(name).parents:
                if str(parent) != "." and str(parent) in tree and tree[str(parent)][0] != "dir":
                    raise ValidationFailure("layer-ancestor")
            if member.isdir():
                node = ("dir", 0o555, b"")
            elif member.isfile():
                total += member.size
                if member.size < 0 or total > MAX_BYTES:
                    raise ValidationFailure("layer-expanded-limit")
                node = ("file", 0o555 if member.mode & 0o111 else 0o444, archive.extractfile(member).read())
            elif member.issym():
                link_path(name, member.linkname)
                node = ("link", 0o777, member.linkname)
            elif member.islnk():
                target = safe_path(member.linkname)
                node = resolve(tree, target)
                if node[0] != "file":
                    raise ValidationFailure("layer-hardlink")
            else:
                raise ValidationFailure("layer-special-entry")
            if name in tree and tree[name][0] != node[0]:
                raise ValidationFailure("layer-type-replacement")
            tree[name] = node
            if len(tree) > MAX_FILES or sum(len(n[2]) for n in tree.values() if n[0] == "file") > MAX_BYTES:
                raise ValidationFailure("rootfs-limit")


def resolve(tree, name):
    name = safe_path(name)
    for _ in range(64):
        parts = name.split("/")
        for i in range(1, len(parts) + 1):
            prefix = "/".join(parts[:i])
            node = tree.get(prefix)
            if node and node[0] == "link":
                name = safe_path(posixpath.join(link_path(prefix, node[2]), *parts[i:]))
                break
        else:
            if name not in tree:
                raise ValidationFailure("rootfs-missing")
            return tree[name]
    raise ValidationFailure("link-cycle")


def base_tree():
    with diagnostic_stage("base-manifest"):
        manifest_bytes = download(BASE + "manifests/" + BASE_MANIFEST, BASE_MANIFEST)
        manifest = unique_json(manifest_bytes)
    with diagnostic_stage("base-config"):
        config = unique_json(download(BASE + "blobs/" + manifest["config"]["digest"], manifest["config"]["digest"]))
    with diagnostic_stage("base-platform"):
        if config["architecture"] != "amd64" or config["os"] != "linux":
            raise ValidationFailure("platform")
    with diagnostic_stage("base-layers"):
        layers = manifest["layers"]
        diff_ids = config["rootfs"]["diff_ids"]
        if len(layers) != len(diff_ids) or len(layers) > 64:
            raise ValidationFailure("layer-count")
    tree = {}
    import gzip
    for layer, diff_id in zip(layers, diff_ids):
        with diagnostic_stage("base-layers"):
            if layer["mediaType"] != "application/vnd.oci.image.layer.v1.tar+gzip":
                raise ValidationFailure("layer-media-type")
        with diagnostic_stage("base-layer-download"):
            compressed = download(BASE + "blobs/" + layer["digest"], layer["digest"])
        with diagnostic_stage("base-layer-decompress"):
            with gzip.GzipFile(fileobj=io.BytesIO(compressed)) as stream:
                data = stream.read(MAX_BYTES + 1)
        with diagnostic_stage("base-layer-diff"):
            if len(data) > MAX_BYTES or digest(data) != diff_id:
                raise ValidationFailure("layer-diff-id")
        with diagnostic_stage("base-layer-import"):
            overlay(tree, data)
    return tree, manifest


def validate_lock(lock):
    if lock.get("lockfileVersion") != 3 or lock.get("packages", {}).get("", {}).get("dependencies") != {"eslint": "10.12.0"}:
        raise ValueError("unexpected lock root")
    packages = lock["packages"]
    if len(packages) > 1024:
        raise ValueError("dependency graph limit")
    for name, package in packages.items():
        if name == "":
            continue
        safe_path(name)
        url = urllib.parse.urlsplit(package.get("resolved", ""))
        if not name.startswith("node_modules/") or package.get("link") or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", package.get("version", "")):
            raise ValueError("unsupported dependency identity")
        if url.scheme != "https" or url.netloc != "registry.npmjs.org" or url.query or url.fragment or url.username:
            raise ValueError("dependency origin denied")
        integrity = package.get("integrity", "")
        if not re.fullmatch(r"sha512-[A-Za-z0-9+/]+={0,2}", integrity) or len(base64.b64decode(integrity[7:], validate=True)) != 64:
            raise ValueError("missing dependency integrity")
        # Execution is disabled regardless; flag packages requiring scripts for
        # human review rather than considering successful installation efficacy.
    if packages.get("node_modules/eslint", {}).get("version") != "10.12.0":
        raise ValueError("wrong ESLint artifact")


class RegistryProxy(http.server.BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_CONNECT(self):
        if self.path != "registry.npmjs.org:443":
            self.server.denied = True
            self.send_error(403)
            return
        with socket.create_connection(("registry.npmjs.org", 443), timeout=30) as upstream:
            self.send_response(200)
            self.end_headers()
            deadline = time.monotonic() + 180
            while time.monotonic() < deadline:
                ready, _, _ = select.select([self.connection, upstream], [], [], 1)
                for source in ready:
                    data = source.recv(65536)
                    if not data:
                        return
                    (upstream if source is self.connection else self.connection).sendall(data)


def npm_install(node_root, target, home):
    node = str(node_root / "bin/node")
    npm = str(node_root / "lib/node_modules/npm/bin/npm-cli.js")
    bundled = unique_json((node_root / "lib/node_modules/npm/package.json").read_bytes())
    if bundled.get("version") != NPM_VERSION:
        raise ValueError("wrong bundled npm")
    for name in ("user.npmrc", "global.npmrc"):
        (home / name).write_text("", encoding="utf-8")
    # No host credential, Node option, Git/SSH executable or repository npmrc.
    env = {"HOME": str(home), "TMPDIR": str(home), "PATH": str(node_root / "bin"), "LANG": "C", "LC_ALL": "C",
           "NPM_CONFIG_USERCONFIG": str(home / "user.npmrc"), "NPM_CONFIG_GLOBALCONFIG": str(home / "global.npmrc")}
    common = ["--ignore-scripts", "--omit=dev", "--omit=optional", "--omit=peer", "--install-strategy=hoisted", "--no-bin-links", "--no-audit", "--no-fund", "--registry=https://registry.npmjs.org", "--noproxy=", "--cache=" + str(home / "cache")]
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), RegistryProxy)
    server.daemon_threads = True
    server.denied = False
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    proxy_url = "http://127.0.0.1:" + str(server.server_port)
    env.update(HTTP_PROXY=proxy_url, HTTPS_PROXY=proxy_url, ALL_PROXY=proxy_url, NO_PROXY="")
    try:
        frozen_lock = None
        for command in (["install", "--package-lock-only"], ["ci"]):
            with diagnostic_stage("npm-lock" if command[0] == "install" else "npm-ci"):
                # Node executes only the pinned npm CLI, never installed tool code.
                completed = subprocess.run([node, npm] + command + common + ["--https-proxy=" + proxy_url, "--proxy=" + proxy_url], cwd=target, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=180, check=False)
                if server.denied:
                    raise PreparationFailure("npm-lock" if command[0] == "install" else "npm-ci", "origin-denied")
                if completed.returncode != 0:
                    raise PreparationFailure("npm-lock" if command[0] == "install" else "npm-ci", "process-failed")
                lock_bytes = (target / "package-lock.json").read_bytes()
                validate_lock(unique_json(lock_bytes))
                if frozen_lock is not None and lock_bytes != frozen_lock:
                    raise ValueError("frozen install changed dependency lock")
                frozen_lock = lock_bytes
    finally:
        server.shutdown()
        server.server_close()
    return common


def add_file(tree, name, data, executable=False):
    name = safe_path(name)
    for parent in reversed(PurePosixPath(name).parents):
        if str(parent) != ".":
            old = tree.setdefault(str(parent), ("dir", 0o555, b""))
            if old[0] != "dir":
                raise ValueError("unsafe owned parent")
    if name in tree:
        raise ValueError("owned path collision")
    tree[name] = ("file", 0o555 if executable else 0o444, data)


def inventory(tree):
    result = []
    for name, (kind, mode, value) in sorted(tree.items()):
        item = {"path": name, "type": kind, "mode": oct(mode), "uid": 0, "gid": 0}
        if kind == "file":
            item.update(size=len(value), sha256=digest(value))
        elif kind == "link":
            resolve(tree, name)
            item["target"] = value
        result.append(item)
    return result


def pack_tar(entries):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w", format=tarfile.PAX_FORMAT) as archive:
        for name, (kind, mode, value) in sorted(entries.items()):
            header = tarfile.TarInfo(name)
            header.mode, header.uid, header.gid, header.mtime = mode, 0, 0, 0
            if kind == "dir":
                header.type = tarfile.DIRTYPE
            elif kind == "link":
                header.type, header.linkname = tarfile.SYMTYPE, value
            else:
                header.size = len(value)
            archive.addfile(header, io.BytesIO(value) if kind == "file" else None)
    return output.getvalue()


def pack_image(tree):
    layer = pack_tar(tree)
    config = canonical({"architecture": "amd64", "os": "linux", "config": {"User": "65532:65532", "WorkingDir": "/work", "Env": [], "Entrypoint": [], "Cmd": []}, "rootfs": {"type": "layers", "diff_ids": [digest(layer)]}, "history": []})
    name = digest(config)[7:] + ".json"
    manifest = canonical([{"Config": name, "RepoTags": [], "Layers": ["layer.tar"]}])
    archive = pack_tar({name: ("file", 0o444, config), "layer.tar": ("file", 0o444, layer), "manifest.json": ("file", 0o444, manifest)})
    return archive, {"image_id": digest(config), "layer_diff_id": digest(layer), "archive_sha256": digest(archive)}


def inspect_archive(archive, expected_inventory, identities):
    outer = {}
    overlay(outer, archive)
    manifest = unique_json(outer["manifest.json"][2])
    if len(manifest) != 1 or manifest[0]["RepoTags"] != [] or manifest[0]["Layers"] != ["layer.tar"] or len(outer) != 3:
        raise ValueError("unexpected image archive")
    name = manifest[0]["Config"]
    config_bytes, layer = outer[name][2], outer["layer.tar"][2]
    config = unique_json(config_bytes)
    wanted = {"User": "65532:65532", "WorkingDir": "/work", "Env": [], "Entrypoint": [], "Cmd": []}
    if name != digest(config_bytes)[7:] + ".json" or config != {"architecture": "amd64", "os": "linux", "config": wanted, "rootfs": {"type": "layers", "diff_ids": [digest(layer)]}, "history": []}:
        raise ValueError("image config mismatch")
    actual = {"image_id": digest(config_bytes), "layer_diff_id": digest(layer), "archive_sha256": digest(archive)}
    if actual != identities:
        raise ValueError("image identities mismatch")
    reconstructed = {}
    overlay(reconstructed, layer)
    if inventory(reconstructed) != expected_inventory:
        raise ValueError("rootfs inventory mismatch")


def prepare():
    with diagnostic_stage("host-inputs"):
        if os.uname().sysname != "Linux" or os.uname().machine != "x86_64":
            raise ValueError("hosted Linux amd64 preparation required")
        workspace = Path(os.environ["GITHUB_WORKSPACE"]).resolve()
        prep = workspace / ".git/lint-image-preparation"
        artifacts = prep / "artifacts"
        artifacts.mkdir(mode=0o700)
        inputs = unique_json((prep / "inputs.json").read_bytes())
        if hashlib.sha256(inputs["Module"].encode()).hexdigest() != inputs["ModuleSHA256"]:
            raise ValueError("owned module mismatch")
        source = workspace / "tools/lint-preparation"
    with tempfile.TemporaryDirectory(prefix="owned-lint-", dir=prep) as temp:
        temp = Path(temp)
        with diagnostic_stage("node-download"):
            node_archive = download("https://nodejs.org/dist/v" + NODE_VERSION + "/" + NODE_ARCHIVE, "sha256:" + NODE_SHA256)
        # Extraction is only for the verified preparation-time Node/npm bundle,
        # never OCI layers or installed package code. Python data filter confines
        # links; the reviewed digest is mandatory before this extraction.
        with diagnostic_stage("node-extract"):
            with tarfile.open(fileobj=io.BytesIO(node_archive), mode="r:xz") as archive:
                archive.extractall(temp, filter="data")
        with diagnostic_stage("owned-manifest"):
            node_root = temp / ("node-v" + NODE_VERSION + "-linux-x64")
            target, home = temp / "owned", temp / "home"
            target.mkdir(); home.mkdir()
            manifest_bytes = (source / "package.json").read_bytes()
            manifest = unique_json(manifest_bytes)
            if manifest != {"name": "ci-trust-owned-eslint-preparation", "version": "0.0.0", "private": True, "dependencies": {"eslint": "10.12.0"}}:
                raise ValueError("unexpected owned manifest")
            (target / "package.json").write_bytes(manifest_bytes)
        with diagnostic_stage("npm-setup"):
            flags = npm_install(node_root, target, home)
            lock_bytes = (target / "package-lock.json").read_bytes()
        with diagnostic_stage("base-image"):
            tree, base_manifest = base_tree()
        with diagnostic_stage("rootfs"):
            # Base is pinned and inventoried, not treated as an independently approved
            # runtime. Reject known development executables instead of inheriting defaults.
            for name in tree:
                if posixpath.basename(name) in {"sh", "bash", "dash", "npm", "npx", "gcc", "cc", "python", "python3"}:
                    raise ValueError("unexpected base executable")
            add_file(tree, "owned/node/bin/node", (node_root / "bin/node").read_bytes(), True)
            add_file(tree, "owned/node/LICENSE", (node_root / "LICENSE").read_bytes())
            add_file(tree, "ci-trust/owned-eslint", (prep / "owned-eslint").read_bytes(), True)
            add_file(tree, "owned/eslint.config.mjs", inputs["Module"].encode())
            tree["work"] = ("dir", 0o555, b"")
            count, total = 0, 0
            for directory, dirs, files in os.walk(target / "node_modules", followlinks=False):
                for name in dirs + files:
                    path = Path(directory) / name
                    if path.is_symlink():
                        raise ValueError("installed symlink requires review")
                for name in files:
                    path = Path(directory) / name
                    count += 1; total += path.stat().st_size
                    if count > MAX_FILES or total > 128 << 20 or not path.is_file():
                        raise ValueError("installed tree limit")
                    add_file(tree, "owned/" + path.relative_to(target).as_posix(), path.read_bytes())
        with diagnostic_stage("image-validation"):
            records = inventory(tree)
            image, identities = pack_image(tree)
            inspect_archive(image, records, identities)
            reproduced, repeated_ids = pack_image(tree)
            if reproduced != image or repeated_ids != identities:
                raise ValueError("nondeterministic archive serialization")
        with diagnostic_stage("artifact-write"):
            (artifacts / "image.tar").write_bytes(image)
            (artifacts / "package.json").write_bytes(manifest_bytes)
            (artifacts / "package-lock.json").write_bytes(lock_bytes)
            (artifacts / "inventory.json").write_bytes(canonical(records) + b"\n")
            (artifacts / "owned-inputs.json").write_bytes(canonical(inputs) + b"\n")
            metadata = dict(identities, status="PREPARATION_ONLY_NOT_ADMITTED", node_security_hold=True,
                            node_version=NODE_VERSION, npm_version=NPM_VERSION, eslint_version="10.12.0",
                            node_archive_sha256="sha256:" + NODE_SHA256, base_manifest=BASE_MANIFEST,
                            lock_sha256=digest(lock_bytes), launcher_sha256=digest((prep / "owned-eslint").read_bytes()),
                            manifest_sha256=digest(manifest_bytes), source_revision=os.environ["GITHUB_SHA"],
                            run_id=os.environ["GITHUB_RUN_ID"], run_attempt=os.environ["GITHUB_RUN_ATTEMPT"],
                            npm_flags=flags, base_layers=base_manifest["layers"],
                            serialization_check="same-process repeat; not independent offline reconstruction",
                            independent_policy_admission=False, real_tool_execution=False,
                            elf_library_closure="NOT_VERIFIED; static and runtime compatibility remain separate gates",
                            python_version=os.sys.version, limits="15 minute hosted job; 180 seconds per npm phase")
            (artifacts / "preparation.json").write_bytes(canonical(metadata) + b"\n")
            checksums = "".join(digest(p.read_bytes())[7:] + "  " + p.name + "\n" for p in sorted(artifacts.iterdir()))
            (artifacts / "SHA256SUMS").write_text(checksums, encoding="ascii")


if __name__ == "__main__":
    raise SystemExit(main())
