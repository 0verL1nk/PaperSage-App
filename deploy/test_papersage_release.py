import contextlib
import gzip
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
import tarfile
import tempfile
import types
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("papersage-release.py")
SPEC = importlib.util.spec_from_file_location("papersage_release", SCRIPT)
release = importlib.util.module_from_spec(SPEC)
if SCRIPT.exists():
    SPEC.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(SCRIPT.exists(), "release helper has not been implemented")
        self.stack = contextlib.ExitStack()
        self.addCleanup(self.stack.close)
        self.root = Path(self.stack.enter_context(tempfile.TemporaryDirectory()))
        paths = {
            "APP_ROOT": self.root / "opt",
            "RELEASES": self.root / "opt" / "releases",
            "CURRENT": self.root / "opt" / "current",
            "UPLOADS": self.root / "deploy" / "uploads",
            "LOCK_FILE": self.root / "deploy" / "release.lock",
            "ENV_FILE": self.root / "etc" / "papersage.env",
            "WORK_DIR": self.root / "state",
        }
        for name, value in paths.items():
            self.stack.enter_context(mock.patch.object(release, name, value))
        # mkdir(parents=True) does not apply its mode to intermediate directories under umask 0002.
        for path in (release.APP_ROOT, release.LOCK_FILE.parent):
            path.mkdir(mode=0o755)
            path.chmod(0o755)
        for path in (release.RELEASES, release.UPLOADS, release.ENV_FILE.parent, release.WORK_DIR):
            path.mkdir(parents=True, mode=0o755)
        release.ENV_FILE.write_text("DATABASE_URL=do-not-print-this-secret\n")
        release.ENV_FILE.chmod(0o600)
        self.stack.enter_context(mock.patch.object(release.os, "geteuid", return_value=0, create=True))
        # Temporary fixtures belong to the runner, so emulate root ownership without requiring chown.
        def root_metadata(info):
            fields = list(info)
            fields[stat.ST_UID] = 0
            return os.stat_result(fields)
        if os.name != "nt":
            # Path.lstat delegates to os.stat(follow_symlinks=False), not os.lstat.
            lstat, fstat = Path.lstat, os.fstat
            self.stack.enter_context(mock.patch.object(Path, "lstat", autospec=True, side_effect=lambda *a, **kw: root_metadata(lstat(*a, **kw))))
            self.stack.enter_context(mock.patch.object(release.os, "fstat", side_effect=lambda *a, **kw: root_metadata(fstat(*a, **kw))))
            nonroot = list(lstat(self.root))
            nonroot[stat.ST_UID] = 1000
            with mock.patch.object(Path, "stat", return_value=os.stat_result(nonroot)):
                self.assertEqual(self.root.lstat().st_uid, 0, "fixture must normalize the Path.lstat ownership boundary")
            with release.ENV_FILE.open("rb") as environment:
                self.assertEqual(os.fstat(environment.fileno()).st_uid, 0, "fixture must normalize the fstat ownership boundary")
            for path in (release.APP_ROOT, release.LOCK_FILE.parent):
                self.assertEqual(path.lstat().st_mode & 0o022, 0, "trusted fixture parents must not be group/world writable")
        # Windows cannot model Unix ownership/mode checks; those have a separate stat-boundary test.
        self.private_environment_check = release.check_private_environment
        if os.name == "nt":
            self.stack.enter_context(mock.patch.object(release, "check_root_owned"))
            self.stack.enter_context(mock.patch.object(release, "check_private_environment"))
            readlink = os.readlink
            self.stack.enter_context(mock.patch.object(release.os, "readlink", side_effect=lambda path: readlink(path).removeprefix("\\\\?\\")))
            # Windows cannot atomically replace directory symlinks; emulate only this OS boundary.
            replace = os.replace
            def replace_link(source, destination):
                Path(destination).unlink(missing_ok=True)
                replace(source, destination)
            self.stack.enter_context(mock.patch.object(release.os, "replace", side_effect=replace_link))
        self.fcntl = types.SimpleNamespace(LOCK_EX=2, LOCK_UN=8, flock=mock.Mock())
        self.stack.enter_context(mock.patch.dict(sys.modules, {"fcntl": self.fcntl}))
        self.system = self.stack.enter_context(mock.patch.object(release.subprocess, "run"))
        self.system.return_value = subprocess.CompletedProcess([], 0)
        self.health = self.stack.enter_context(mock.patch.object(release, "wait_healthy"))
        self.commit = "a" * 40
        self.version = "v1.2.3"

    def cli(self, args):
        output = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
            code = release.main(args)
        return code, output.getvalue()

    def upload(self, entries=None):
        path = release.UPLOADS / f"{self.version}-{self.commit}.tar.gz"
        if entries is None:
            entries = [("papersage", b"app", tarfile.REGTYPE), ("migrate", b"migration", tarfile.REGTYPE)]
        with tarfile.open(path, "w:gz") as archive:
            for name, body, kind in entries:
                member = tarfile.TarInfo(name)
                member.type = kind
                member.mode = 0o6777
                if kind in (tarfile.SYMTYPE, tarfile.LNKTYPE):
                    member.linkname = "/etc/shadow"
                member.size = len(body) if member.isfile() else 0
                archive.addfile(member, io.BytesIO(body) if member.isfile() else None)
        return hashlib.sha256(path.read_bytes()).hexdigest()

    def existing(self, version, commit=None, checksum=None):
        path = release.RELEASES / version
        path.mkdir(mode=0o755)
        for name in ("papersage", "migrate"):
            (path / name).write_bytes(name.encode())
            (path / name).chmod(0o555)
        (path / "manifest.json").write_text(json.dumps({
            "version": version,
            "commit": commit or "b" * 40,
            "archive_sha256": checksum or "c" * 64,
        }))
        (path / "manifest.json").chmod(0o444)
        return path

    def current(self, path):
        os.symlink(path, release.CURRENT, target_is_directory=True)

    def commands(self):
        return [call.args[0] for call in self.system.call_args_list]

    def test_bad_arguments_do_not_touch_system(self):
        cases = [[], ["activate"], ["rollback", "../bad"], ["rollback", "v01.2.3"],
                 ["activate", "v1.2.3-rc1", self.commit, "c" * 64],
                 ["activate", self.version, "a" * 39, "c" * 64],
                 ["activate", self.version, "A" * 40, "c" * 64],
                 ["activate", self.version, self.commit, "c" * 63],
                 ["activate", self.version, self.commit, "g" * 64],
                 ["rollback", self.version, "extra"]]
        for args in cases:
            with self.subTest(args=args):
                code, output = self.cli(args)
                self.assertNotEqual(code, 0)
                self.assertRegex(output, r"release error: (usage:|invalid release version, commit or checksum)")
        self.system.assert_not_called()
        self.assertFalse(release.LOCK_FILE.exists())

    def test_readable_environment_is_rejected_before_any_service_command(self):
        original = release.trusted
        def inspect_path(path, directory=False):
            if path == release.ENV_FILE:
                return types.SimpleNamespace(st_mode=stat.S_IFREG | 0o644)
            return original(path, directory)
        with mock.patch.object(release, "trusted", side_effect=inspect_path), \
                mock.patch.object(release, "check_private_environment", wraps=self.private_environment_check) as check:
            code, output = self.cli(["activate", self.version, self.commit, "c" * 64])
        self.assertNotEqual(code, 0)
        self.assertIn("only to root", output)
        check.assert_called_once()
        self.system.assert_not_called()
        self.assertFalse(os.path.lexists(release.CURRENT))

    def test_non_root_caller_cannot_acquire_release_lock_or_stop_service(self):
        with mock.patch.object(release.os, "geteuid", return_value=1000):
            code, output = self.cli(["activate", self.version, self.commit, "c" * 64])
        self.assertNotEqual(code, 0)
        self.assertIn("must run as root", output)
        self.system.assert_not_called()
        self.assertFalse(release.LOCK_FILE.exists())

    def test_checksum_mismatch_does_not_stop_or_install(self):
        self.upload()
        code, output = self.cli(["activate", self.version, self.commit, "0" * 64])
        self.assertNotEqual(code, 0)
        self.assertIn("archive checksum mismatch", output)
        self.system.assert_not_called()
        self.assertEqual(list(release.RELEASES.iterdir()), [])

    def test_unsafe_archive_members_are_rejected(self):
        valid = [("papersage", b"app", tarfile.REGTYPE), ("migrate", b"migration", tarfile.REGTYPE)]
        for entry in [("../escape", b"x", tarfile.REGTYPE), ("/etc/passwd", b"x", tarfile.REGTYPE),
                      ("papersage", b"", tarfile.SYMTYPE), ("migrate", b"", tarfile.LNKTYPE),
                      ("extra", b"x", tarfile.REGTYPE), ("papersage", b"again", tarfile.REGTYPE),
                      ("folder", b"", tarfile.DIRTYPE), ("fifo", b"", tarfile.FIFOTYPE)]:
            with self.subTest(entry=entry[0:3:2]):
                checksum = self.upload(valid + [entry])
                code, output = self.cli(["activate", self.version, self.commit, checksum])
                self.assertNotEqual(code, 0)
                self.assertIn("archive must contain only the two regular binaries", output)
                self.assertEqual(list(release.RELEASES.iterdir()), [])
        self.system.assert_not_called()
        self.assertFalse((self.root / "escape").exists())

    def test_members_hidden_after_tar_end_or_in_second_gzip_stream_are_rejected(self):
        for concatenate_gzip in (False, True):
            with self.subTest(concatenate_gzip=concatenate_gzip):
                self.upload()
                path = release.UPLOADS / f"{self.version}-{self.commit}.tar.gz"
                original = path.read_bytes()
                extra_buffer = io.BytesIO()
                with tarfile.open(fileobj=extra_buffer, mode="w:") as archive:
                    info = tarfile.TarInfo("extra")
                    info.size = 1
                    archive.addfile(info, io.BytesIO(b"x"))
                extra = extra_buffer.getvalue()
                path.write_bytes(original + gzip.compress(extra) if concatenate_gzip
                                 else gzip.compress(gzip.decompress(original) + extra))
                checksum = hashlib.sha256(path.read_bytes()).hexdigest()
                code, output = self.cli(["activate", self.version, self.commit, checksum])
                self.assertNotEqual(code, 0, output)
                self.assertIn("archive must contain only the two regular binaries", output)
                self.assertEqual(list(release.RELEASES.iterdir()), [])
        self.system.assert_not_called()

    def test_compressed_padding_bomb_is_bounded_before_tar_parsing(self):
        self.upload()
        path = release.UPLOADS / f"{self.version}-{self.commit}.tar.gz"
        path.write_bytes(gzip.compress(b"\\0" * (2 * 1024 * 1024)))
        checksum = hashlib.sha256(path.read_bytes()).hexdigest()
        with mock.patch.object(release, "MAX_RELEASE_BYTES", 0):
            code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertIn("unpacked archive is too large", output)
        self.system.assert_not_called()

    def test_missing_binary_is_rejected(self):
        checksum = self.upload([("papersage", b"app", tarfile.REGTYPE)])
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertIn("archive is missing a required binary", output)
        self.system.assert_not_called()

    def test_activate_stops_migrates_then_switches_and_starts(self):
        old = self.existing("v1.2.2")
        self.current(old)
        checksum = self.upload()
        observed = []
        self.system.side_effect = lambda command, **kwargs: observed.append((command, release.CURRENT.resolve()))
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertEqual(code, 0, output)
        new = release.RELEASES / self.version
        self.assertEqual(release.CURRENT.resolve(), new)
        self.assertEqual([command[0] for command, _ in observed],
                         ["/usr/bin/systemctl", "/usr/bin/systemd-run", "/usr/bin/systemctl"])
        self.assertEqual(observed[0][0][-2:], ["stop", "papersage.service"])
        self.assertEqual(observed[0][1], old)
        self.assertEqual(observed[1][1], old)
        self.assertEqual(observed[2][1], new)
        migrate = observed[1][0]
        for flag in ("--wait", "--collect", "--uid=papersage", "--gid=papersage",
                     f"--property=EnvironmentFile={release.ENV_FILE}", f"--working-directory={release.WORK_DIR}"):
            self.assertIn(flag, migrate)
        self.assertEqual(migrate[-1], str(new / "migrate"))
        self.assertNotIn("source", " ".join(migrate))
        self.assertEqual(json.loads((new / "manifest.json").read_text())["archive_sha256"], checksum)
        self.assertEqual((new / "papersage").read_bytes(), b"app")
        if os.name != "nt":
            self.assertEqual(stat.S_IMODE((new / "papersage").stat().st_mode), 0o555)
        self.fcntl.flock.assert_has_calls([mock.call(mock.ANY, self.fcntl.LOCK_EX),
                                        mock.call(mock.ANY, self.fcntl.LOCK_UN)])

    def test_health_failure_restores_previous_release_and_checks_it(self):
        old = self.existing("v1.2.2")
        self.current(old)
        self.health.side_effect = [release.DeployError("health check failed"), None]
        checksum = self.upload()
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertIn("previous release restored", output)
        self.assertEqual(release.CURRENT.resolve(), old)
        self.assertEqual([command[1:] for command in self.commands()
                          if command[0] == "/usr/bin/systemctl" and command[1] in {"start", "stop"}],
                         [["stop", "papersage.service"], ["start", "papersage.service"],
                          ["stop", "papersage.service"], ["start", "papersage.service"]])
        self.assertEqual(self.health.call_args_list[-1].args[0]["version"], "v1.2.2")

    def test_first_release_failure_removes_current_without_claiming_recovery(self):
        self.health.side_effect = release.DeployError("health check failed")
        checksum = self.upload()
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertIn("no previous release", output)
        self.assertNotIn("previous release restored", output)
        self.assertFalse(os.path.lexists(release.CURRENT))
        self.assertEqual(sum(command[1] == "start" for command in self.commands()
                             if command[0] == "/usr/bin/systemctl"), 1)

    def test_failed_existing_recovery_is_reported_as_failure(self):
        self.current(self.existing("v1.2.2"))
        self.health.side_effect = release.DeployError("health check failed")
        checksum = self.upload()
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertIn("recovery failed", output)
        self.assertNotIn("previous release restored", output)

    def test_same_version_different_identity_cannot_overwrite(self):
        old = self.existing(self.version)
        before = {p.name: p.read_bytes() for p in old.iterdir()}
        checksum = self.upload()
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertIn("version already exists with a different commit or checksum", output)
        self.assertEqual({p.name: p.read_bytes() for p in old.iterdir()}, before)
        self.system.assert_not_called()

    def test_failed_activation_can_reuse_identical_release_without_overwrite(self):
        checksum = self.upload()
        installed = self.existing(self.version, self.commit, checksum)
        before = {p.name: p.read_bytes() for p in installed.iterdir()}
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertEqual(code, 0, output)
        self.assertEqual({p.name: p.read_bytes() for p in installed.iterdir()}, before)
        self.assertEqual(release.CURRENT.resolve(), installed)
        self.assertEqual(sum(command[0] == "/usr/bin/systemd-run" for command in self.commands()), 1)

    def test_current_healthy_identical_release_is_idempotent_without_migration(self):
        checksum = self.upload()
        installed = self.existing(self.version, self.commit, checksum)
        self.current(installed)
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertEqual(code, 0, output)
        self.system.assert_not_called()
        self.health.assert_called_once()

    def test_rollback_only_uses_installed_release_and_never_migrates(self):
        target = self.existing("v1.1.0")
        self.current(self.existing("v1.2.2"))
        code, output = self.cli(["rollback", "v1.1.0"])
        self.assertEqual(code, 0, output)
        self.assertEqual(release.CURRENT.resolve(), target)
        self.assertTrue(all(command[0] == "/usr/bin/systemctl" for command in self.commands()))
        self.assertEqual(len(self.commands()), 2)

    def test_rollback_rejects_missing_or_mismatched_manifest(self):
        with mock.patch.object(release, "load_release", wraps=release.load_release) as load:
            self.assertNotEqual(self.cli(["rollback", self.version])[0], 0)
            load.assert_called_once_with(self.version)
        target = self.existing(self.version)
        (target / "manifest.json").chmod(0o644)
        (target / "manifest.json").write_text(json.dumps({"version": "v9.9.9"}))
        code, output = self.cli(["rollback", self.version])
        self.assertNotEqual(code, 0)
        self.assertIn("invalid release manifest", output)
        self.system.assert_not_called()

    def test_start_failure_restores_previous_release(self):
        old = self.existing("v1.2.2")
        self.current(old)
        failed = False
        def fail_first_start(command, **kwargs):
            nonlocal failed
            if command == ["/usr/bin/systemctl", "start", "papersage.service"] and not failed:
                failed = True
                raise subprocess.CalledProcessError(1, command)
        self.system.side_effect = fail_first_start
        checksum = self.upload()
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertIn("previous release restored", output)
        self.assertEqual(release.CURRENT.resolve(), old)
        self.assertIn(["/usr/bin/systemctl", "reset-failed", "papersage.service"], self.commands())

    def test_upload_symlink_is_rejected(self):
        self.upload()
        upload = release.UPLOADS / f"{self.version}-{self.commit}.tar.gz"
        upload.unlink()
        os.symlink(release.ENV_FILE, upload)
        code, output = self.cli(["activate", self.version, self.commit, "0" * 64])
        self.assertNotEqual(code, 0)
        self.assertIn("upload must be a regular, non-linked file", output)
        self.system.assert_not_called()
        self.assertNotIn("do-not-print", output)

    def test_rollback_rejects_symlinked_binary(self):
        target = self.existing(self.version)
        (target / "papersage").chmod(0o755)
        (target / "papersage").unlink()
        os.symlink(release.ENV_FILE, target / "papersage")
        with mock.patch.object(release, "trusted", wraps=release.trusted) as inspected:
            code, output = self.cli(["rollback", self.version])
        self.assertNotEqual(code, 0, output)
        self.assertEqual(inspected.call_args.args[0], target / "papersage")
        self.system.assert_not_called()

    def test_rollback_rejects_symlinked_release_directory(self):
        external = self.existing("v1.0.0")
        os.symlink(external, release.RELEASES / self.version, target_is_directory=True)
        with mock.patch.object(release, "trusted", wraps=release.trusted) as inspected:
            code, output = self.cli(["rollback", self.version])
        self.assertNotEqual(code, 0, output)
        self.assertEqual(inspected.call_args.args[0], release.RELEASES / self.version)
        self.system.assert_not_called()

    def test_archive_is_extracted_from_verified_private_snapshot(self):
        checksum = self.upload()
        tar_open = tarfile.open
        upload = release.UPLOADS / f"{self.version}-{self.commit}.tar.gz"
        def alter_original_then_unpack(path, *args, **kwargs):
            upload.write_bytes(b"different upload after checksum")
            return tar_open(path, *args, **kwargs)
        with mock.patch.object(release.tarfile, "open", side_effect=alter_original_then_unpack):
            code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertEqual(code, 0, output)
        self.assertEqual((release.RELEASES / self.version / "papersage").read_bytes(), b"app")

    @unittest.skipIf(os.name == "nt", "Unix umask/execute permissions are not modeled by Windows")
    def test_restrictive_caller_umask_does_not_make_release_inaccessible(self):
        checksum = self.upload()
        previous = os.umask(0o077)
        try:
            code, output = self.cli(["activate", self.version, self.commit, checksum])
        finally:
            os.umask(previous)
        self.assertEqual(code, 0, output)
        self.assertEqual(stat.S_IMODE((release.RELEASES / self.version).stat().st_mode), 0o755)

    def test_same_commit_different_checksum_cannot_overwrite(self):
        installed = self.existing(self.version, self.commit, "c" * 64)
        before = (installed / "papersage").read_bytes()
        code, output = self.cli(["activate", self.version, self.commit, "d" * 64])
        self.assertNotEqual(code, 0)
        self.assertIn("version already exists with a different commit or checksum", output)
        self.assertEqual((installed / "papersage").read_bytes(), before)
        self.system.assert_not_called()

    def test_migration_timeout_releases_lock_and_restores_old_service_without_schema_claim(self):
        old = self.existing("v1.2.2")
        self.current(old)
        def time_out_migration(command, **kwargs):
            if command[0] == "/usr/bin/systemd-run":
                raise subprocess.TimeoutExpired(command, 360)
        self.system.side_effect = time_out_migration
        checksum = self.upload()
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertEqual(release.CURRENT.resolve(), old)
        self.assertIn("previous release restored", output)
        self.assertIn("schema was not rolled back", output)
        self.fcntl.flock.assert_has_calls([mock.call(mock.ANY, self.fcntl.LOCK_EX),
                                        mock.call(mock.ANY, self.fcntl.LOCK_UN)])
        for call in self.system.call_args_list:
            self.assertIsInstance(call.kwargs.get("timeout"), int)
            self.assertGreater(call.kwargs["timeout"], 0)
        migration = next(call for call in self.system.call_args_list
                         if call.args[0][0] == "/usr/bin/systemd-run")
        self.assertIn("--property=RuntimeMaxSec=300s", migration.args[0])
        self.assertIn("--property=TimeoutStopSec=30s", migration.args[0])
        self.assertEqual(migration.kwargs["timeout"], 360)

    def test_service_command_wait_is_bounded(self):
        release.service("stop")
        self.assertEqual(self.system.call_args.kwargs.get("timeout"), 120)

    def test_migration_error_restores_old_service_and_redacts_output(self):
        old = self.existing("v1.2.2")
        self.current(old)
        def fail_migration(command, **kwargs):
            if command[0] == "/usr/bin/systemd-run":
                raise subprocess.CalledProcessError(1, command, stderr="DATABASE_URL=secret-from-database")
        self.system.side_effect = fail_migration
        checksum = self.upload()
        code, output = self.cli(["activate", self.version, self.commit, checksum])
        self.assertNotEqual(code, 0)
        self.assertEqual(release.CURRENT.resolve(), old)
        self.assertIn("previous release restored", output)
        self.assertNotIn("secret", output)
        self.assertNotIn("DATABASE_URL", output)
        for call in self.system.call_args_list:
            self.assertEqual(call.kwargs["stdout"], subprocess.DEVNULL)
            self.assertEqual(call.kwargs["stderr"], subprocess.DEVNULL)
            self.assertNotIn("DATABASE_URL", call.kwargs["env"])


class HealthAndTrustTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(SCRIPT.exists(), "release helper has not been implemented")

    def test_health_requires_healthy_unprefixed_version_and_full_commit(self):
        manifest = {"version": "v1.2.3", "commit": "a" * 40}
        for payload, accepted in [
            ({"status": "healthy", "version": "1.2.3", "commit": "a" * 40}, True),
            ({"status": "ok", "version": "1.2.3", "commit": "a" * 40}, False),
            ({"status": "healthy", "version": "v1.2.3", "commit": "a" * 40}, False),
            ({"status": "healthy", "version": "1.2.3", "commit": "a" * 7}, False),
            ({"status": "healthy", "version": "1.2.3", "commit": "b" * 40}, False),
            ({"status": "healthy", "version": "1.2.2", "commit": "a" * 40}, False),
            (["healthy"], False),
        ]:
            response = mock.MagicMock(status=200)
            response.__enter__.return_value = response
            response.read.return_value = json.dumps(payload).encode()
            with self.subTest(payload=payload), mock.patch.object(release.urllib.request, "build_opener") as opener, \
                    mock.patch.object(release.time, "monotonic", side_effect=[0, 0, 100]), \
                    mock.patch.object(release.time, "sleep"):
                opener.return_value.open.return_value = response
                if accepted:
                    release.wait_healthy(manifest)
                else:
                    with self.assertRaises(release.DeployError):
                        release.wait_healthy(manifest)
                self.assertEqual(opener.return_value.open.call_args.args[0], "http://127.0.0.1:5478/health")

    def test_environment_rejects_group_or_other_access(self):
        self.assertTrue(hasattr(release, "check_private_environment"), "private env validation is missing")
        for mode in (0o100644, 0o100640, 0o100604, 0o100660, 0o100601):
            with self.subTest(mode=oct(mode)), self.assertRaises(release.DeployError):
                release.check_private_environment(types.SimpleNamespace(st_mode=mode))
        for mode in (0o100600, 0o100400):
            release.check_private_environment(types.SimpleNamespace(st_mode=mode))

    def test_untrusted_owner_or_write_mode_is_rejected(self):
        for uid, mode in [(1000, 0o100555), (0, 0o100775), (0, 0o100557)]:
            with self.subTest(uid=uid, mode=oct(mode)), self.assertRaises(release.DeployError):
                release.check_root_owned(types.SimpleNamespace(st_uid=uid, st_mode=mode))
        release.check_root_owned(types.SimpleNamespace(st_uid=0, st_mode=0o100555))


