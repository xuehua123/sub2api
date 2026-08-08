#!/usr/bin/env python3
"""Fail-closed secret scanning for the current Git state and full history.

The scanner deliberately avoids a mutable action or an unverified executable:
it downloads one pinned Gitleaks release, verifies the release asset SHA-256,
and runs a detection canary before inspecting the repository. Findings are
reported as metadata only; secret values and matched source lines are never
printed by this wrapper.
"""

from __future__ import annotations

import argparse
import base64
import hashlib
import io
import json
import os
import platform
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
import zipfile
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path, PurePosixPath
from typing import Iterator, Sequence


GITLEAKS_VERSION = "8.29.1"
RELEASE_BASE_URL = (
    "https://github.com/gitleaks/gitleaks/releases/download/"
    f"v{GITLEAKS_VERSION}"
)
MAX_ARCHIVE_BYTES = 64 * 1024 * 1024


@dataclass(frozen=True)
class ReleaseAsset:
    filename: str
    sha256: str


RELEASE_ASSETS: dict[tuple[str, str], ReleaseAsset] = {
    ("darwin", "arm64"): ReleaseAsset(
        "gitleaks_8.29.1_darwin_arm64.tar.gz",
        "69836c841d7e648fb30ff4846f8c3587855c5754ed02b8510caaf6008f65d177",
    ),
    ("darwin", "x64"): ReleaseAsset(
        "gitleaks_8.29.1_darwin_x64.tar.gz",
        "2cd739c684bf3f543f4f37774075c276e40a72bb16c4c5bb9dfd27bf4a4465a7",
    ),
    ("linux", "arm64"): ReleaseAsset(
        "gitleaks_8.29.1_linux_arm64.tar.gz",
        "691f826ce7c1c564c9c02d0f9025e8e70803e3816707a4be6224408a06a81eaa",
    ),
    ("linux", "x64"): ReleaseAsset(
        "gitleaks_8.29.1_linux_x64.tar.gz",
        "e4eb209d04e20339d77122a3bdf9cd41351255cfb27ebcb75e85325e04f88924",
    ),
    ("windows", "x64"): ReleaseAsset(
        "gitleaks_8.29.1_windows_x64.zip",
        "e4b7d556f0cddbe23d10d8fac2ab0f29f68f019091c6599ffbeaa8a4fb71ac78",
    ),
}


class SecretScanError(RuntimeError):
    """An operational failure that must block the release."""


def _normalize_machine(machine: str) -> str:
    normalized = machine.strip().lower()
    if normalized in {"amd64", "x86_64", "x64"}:
        return "x64"
    if normalized in {"aarch64", "arm64"}:
        return "arm64"
    return normalized


def _command_for_binary(path: Path) -> list[str]:
    # Python scripts are accepted only as an explicit override. This makes the
    # wrapper testable on Windows without weakening the downloaded production
    # path, which is always a native executable from a verified archive.
    if path.suffix.lower() == ".py":
        return [sys.executable, str(path)]
    return [str(path)]


