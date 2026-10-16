"""Offline owned synthetic inputs only: no package manager or network calls."""

import base64
import gzip
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
from types import SimpleNamespace
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("owned_prepare", Path(__file__).with_name("prepare.py"))
prep = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prep)


class PreparationTests(unittest.TestCase):
    def base_fixture(self, members=None):
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode="w") as tar:
            for name, mode, kind, target in members or [("owned", 0o555, tarfile.DIRTYPE, "")]:
                entry = tarfile.TarInfo(name)
                entry.mode, entry.type, entry.linkname = mode, kind, target
                tar.addfile(entry)
        raw = archive.getvalue()
        compressed = gzip.compress(raw)
        config = {"architecture": "amd64", "os": "linux", "rootfs": {"diff_ids": [prep.digest(raw)]}}
        manifest = {"config": {"digest": prep.digest(prep.canonical(config))},
                    "layers": [{"digest": prep.digest(compressed), "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip"}]}
        return manifest, config, compressed

    def run_base(self, manifest, config, compressed):
        # Mock every download: no network or untrusted code execution.
        with mock.patch.object(prep, "download", side_effect=[prep.canonical(manifest), prep.canonical(config), compressed]):
            with prep.diagnostic_stage("base-image"):
                return prep.base_tree()

    def test_base_success_and_precise_invariant_failures(self):
        manifest, config, compressed = self.base_fixture()
        tree, returned = self.run_base(manifest, config, compressed)
        self.assertEqual(tree, {"owned": ("dir", 0o555, b"")})
        self.assertEqual(returned, manifest)
        for mutation, stage, reason in [
            (lambda m, c: c.update(architecture="arm64"), "base-platform", "platform"),
            (lambda m, c: c["rootfs"].update(diff_ids=[]), "base-layers", "layer-count"),
            (lambda m, c: m["layers"][0].update(mediaType="untrusted"), "base-layers", "layer-media-type"),
            (lambda m, c: c["rootfs"].update(diff_ids=["sha256:" + "0" * 64]), "base-layer-diff", "layer-diff-id"),
        ]:
            manifest, config, compressed = self.base_fixture()
            mutation(manifest, config)
            with self.subTest(reason=reason), self.assertRaises(prep.PreparationFailure) as caught:
                self.run_base(manifest, config, compressed)
            self.assertEqual((caught.exception.stage, caught.exception.category, caught.exception.reason), (stage, "validation", reason))

    def test_base_download_and_parse_failures_retain_substage(self):
        manifest, config, compressed = self.base_fixture()
        for index, stage in enumerate(("base-manifest", "base-config", "base-layer-download")):
            values = [prep.canonical(manifest), prep.canonical(config), compressed]
            values[index] = prep.ValidationFailure("download-identity")
            with self.subTest(stage=stage), mock.patch.object(prep, "download", side_effect=values):
                with self.assertRaises(prep.PreparationFailure) as caught:
                    with prep.diagnostic_stage("base-image"):
                        prep.base_tree()
                self.assertEqual((caught.exception.stage, caught.exception.reason), (stage, "download-identity"))
        for values, stage in [
            ([b'{"x":1,"x":2}'], "base-manifest"),
            ([prep.canonical(manifest), b'{"x":1,"x":2}'], "base-config"),
        ]:
            with mock.patch.object(prep, "download", side_effect=values), self.assertRaises(prep.PreparationFailure) as caught:
                prep.base_tree()
            self.assertEqual((caught.exception.stage, caught.exception.reason), (stage, "json-duplicate-key"))
        with self.assertRaises(prep.PreparationFailure) as caught:
            self.run_base(manifest, config, b"not gzip")
        self.assertEqual(caught.exception.stage, "base-layer-decompress")

    def test_base_import_preserves_fail_closed_invariant_codes(self):
        directory = ("owned", 0o555, tarfile.DIRTYPE, "")
        cases = [
            ([directory, directory], "layer-duplicate-or-limit"),
            ([("../escape", 0o444, tarfile.REGTYPE, "")], "path-unsafe"),
            ([("privileged", 0o4755, tarfile.REGTYPE, "")], "layer-privileged"),
            ([(".wh.deleted", 0o444, tarfile.REGTYPE, "")], "layer-whiteout"),
            ([("device", 0o444, tarfile.CHRTYPE, "")], "layer-special-entry"),
            ([("link", 0o777, tarfile.SYMTYPE, "../../escape")], "path-unsafe"),
            ([("file", 0o444, tarfile.REGTYPE, ""), ("file/child", 0o444, tarfile.REGTYPE, "")], "layer-ancestor"),
        ]
        for members, reason in cases:
            with self.subTest(reason=reason), self.assertRaises(prep.PreparationFailure) as caught:
                self.run_base(*self.base_fixture(members))
            self.assertEqual((caught.exception.stage, caught.exception.category, caught.exception.reason), ("base-layer-import", "validation", reason))

    def test_reason_codes_are_revalidated_and_never_expose_input(self):
        secret = "secret\n::error::injected https://user:password@private.test"
        for reason in sorted(prep.REASONS):
            def fail():
                with prep.diagnostic_stage("base-image"):
                    with prep.diagnostic_stage("base-layer-import"):
                        raise prep.ValidationFailure(reason)
            output = io.StringIO()
            with mock.patch.object(prep, "prepare", side_effect=fail), mock.patch.object(prep.os.sys, "stderr", output):
                self.assertEqual(prep.main(), 1)
            self.assertEqual(output.getvalue(), "owned lint preparation failed; stage=base-layer-import; category=validation; reason=" + reason + "; no runtime admission or ESLint execution\n")
        invalid = prep.PreparationFailure("base-layer-import", "validation", secret)
        invalid.reason = secret
        with mock.patch.object(prep, "prepare", side_effect=invalid), mock.patch.object(prep.os.sys, "stderr", io.StringIO()) as output:
            self.assertEqual(prep.main(), 1)
        self.assertNotIn(secret, output.getvalue())
        self.assertNotIn("reason=", output.getvalue())
        self.assertIsNone(prep.ValidationFailure(secret).reason)

    def test_stage_failures_are_bounded_and_do_not_expose_error_text(self):
        secret = "https://user:secret@private.test/path\n::error::injected /private/file"
        for stage in sorted(prep.STAGES):
            def fail():
                with prep.diagnostic_stage(stage):
                    raise ValueError(secret)
            output = io.StringIO()
            with self.subTest(stage=stage), mock.patch.object(prep, "prepare", side_effect=fail), mock.patch.object(prep.os.sys, "stderr", output):
                self.assertEqual(prep.main(), 1)
            self.assertEqual(output.getvalue(), "owned lint preparation failed; stage=" + stage + "; category=validation; no runtime admission or ESLint execution\n")

    def test_error_categories_never_require_exception_text(self):
        class UnprintableError(Exception):
            def __str__(self):
                raise AssertionError("must not stringify errors")
        cases = [
            (prep.urllib.error.HTTPError("private", 403, "secret", {}, None), "http"),
            (prep.urllib.error.URLError("secret"), "network"),
            (TimeoutError("secret"), "timeout"),
            (prep.subprocess.TimeoutExpired(["secret"], 180, output=b"secret"), "timeout"),
            (FileNotFoundError("secret"), "filesystem"),
            (tarfile.ReadError("secret"), "archive"),
            (KeyError("secret"), "validation"),
            (UnprintableError(), "unexpected"),
        ]
        for error, category in cases:
            with self.subTest(category=category):
                self.assertEqual(prep.error_category(error), category)
                with self.assertRaises(prep.PreparationFailure) as caught:
                    with prep.diagnostic_stage("base-image"):
                        raise error
                self.assertEqual(str(caught.exception), "base-image:" + category)

    def test_nested_stage_preserves_innermost_failure(self):
        with self.assertRaises(prep.PreparationFailure) as caught:
            with prep.diagnostic_stage("npm-setup"):
                with prep.diagnostic_stage("npm-ci"):
                    raise ValueError("secret")
        self.assertEqual(str(caught.exception), "npm-ci:validation")

    def test_main_revalidates_structured_errors_and_handles_unknown_failures(self):
        invalid = prep.PreparationFailure("npm-ci", "process-failed")
        invalid.stage = "secret\n::error::injected"
        invalid.category = "private"
        for error, category in ((invalid, "unexpected"), (OSError("private"), "filesystem")):
            output = io.StringIO()
            with mock.patch.object(prep, "prepare", side_effect=error), mock.patch.object(prep.os.sys, "stderr", output):
                self.assertEqual(prep.main(), 1)
            self.assertEqual(output.getvalue(), "owned lint preparation failed; stage=unknown; category=" + category + "; no runtime admission or ESLint execution\n")
        with mock.patch.object(prep, "prepare"), mock.patch.object(prep.os.sys, "stderr", io.StringIO()) as output:
            self.assertEqual(prep.main(), 0)
            self.assertEqual(output.getvalue(), "")
        with self.assertRaises(prep.PreparationFailure):
            with prep.diagnostic_stage("untrusted-stage"):
                self.fail("invalid stage must not execute")

    def test_reproducible_archive_and_independent_decode(self):
        tree = {"work": ("dir", 0o555, b""), "owned/config": ("file", 0o444, b"owned\n")}
        original = dict(tree)
        image, identities = prep.pack_image(tree)
        repeated, _ = prep.pack_image(tree)
        self.assertEqual(image, repeated)
        self.assertEqual(tree, original)
        prep.inspect_archive(image, prep.inventory(tree), identities)
        with self.assertRaises(ValueError):
            prep.inspect_archive(image, [], identities)
        changed = dict(identities, image_id="sha256:" + "b" * 64)
        with self.assertRaises(ValueError):
            prep.inspect_archive(image, prep.inventory(tree), changed)
        with self.assertRaises((ValueError, tarfile.TarError)):
            prep.inspect_archive(image[:-len(image)//2], prep.inventory(tree), identities)

    def test_rejects_paths_links_and_duplicates(self):
        for name in ("", "/absolute", "../outside", "a/../../b", "a\\b", "a//b", "a/./b", "bad\nname"):
            with self.subTest(name=name), self.assertRaises(ValueError):
                prep.safe_path(name)
        with self.assertRaises(ValueError):
            prep.link_path("a", "../../outside")
        self.assertEqual(prep.link_path("lib64/loader", "/lib/loader"), "lib/loader")
        with self.assertRaises(ValueError):
            prep.unique_json(b'{"same":1,"same":2}')
        with self.assertRaises(ValueError):
            prep.resolve({"a": ("link", 0o777, "b"), "b": ("link", 0o777, "a")}, "a")

    def test_rejects_archive_special_privileged_and_shadowed_entries(self):
        for kind, mode, path in ((tarfile.CHRTYPE, 0o444, "device"), (tarfile.REGTYPE, 0o4755, "setuid"), (tarfile.REGTYPE, 0o444, "../outside"), (tarfile.REGTYPE, 0o444, "a/.wh.hidden")):
            output = io.BytesIO()
            with tarfile.open(fileobj=output, mode="w") as archive:
                member = tarfile.TarInfo(path)
                member.type, member.mode = kind, mode
                archive.addfile(member)
            with self.assertRaises(ValueError):
                prep.overlay({}, output.getvalue())
        data = prep.pack_tar({"link/child": ("file", 0o444, b"x")})
        with self.assertRaises(ValueError):
            prep.overlay({"link": ("link", 0o777, "elsewhere")}, data)
        output = io.BytesIO()
        with tarfile.open(fileobj=output, mode="w") as archive:
            archive.addfile(tarfile.TarInfo("duplicate"))
            archive.addfile(tarfile.TarInfo("duplicate"))
        with self.assertRaises(ValueError):
            prep.overlay({}, output.getvalue())

    def test_registry_lock_gate(self):
        package = {"version": "10.12.0", "resolved": "https://registry.npmjs.org/eslint/-/eslint-10.12.0.tgz", "integrity": "sha512-" + base64.b64encode(b"x" * 64).decode()}
        lock = {"lockfileVersion": 3, "packages": {"": {"dependencies": {"eslint": "10.12.0"}}, "node_modules/eslint": package}}
        prep.validate_lock(lock)
        for key, value in (("resolved", "https://other.test/file"), ("resolved", "file:/etc/private"), ("resolved", "https://registry.npmjs.org@other.test/file"), ("integrity", "sha512-bad"), ("version", "*"), ("link", True)):
            candidate = json.loads(json.dumps(lock))
            candidate["packages"]["node_modules/eslint"][key] = value
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                prep.validate_lock(candidate)

    def test_download_integrity_and_origin_before_network(self):
        for url in ("http://nodejs.org/file", "https://private.test/file", "https://user:pass@nodejs.org/file", "file:///etc/private"):
            with mock.patch.object(prep.urllib.request, "build_opener", side_effect=AssertionError("network")):
                with self.assertRaises(ValueError):
                    prep.download(url, "sha256:" + "a" * 64)
        fake = mock.MagicMock()
        fake.open.return_value.__enter__.return_value.read.return_value = b"changed"
        with mock.patch.object(prep.urllib.request, "build_opener", return_value=fake):
            with self.assertRaises(ValueError):
                prep.download("https://nodejs.org/file", "sha256:" + "a" * 64)

    def test_owned_file_ancestor_and_collision(self):
        tree = {}
        prep.add_file(tree, "owned/config", b"x")
        self.assertEqual(tree["owned"][0], "dir")
        with self.assertRaises(ValueError):
            prep.add_file(tree, "owned/config", b"y")
        with self.assertRaises(ValueError):
            prep.add_file({"owned": ("link", 0o777, "elsewhere")}, "owned/config", b"x")

    def test_proxy_denies_other_origins_without_connecting(self):
        handler = object.__new__(prep.RegistryProxy)
        handler.path = "outside.test:443"
        handler.server = SimpleNamespace(denied=False)
        handler.send_error = mock.MagicMock()
        with mock.patch.object(prep.socket, "create_connection", side_effect=AssertionError("network")):
            handler.do_CONNECT()
        self.assertTrue(handler.server.denied)
        handler.send_error.assert_called_once_with(403)

    def test_npm_is_the_only_executed_javascript_and_scripts_are_disabled(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            node_root, target, home = root / "node", root / "owned", root / "home"
            (node_root / "lib/node_modules/npm").mkdir(parents=True)
            (node_root / "lib/node_modules/npm/package.json").write_text(json.dumps({"version": prep.NPM_VERSION}))
            target.mkdir(); home.mkdir()
            lock = {"lockfileVersion": 3, "packages": {"": {"dependencies": {"eslint": "10.12.0"}}, "node_modules/eslint": {"version": "10.12.0", "resolved": "https://registry.npmjs.org/eslint/-/eslint-10.12.0.tgz", "integrity": "sha512-" + base64.b64encode(b"x"*64).decode()}}}
            (target / "package-lock.json").write_text(json.dumps(lock))
            server = mock.MagicMock(server_port=1234)
            with mock.patch.object(prep.http.server, "ThreadingHTTPServer", return_value=server), mock.patch.object(prep.subprocess, "run", return_value=SimpleNamespace(returncode=0)) as run:
                prep.npm_install(node_root, target, home)
                self.assertEqual(run.call_count, 2)
                for invocation in run.call_args_list:
                    command = invocation.args[0]
                    self.assertEqual(command[:2], [str(node_root / "bin/node"), str(node_root / "lib/node_modules/npm/bin/npm-cli.js")])
                    for flag in ("--ignore-scripts", "--omit=optional", "--omit=peer", "--no-audit", "--no-bin-links"):
                        self.assertIn(flag, command)
                    env = invocation.kwargs["env"]
                    self.assertEqual(env["PATH"], str(node_root / "bin"))
                    for forbidden in ("GITHUB_TOKEN", "NODE_OPTIONS", "SSH_AUTH_SOCK", "NPM_TOKEN"):
                        self.assertNotIn(forbidden, env)
                    self.assertEqual(invocation.kwargs["timeout"], 180)
            server.shutdown.assert_called_once()
            # Mock both npm phases: no Node process, socket or network is used.
            cases = [
                ([SimpleNamespace(returncode=1)], False, "npm-lock", "process-failed"),
                ([SimpleNamespace(returncode=0), SimpleNamespace(returncode=1)], False, "npm-ci", "process-failed"),
                ([prep.subprocess.TimeoutExpired(["private"], 180)], False, "npm-lock", "timeout"),
                ([SimpleNamespace(returncode=0)], True, "npm-lock", "origin-denied"),
            ]
            for outcomes, denied, stage, category in cases:
                server.reset_mock()
                def execute(*args, **kwargs):
                    server.denied = denied
                    outcome = outcomes.pop(0)
                    if isinstance(outcome, Exception):
                        raise outcome
                    return outcome
                with self.subTest(stage=stage, category=category), mock.patch.object(prep.http.server, "ThreadingHTTPServer", return_value=server), mock.patch.object(prep.subprocess, "run", side_effect=execute):
                    with self.assertRaises(prep.PreparationFailure) as caught:
                        prep.npm_install(node_root, target, home)
                    self.assertEqual(str(caught.exception), stage + ":" + category)
                server.shutdown.assert_called_once()
                server.server_close.assert_called_once()


if __name__ == "__main__":
    unittest.main()