class DeploymentTemplateTests(unittest.TestCase):
    def test_service_runs_unprivileged_with_drain_and_no_arbitrary_memory_cap(self):
        path = SCRIPT.with_name("papersage.service")
        self.assertTrue(path.exists(), "systemd unit has not been implemented")
        content = path.read_text()
        for setting in ("User=papersage", "Group=papersage", "EnvironmentFile=/etc/papersage/papersage.env",
                        "WorkingDirectory=/var/lib/papersage", "ExecStart=/opt/papersage/current/papersage",
                        "Restart=on-failure", "TimeoutStopSec=90s", "NoNewPrivileges=true",
                        "ProtectSystem=strict", "ReadWritePaths=/var/lib/papersage",
                        "CPUAccounting=true", "MemoryAccounting=true", "TasksAccounting=true"):
            self.assertIn(setting, content)
        self.assertNotIn("MemoryMax", content)

    def test_caddy_enables_https_host_and_unbuffered_streaming(self):
        path = SCRIPT.with_name("Caddyfile")
        self.assertTrue(path.exists(), "Caddy template has not been implemented")
        content = path.read_text()
        self.assertIn("api-papersage.overlink.top {", content)
        self.assertIn("reverse_proxy 127.0.0.1:5478", content)
        self.assertIn("flush_interval -1", content)
        self.assertIn("header_up X-Forwarded-For {http.request.header.CF-Connecting-IP}", content)
        self.assertIn("header_up X-Real-IP {http.request.header.CF-Connecting-IP}", content)
        self.assertNotIn("http://", content)

    def test_caddy_requires_the_ingress_guard(self):
        content = SCRIPT.with_name("caddy-cloudflare-guard.conf").read_text()
        self.assertIn("Requires=papersage-cloudflare-guard.service", content)
        self.assertIn("After=papersage-cloudflare-guard.service", content)
        self.assertIn("ExecStartPre=+/usr/sbin/nft list table inet papersage_cloudflare", content)

    def test_sudoers_allows_only_the_validating_helper_without_setenv(self):
        path = SCRIPT.with_name("papersage-deploy.sudoers")
        self.assertTrue(path.exists(), "sudoers template has not been implemented")
        content = path.read_text().strip()
        self.assertEqual(content, "papersage-deploy ALL=(root) NOPASSWD: NOSETENV: /usr/local/sbin/papersage-release")


if __name__ == "__main__":
    unittest.main()