def _run(
    command: Sequence[str],
    *,
    cwd: Path | None = None,
    input_text: str | None = None,
    timeout: int = 300,
) -> subprocess.CompletedProcess[str]:
    try:
        return subprocess.run(
            list(command),
            cwd=cwd,
            input=input_text,
            text=True,
            capture_output=True,
            timeout=timeout,
            check=False,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise SecretScanError(f"failed to execute required command: {command[0]}") from exc


def _verify_version(command: Sequence[str]) -> None:
    result = _run([*command, "version"], timeout=30)
    if result.returncode != 0:
        raise SecretScanError("Gitleaks version check failed")
    output = f"{result.stdout}\n{result.stderr}"
    versions = re.findall(r"(?<![0-9])v?([0-9]+\.[0-9]+\.[0-9]+)(?![0-9])", output)
    if versions != [GITLEAKS_VERSION]:
        raise SecretScanError(
            f"Gitleaks must report exactly version {GITLEAKS_VERSION}"
        )


def _canary_values() -> tuple[str, str, str]:
    # Keep signatures split in source so the repository scan does not flag its
    # own canaries. Each value is synthetic and used only through stdin.
    github_seed = hashlib.sha256(b"sub2api-github-secret-scanner-canary").digest()
    github_suffix = "".join(
        char
        for char in base64.b64encode(github_seed).decode("ascii")
        if char.isalnum()
    )[:36]
    github = "gh" + "p_" + github_suffix
    aws_suffix = base64.b32encode(
        hashlib.sha256(b"sub2api-aws-secret-scanner-canary").digest()
    ).decode("ascii")[:16]
    aws = "AK" + "IA" + aws_suffix
    private_key = (
        "-----BEGIN OPENSSH PRI"
        + "VATE KEY-----\n"
        + ("b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQ" * 3)
        + "\n-----END OPENSSH PRI"
        + "VATE KEY-----\n"
    )
    if len(github.removeprefix("ghp_")) != 36:
        raise SecretScanError("internal GitHub canary is malformed")
    if len(aws.removeprefix("AKIA")) != 16:
        raise SecretScanError("internal AWS canary is malformed")
    return github, aws, private_key


def _verify_detection_canaries(command: Sequence[str]) -> None:
    args = [
        *command,
        "stdin",
        "--redact=100",
        "--no-banner",
        "--no-color",
        "--exit-code=1",
    ]
    for label, canary in zip(
        ("github-pat", "aws-access-key", "private-key"),
        _canary_values(),
        strict=True,
    ):
        result = _run(args, input_text=canary, timeout=30)
        if result.returncode != 1:
            raise SecretScanError(
                f"Gitleaks detection self-test failed for {label}; refusing to scan"
            )
        combined_output = f"{result.stdout}\n{result.stderr}"
        if canary in combined_output:
            raise SecretScanError(
                f"Gitleaks did not redact its {label} self-test output"
            )


def _download_archive(asset: ReleaseAsset) -> bytes:
    request = urllib.request.Request(
        f"{RELEASE_BASE_URL}/{asset.filename}",
        headers={"User-Agent": "sub2api-secret-scan/1"},
    )
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            length = response.headers.get("Content-Length")
            if length is not None and int(length) > MAX_ARCHIVE_BYTES:
                raise SecretScanError("Gitleaks release archive exceeds the size limit")
            chunks: list[bytes] = []
            total = 0
            while True:
                chunk = response.read(1024 * 1024)
                if not chunk:
                    break
                total += len(chunk)
                if total > MAX_ARCHIVE_BYTES:
                    raise SecretScanError("Gitleaks release archive exceeds the size limit")
                chunks.append(chunk)
    except SecretScanError:
        raise
    except Exception as exc:
        raise SecretScanError("failed to download pinned Gitleaks release") from exc

    archive = b"".join(chunks)
    if hashlib.sha256(archive).hexdigest() != asset.sha256:
        raise SecretScanError("downloaded Gitleaks archive failed SHA-256 verification")
    return archive


def _extract_binary(archive: bytes, asset: ReleaseAsset, destination: Path) -> Path:
    executable_name = "gitleaks.exe" if asset.filename.endswith(".zip") else "gitleaks"
    try:
        if asset.filename.endswith(".zip"):
            with zipfile.ZipFile(io.BytesIO(archive)) as bundle:
                candidates = [
                    info
                    for info in bundle.infolist()
                    if not info.is_dir()
                    and PurePosixPath(info.filename).name == executable_name
                ]
                if len(candidates) != 1:
                    raise SecretScanError(
                        "verified Gitleaks archive has an unexpected executable layout"
                    )
                if candidates[0].file_size <= 0 or candidates[0].file_size > MAX_ARCHIVE_BYTES:
                    raise SecretScanError("Gitleaks executable has an invalid size")
                binary = bundle.read(candidates[0])
        else:
            with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as bundle:
                candidates = [
                    member
                    for member in bundle.getmembers()
                    if member.isfile()
                    and PurePosixPath(member.name).name == executable_name
                ]
                if len(candidates) != 1:
                    raise SecretScanError(
                        "verified Gitleaks archive has an unexpected executable layout"
                    )
                if candidates[0].size <= 0 or candidates[0].size > MAX_ARCHIVE_BYTES:
                    raise SecretScanError("Gitleaks executable has an invalid size")
                stream = bundle.extractfile(candidates[0])
                if stream is None:
                    raise SecretScanError("failed to read Gitleaks executable from archive")
                binary = stream.read(MAX_ARCHIVE_BYTES + 1)
    except SecretScanError:
        raise
    except (tarfile.TarError, zipfile.BadZipFile, OSError) as exc:
        raise SecretScanError("verified Gitleaks archive could not be read") from exc

    if not binary or len(binary) > MAX_ARCHIVE_BYTES:
        raise SecretScanError("Gitleaks executable has an invalid size")
    path = destination / executable_name
    path.write_bytes(binary)
    if os.name != "nt":
        path.chmod(path.stat().st_mode | stat.S_IXUSR)
    return path


@contextmanager
def _gitleaks_command(override: str | None) -> Iterator[list[str]]:
    if override:
        path = Path(override).expanduser().resolve()
        if not path.is_file():
            raise SecretScanError("configured Gitleaks binary is not a regular file")
        command = _command_for_binary(path)
        _verify_version(command)
        _verify_detection_canaries(command)
        yield command
        return

    system = platform.system().strip().lower()
    machine = _normalize_machine(platform.machine())
    asset = RELEASE_ASSETS.get((system, machine))
    if asset is None:
        raise SecretScanError(
            "no verified Gitleaks release asset for this platform; provide "
            f"--gitleaks-binary with exact version {GITLEAKS_VERSION}"
        )

    with tempfile.TemporaryDirectory(prefix="sub2api-gitleaks-") as temp:
        root = Path(temp)
        archive = _download_archive(asset)
        binary = _extract_binary(archive, asset, root)
        command = _command_for_binary(binary)
        _verify_version(command)
        _verify_detection_canaries(command)
        yield command


def _git(repo_root: Path, *args: str, text: bool = True) -> subprocess.CompletedProcess:
    try:
        return subprocess.run(
            ["git", "-C", str(repo_root), *args],
            check=False,
            capture_output=True,
            text=text,
            timeout=120,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        raise SecretScanError("failed to inspect repository with Git") from exc


def _validate_repo_root(repo_root: Path) -> Path:
    root = repo_root.resolve()
    result = _git(root, "rev-parse", "--show-toplevel")
    if result.returncode != 0:
        raise SecretScanError("secret scan root is not a Git worktree")
    discovered = Path(result.stdout.strip()).resolve()
    if discovered != root:
        raise SecretScanError("--repo-root must be the Git worktree root")
    return root


def _require_full_history(repo_root: Path) -> None:
    result = _git(repo_root, "rev-parse", "--is-shallow-repository")
    if result.returncode != 0 or result.stdout.strip() != "false":
        raise SecretScanError(
            "full-history secret scan requires a non-shallow clone (fetch-depth: 0)"
        )


def _safe_repo_path(repo_root: Path, raw_path: str) -> tuple[Path, tuple[str, ...]]:
    pure = PurePosixPath(raw_path)
    if pure.is_absolute() or not pure.parts or ".." in pure.parts:
        raise SecretScanError("Git returned an unsafe repository path")
    source = repo_root.joinpath(*pure.parts)
    return source, tuple(pure.parts)


def _copy_worktree_files(repo_root: Path, destination: Path) -> None:
    result = _git(
        repo_root,
        "ls-files",
        "-z",
        "--cached",
        "--others",
        "--exclude-standard",
        text=False,
    )
    if result.returncode != 0:
        raise SecretScanError("failed to enumerate current worktree files")
    try:
        raw_paths = [
            value.decode("utf-8", errors="strict")
            for value in result.stdout.split(b"\0")
            if value
        ]
    except UnicodeDecodeError as exc:
        raise SecretScanError("repository contains a non-UTF-8 Git path") from exc

    for raw_path in raw_paths:
        source, parts = _safe_repo_path(repo_root, raw_path)
        try:
            if source.is_symlink():
                content = os.readlink(source).encode("utf-8")
            elif source.is_file():
                content = source.read_bytes()
            elif not source.exists():
                # A tracked deletion has no current worktree content. The index
                # snapshot below still covers a staged version, if one exists.
                continue
            elif source.is_dir():
                # Gitlinks/submodules are scanned by their own repository gate.
                continue
            else:
                raise SecretScanError("repository path is not a regular file")
        except OSError as exc:
            raise SecretScanError(f"failed to read repository path: {raw_path}") from exc
        target = destination.joinpath(*parts)
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content)


def _copy_index(repo_root: Path, destination: Path) -> None:
    destination.mkdir(parents=True, exist_ok=True)
    prefix = str(destination.resolve()) + os.sep
    result = _git(repo_root, "checkout-index", "--all", "--force", f"--prefix={prefix}")
    if result.returncode != 0:
        raise SecretScanError("failed to materialize the staged Git index")


def _common_gitleaks_args(repo_root: Path, report_path: Path) -> list[str]:
    args = [
        "--redact=100",
        "--no-banner",
        "--no-color",
        "--exit-code=1",
        "--timeout=300",
        "--max-archive-depth=1",
        "--max-decode-depth=3",
        "--report-format=json",
        f"--report-path={report_path}",
    ]
    config = repo_root / ".gitleaks.toml"
    if config.exists():
        # A custom config can silently disable built-in rules. Keep the gate on
        # the config embedded in the checksum-pinned binary until a future
        # config is explicitly hash-pinned and covered by canaries here.
        raise SecretScanError("custom .gitleaks.toml is not allowed by this gate")
    ignore = repo_root / ".gitleaksignore"
    if ignore.exists():
        if not ignore.is_file() or ignore.is_symlink():
            raise SecretScanError(".gitleaksignore must be a regular non-symlink file")
        _validate_ignore_file(ignore)
        args.append(f"--gitleaks-ignore-path={ignore}")
    return args


def _validate_ignore_file(path: Path) -> None:
    try:
        raw_lines = path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeDecodeError) as exc:
        raise SecretScanError("failed to read .gitleaksignore") from exc
    fingerprints = [
        line.strip()
        for line in raw_lines
        if line.strip() and not line.lstrip().startswith("#")
    ]
    exact_fingerprint = re.compile(
        r"(?:"
        r"(?:index|worktree)/[^:\r\n]+:[a-z0-9-]+:[1-9][0-9]*"
        r"|[0-9a-f]{40}:[^:\r\n]+:[a-z0-9-]+:[1-9][0-9]*"
        r")"
    )
    if len(fingerprints) != len(set(fingerprints)):
        raise SecretScanError(".gitleaksignore contains duplicate fingerprints")
    if any(exact_fingerprint.fullmatch(value) is None for value in fingerprints):
        raise SecretScanError(
            ".gitleaksignore may contain only exact current/history fingerprints"
        )


