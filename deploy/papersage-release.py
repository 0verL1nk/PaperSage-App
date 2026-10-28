#!/usr/bin/python3 -I
import contextlib
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request
import uuid


APP_ROOT = Path("/opt/papersage")
RELEASES = APP_ROOT / "releases"
CURRENT = APP_ROOT / "current"
UPLOADS = Path("/var/lib/papersage-deploy/uploads")
LOCK_FILE = UPLOADS.parent / "release.lock"
ENV_FILE = Path("/etc/papersage/papersage.env")
WORK_DIR = Path("/var/lib/papersage")
HEALTH_URL = "http://127.0.0.1:5478/health"
MAX_ARCHIVE_BYTES = 512 * 1024 * 1024
MAX_RELEASE_BYTES = 512 * 1024 * 1024
COMMAND_TIMEOUT_SECONDS = 120
MIGRATION_BUDGET_SECONDS = 300
MIGRATION_STOP_SECONDS = 30
VERSION = re.compile(r"v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)", re.ASCII)
COMMIT = re.compile(r"[0-9a-f]{40}", re.ASCII)
CHECKSUM = re.compile(r"[0-9a-f]{64}", re.ASCII)


class DeployError(Exception):
    pass


def validate(value, pattern):
    if not isinstance(value, str) or not pattern.fullmatch(value):
        raise DeployError("invalid release version, commit or checksum")


def check_root_owned(info):
    if info.st_uid != 0 or info.st_mode & 0o022:
        raise DeployError("deployment paths must be root-owned and not group/world writable")


def check_private_environment(info):
    if info.st_mode & 0o077:
        raise DeployError("environment file must allow access only to root (mode 0600 or 0400)")


def trusted(path, directory=False):
    info = path.lstat()
    check_root_owned(info)
    expected = stat.S_ISDIR if directory else stat.S_ISREG
    if not expected(info.st_mode) or info.st_mode & 0o6000:
        raise DeployError("deployment path has an unsafe file type or permissions")
    return info


@contextlib.contextmanager
def release_lock():
    import fcntl

    trusted(LOCK_FILE.parent, directory=True)
    descriptor = os.open(LOCK_FILE, os.O_RDWR | os.O_CREAT | getattr(os, "O_NOFOLLOW", 0), 0o600)
    with os.fdopen(descriptor, "r+b") as lock:
        info = os.fstat(lock.fileno())
        check_root_owned(info)
        if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1:
            raise DeployError("unsafe release lock")
        fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
        try:
            yield
        finally:
            fcntl.flock(lock.fileno(), fcntl.LOCK_UN)


def load_release(version):
    validate(version, VERSION)
    target = RELEASES / version
    trusted(target, directory=True)
    if {path.name for path in target.iterdir()} != {"papersage", "migrate", "manifest.json"}:
        raise DeployError("installed release has unexpected files")
    for name in ("papersage", "migrate", "manifest.json"):
        trusted(target / name)
    if (target / "manifest.json").stat().st_size > 4096:
        raise DeployError("invalid release manifest")
    manifest = json.loads((target / "manifest.json").read_text(encoding="utf-8"))
    if not isinstance(manifest, dict) or set(manifest) != {"version", "commit", "archive_sha256"}:
        raise DeployError("invalid release manifest")
    if manifest["version"] != version:
        raise DeployError("release manifest version mismatch")
    validate(manifest["commit"], COMMIT)
    validate(manifest["archive_sha256"], CHECKSUM)
    return manifest


def current_release():
    if not os.path.lexists(CURRENT):
        return None
    info = CURRENT.lstat()
    if not stat.S_ISLNK(info.st_mode) or info.st_uid != 0:
        raise DeployError("current must be a root-owned release symlink")
    target = Path(os.readlink(CURRENT))
    if target.parent != RELEASES:
        raise DeployError("current points outside the release directory")
    return load_release(target.name)