def _safe_finding_metadata(report_path: Path) -> list[str]:
    try:
        payload = json.loads(report_path.read_text(encoding="utf-8"))
    except (OSError, UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise SecretScanError("Gitleaks findings report is missing or invalid") from exc
    if not isinstance(payload, list) or not payload:
        raise SecretScanError("Gitleaks returned findings without report metadata")

    metadata: list[str] = []
    for finding in payload:
        if not isinstance(finding, dict):
            raise SecretScanError("Gitleaks findings report has an invalid entry")
        file_name = finding.get("File", "<unknown>")
        line = finding.get("StartLine", 0)
        rule = finding.get("RuleID", "<unknown>")
        commit = finding.get("Commit", "")
        fingerprint = finding.get("Fingerprint", "")
        if not isinstance(file_name, str) or not isinstance(rule, str):
            raise SecretScanError("Gitleaks findings metadata has invalid types")
        try:
            line_number = int(line)
        except (TypeError, ValueError) as exc:
            raise SecretScanError("Gitleaks finding line number is invalid") from exc
        commit_text = str(commit)
        if commit_text and not re.fullmatch(r"[0-9a-fA-F]{7,64}", commit_text):
            commit_text = "<invalid>"
        fingerprint_text = str(fingerprint)
        if (
            len(fingerprint_text) > 2048
            or "\n" in fingerprint_text
            or "\r" in fingerprint_text
        ):
            fingerprint_text = "<invalid>"
        metadata.append(
            "file="
            + json.dumps(file_name, ensure_ascii=True)
            + f" line={line_number} rule="
            + json.dumps(rule, ensure_ascii=True)
            + (f" commit={commit_text[:12]}" if commit_text else "")
            + (
                " fingerprint=" + json.dumps(fingerprint_text, ensure_ascii=True)
                if fingerprint_text
                else ""
            )
        )
    return metadata


def _run_repository_scan(
    command: Sequence[str],
    repo_root: Path,
    *,
    label: str,
    subcommand: Sequence[str],
    report_path: Path,
    scan_cwd: Path,
) -> bool:
    result = _run(
        [
            *command,
            *subcommand,
            *_common_gitleaks_args(repo_root, report_path),
        ],
        cwd=scan_cwd,
        timeout=360,
    )
    if result.returncode == 0:
        return True
    if result.returncode != 1:
        raise SecretScanError(
            f"Gitleaks {label} scan failed operationally (exit {result.returncode})"
        )

    findings = _safe_finding_metadata(report_path)
    sys.stderr.write(f"Secret scan found potential secrets in {label}:\n")
    for finding in findings:
        sys.stderr.write(f"- {finding}\n")
    return False


def scan_repository(command: Sequence[str], repo_root: Path, scope: str) -> bool:
    passed = True
    with tempfile.TemporaryDirectory(prefix="sub2api-secret-snapshot-") as temp:
        temp_root = Path(temp)
        if scope in {"all", "worktree"}:
            snapshot = temp_root / "snapshot"
            _copy_index(repo_root, snapshot / "index")
            _copy_worktree_files(repo_root, snapshot / "worktree")
            passed = _run_repository_scan(
                command,
                repo_root,
                label="current index/worktree",
                subcommand=("dir", "."),
                report_path=temp_root / "worktree-findings.json",
                scan_cwd=snapshot,
            ) and passed

        if scope in {"all", "history"}:
            _require_full_history(repo_root)
            passed = _run_repository_scan(
                command,
                repo_root,
                label="full Git history",
                subcommand=(
                    "git",
                    str(repo_root),
                    # Scan every commit that can reach the exact release HEAD.
                    # Unrelated local backup/upstream refs are not part of this
                    # deployable history and would make local and CI results
                    # depend on which extra remotes happen to be fetched.
                    "--log-opts=--full-history HEAD",
                ),
                report_path=temp_root / "history-findings.json",
                scan_cwd=repo_root,
            ) and passed
    return passed


def parse_args(argv: Sequence[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--repo-root",
        default=str(Path(__file__).resolve().parents[1]),
        help="Git worktree root (defaults to this script's repository)",
    )
    parser.add_argument(
        "--gitleaks-binary",
        "--gitleaks",
        dest="gitleaks_binary",
        default=os.environ.get("GITLEAKS_BIN"),
        help=f"verified override binary; it must report version {GITLEAKS_VERSION}",
    )
    parser.add_argument(
        "--scope",
        choices=("all", "worktree", "history"),
        default="all",
        help="scan current Git state, history, or both (default: both)",
    )
    return parser.parse_args(argv)


def main(argv: Sequence[str]) -> int:
    args = parse_args(argv)
    try:
        repo_root = _validate_repo_root(Path(args.repo_root))
        with _gitleaks_command(args.gitleaks_binary) as command:
            if not scan_repository(command, repo_root, args.scope):
                return 1
    except SecretScanError as exc:
        sys.stderr.write(f"Secret scan FAILED: {exc}\n")
        return 2
    print("Secret scan OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