def install_release(version, commit, checksum):
    if os.path.lexists(RELEASES / version):
        manifest = load_release(version)
        if manifest["commit"] != commit or manifest["archive_sha256"] != checksum:
            raise DeployError("version already exists with a different commit or checksum")
        return manifest
    source = UPLOADS / f"{version}-{commit}.tar.gz"
    with tempfile.TemporaryDirectory(prefix=".release-", dir=RELEASES) as directory:
        staging = Path(directory)
        archive_path = staging / "upload.tar.gz"
        info = source.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1:
            raise DeployError("upload must be a regular, non-linked file")
        descriptor = os.open(source, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0))
        digest = hashlib.sha256()
        size = 0
        # Snapshot the writable upload so checksum validation and extraction see identical bytes.
        with os.fdopen(descriptor, "rb") as upload, archive_path.open("xb") as snapshot:
            info = os.fstat(upload.fileno())
            if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1:
                raise DeployError("upload must be a regular, non-linked file")
            while chunk := upload.read(1024 * 1024):
                size += len(chunk)
                if size > MAX_ARCHIVE_BYTES:
                    raise DeployError("release archive is too large")
                digest.update(chunk)
                snapshot.write(chunk)
        if digest.hexdigest() != checksum:
            raise DeployError("archive checksum mismatch")
        tar_path = staging / "payload.tar"
        expanded = 0
        # Bound decompression, including padding and metadata, before tarfile processes any headers.
        with gzip.open(archive_path, "rb") as compressed, tar_path.open("xb") as raw_tar:
            while chunk := compressed.read(1024 * 1024):
                expanded += len(chunk)
                if expanded > MAX_RELEASE_BYTES + 1024 * 1024:
                    raise DeployError("unpacked archive is too large")
                raw_tar.write(chunk)
        unpacked = staging / "release"
        unpacked.mkdir(mode=0o755)
        seen = set()
        unpacked_size = 0
        with tarfile.open(tar_path, "r:", ignore_zeros=True) as archive:
            for member in archive:
                if (member.name not in {"papersage", "migrate"} or member.name in seen
                        or member.type not in (tarfile.REGTYPE, tarfile.AREGTYPE)
                        or member.sparse is not None or member.size <= 0):
                    raise DeployError("archive must contain only the two regular binaries papersage and migrate")
                unpacked_size += member.size
                if unpacked_size > MAX_RELEASE_BYTES:
                    raise DeployError("unpacked release is too large")
                seen.add(member.name)
                with archive.extractfile(member) as binary, (unpacked / member.name).open("xb") as output:
                    shutil.copyfileobj(binary, output, length=1024 * 1024)
                (unpacked / member.name).chmod(0o555)
        if seen != {"papersage", "migrate"}:
            raise DeployError("archive is missing a required binary")
        manifest = {"version": version, "commit": commit, "archive_sha256": checksum}
        (unpacked / "manifest.json").write_text(json.dumps(manifest) + "\n", encoding="utf-8")
        (unpacked / "manifest.json").chmod(0o444)
        unpacked.chmod(0o755)
        os.rename(unpacked, RELEASES / version)
    return manifest


def run_command(command, operation, timeout=COMMAND_TIMEOUT_SECONDS):
    try:
        subprocess.run(command, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
                       env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C.UTF-8"}, cwd="/", timeout=timeout)
    except (OSError, subprocess.SubprocessError):
        raise DeployError(f"{operation} failed; inspect the system journal") from None


def service(action):
    run_command(["/usr/bin/systemctl", action, "papersage.service"], f"service {action}")


def migrate(manifest):
    run_command([
        "/usr/bin/systemd-run", "--quiet", "--wait", "--collect", "--service-type=exec",
        f"--unit=papersage-migrate-{uuid.uuid4().hex}", "--uid=papersage", "--gid=papersage",
        f"--working-directory={WORK_DIR}", f"--property=EnvironmentFile={ENV_FILE}",
        # The migration unit must also expire if the SSH client or helper disappears.
        f"--property=RuntimeMaxSec={MIGRATION_BUDGET_SECONDS}s",
        f"--property=TimeoutStopSec={MIGRATION_STOP_SECONDS}s",
        "--property=NoNewPrivileges=yes", "--property=ProtectSystem=strict",
        "--property=ProtectHome=yes", "--property=PrivateTmp=yes",
        f"--property=ReadWritePaths={WORK_DIR}", "--property=UMask=0077",
        "--property=StandardOutput=journal", "--property=StandardError=journal",
        "--", str(RELEASES / manifest["version"] / "migrate"),
    ], "migration", timeout=MIGRATION_BUDGET_SECONDS + MIGRATION_STOP_SECONDS + 30)


def switch_current(manifest):
    temporary = APP_ROOT / f".current-{uuid.uuid4().hex}"
    try:
        os.symlink(RELEASES / manifest["version"], temporary, target_is_directory=True)
        os.replace(temporary, CURRENT)
    finally:
        temporary.unlink(missing_ok=True)


def wait_healthy(manifest):
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    deadline = time.monotonic() + 45
    while time.monotonic() < deadline:
        try:
            with opener.open(HEALTH_URL, timeout=2) as response:
                body = response.read(65537)
                payload = json.loads(body) if len(body) <= 65536 else None
                if (response.status == 200 and isinstance(payload, dict)
                        and payload.get("status") == "healthy"
                        and payload.get("version") == manifest["version"][1:]
                        and payload.get("commit") == manifest["commit"]):
                    return
        except (OSError, ValueError, urllib.error.URLError):
            pass
        time.sleep(1)
    raise DeployError("release health check failed")


def transition(target, previous, with_migration):
    try:
        service("stop")
        if with_migration:
            migrate(target)
        switch_current(target)
        service("start")
        wait_healthy(target)
    except (Exception, KeyboardInterrupt):
        try:
            service("stop")
            if previous is None:
                CURRENT.unlink(missing_ok=True)
            else:
                switch_current(previous)
                # Failed candidates can exhaust systemd's start limit before recovery starts the old binary.
                service("reset-failed")
                service("start")
                wait_healthy(previous)
        except (Exception, KeyboardInterrupt):
            raise DeployError("release failed; recovery failed; inspect the system journal") from None
        if previous is None:
            raise DeployError("release failed; no previous release; service left stopped") from None
        raise DeployError("release failed; previous release restored; schema was not rolled back") from None


def main(args=None):
    args = sys.argv[1:] if args is None else args
    try:
        if len(args) == 4 and args[0] == "activate":
            _, version, commit, checksum = args
            validate(version, VERSION)
            validate(commit, COMMIT)
            validate(checksum, CHECKSUM)
        elif len(args) == 2 and args[0] == "rollback":
            _, version = args
            validate(version, VERSION)
        else:
            raise DeployError("usage: papersage-release activate <vX.Y.Z> <40hexCommit> <64hexArchiveChecksum> | rollback <vX.Y.Z>")
        if os.geteuid() != 0:
            raise DeployError("release helper must run as root through the restricted sudo command")
        with release_lock():
            trusted(APP_ROOT, directory=True)
            trusted(RELEASES, directory=True)
            trusted(ENV_FILE.parent, directory=True)
            check_private_environment(trusted(ENV_FILE))
            previous = current_release()
            if args[0] == "activate":
                target = install_release(version, commit, checksum)
                if previous == target:
                    try:
                        wait_healthy(target)
                    except DeployError:
                        pass
                    else:
                        print(f"already active: {version} {commit}")
                        return 0
                transition(target, previous, with_migration=True)
            else:
                target = load_release(version)
                transition(target, previous, with_migration=False)
        print(f"active: {target['version']} {target['commit']}")
        return 0
    except DeployError as error:
        print(f"release error: {error}", file=sys.stderr)
    except (Exception, KeyboardInterrupt):
        print("release error: verify archive, paths, permissions and system journal", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
