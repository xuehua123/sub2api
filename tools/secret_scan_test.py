from __future__ import annotations

import os
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[1]
SECRET_SCAN = REPO_ROOT / "tools" / "secret_scan.py"


def run(
    command: list[str], *, cwd: Path, check: bool = True
) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        command,
        cwd=cwd,
        check=check,
        text=True,
        capture_output=True,
    )


class SecretScanCLITest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory(prefix="secret-scan-test-")
        self.addCleanup(self.temp_dir.cleanup)
        self.root = Path(self.temp_dir.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        run(["git", "init", "--quiet"], cwd=self.repo)
        run(["git", "config", "user.email", "secret-scan@example.invalid"], cwd=self.repo)
        run(["git", "config", "user.name", "Secret Scan Test"], cwd=self.repo)
        self.gitleaks = self.root / "fake_gitleaks.py"
        self.gitleaks.write_text(
            textwrap.dedent(
                r'''
                import json
                import os
                import re
                import subprocess
                import sys
                from pathlib import Path

                args = sys.argv[1:]
                version = os.environ.get("FAKE_GITLEAKS_VERSION", "8.29.1")
                if args == ["version"]:
                    print(version)
                    raise SystemExit(0)

                github = re.compile(r"gh" + r"p_[0-9A-Za-z]{36}")
                aws = re.compile(r"AK" + r"IA[A-Z2-7]{16}")
                private_key = re.compile(r"-----BEGIN OPENSSH PRI" + r"VATE KEY-----")

                if "stdin" in args:
                    content = sys.stdin.read()
                    source = "stdin"
                elif "dir" in args:
                    source_path = Path(args[args.index("dir") + 1])
                    source = str(source_path)
                    content = "\n".join(
                        path.read_text(encoding="utf-8", errors="ignore")
                        for path in source_path.rglob("*")
                        if path.is_file()
                    )
                elif "git" in args:
                    source_path = Path(args[args.index("git") + 1])
                    source = str(source_path)
                    result = subprocess.run(
                        ["git", "log", "-p", "--all", "--full-history", "--no-ext-diff", "--text"],
                        cwd=source_path,
                        check=True,
                        text=True,
                        capture_output=True,
                    )
                    content = result.stdout
                else:
                    raise SystemExit(2)

                disabled = os.environ.get("FAKE_GITLEAKS_DISABLE_DETECTION") == "1"
                leaked = not disabled and (
                    github.search(content) or aws.search(content) or private_key.search(content)
                )
                if not leaked:
                    print("INF no leaks found", file=sys.stderr)
                    raise SystemExit(0)

                report_arg = next((arg for arg in args if arg.startswith("--report-path=")), None)
                if report_arg:
                    report = Path(report_arg.split("=", 1)[1])
                    report.write_text(
                        json.dumps(
                            [
                                {
                                    "File": source,
                                    "StartLine": 1,
                                    "RuleID": "synthetic-secret",
                                    "Commit": "a" * 40 if "git" in args else "",
                                }
                            ]
                        ),
                        encoding="utf-8",
                    )
                print("WRN leaks found: 1", file=sys.stderr)
                raise SystemExit(1)
                '''
            ),
            encoding="utf-8",
        )

    def commit_file(self, relative_path: str, content: str) -> None:
        path = self.repo / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        run(["git", "add", "--", relative_path], cwd=self.repo)
        run(["git", "commit", "--quiet", "-m", f"add {relative_path}"], cwd=self.repo)

    @staticmethod
    def fake_github_pat() -> str:
        return "gh" + "p_" + "aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"

    def scan(
        self, *extra: str, env: dict[str, str] | None = None, repo: Path | None = None
    ) -> subprocess.CompletedProcess[str]:
        command = [
            sys.executable,
            str(SECRET_SCAN),
            "--repo-root",
            str(repo or self.repo),
            "--gitleaks-binary",
            str(self.gitleaks),
            *extra,
        ]
        merged_env = os.environ.copy()
        if env:
            merged_env.update(env)
        return subprocess.run(command, text=True, capture_output=True, env=merged_env)

    def test_placeholder_passes_current_tree_and_history_scan(self) -> None:
        self.commit_file("settings.example", "token = ghp_your-token-here\n")

        result = self.scan()

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("Secret scan OK", result.stdout)

    def test_untracked_secret_is_blocked(self) -> None:
        self.commit_file("README.md", "safe\n")
        (self.repo / "untracked.env").write_text(
            f"TOKEN={self.fake_github_pat()}\n", encoding="utf-8"
        )

        result = self.scan("--scope", "worktree")

        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertNotIn(self.fake_github_pat(), result.stderr)

    def test_staged_secret_hidden_by_unstaged_edit_is_blocked(self) -> None:
        self.commit_file("config.env", "safe=true\n")
        path = self.repo / "config.env"
        path.write_text(f"TOKEN={self.fake_github_pat()}\n", encoding="utf-8")
        run(["git", "add", "--", "config.env"], cwd=self.repo)
        path.write_text("safe=true\n", encoding="utf-8")

        result = self.scan("--scope", "worktree")

        self.assertEqual(result.returncode, 1, result.stderr)

    def test_removed_historical_secret_is_blocked(self) -> None:
        self.commit_file("old.env", f"TOKEN={self.fake_github_pat()}\n")
        self.commit_file("old.env", "removed=true\n")

        result = self.scan("--scope", "history")

        self.assertEqual(result.returncode, 1, result.stderr)

    def test_gitignored_local_secret_is_not_in_worktree_snapshot(self) -> None:
        self.commit_file(".gitignore", ".env.local\n")
        (self.repo / ".env.local").write_text(
            f"TOKEN={self.fake_github_pat()}\n", encoding="utf-8"
        )

        result = self.scan("--scope", "worktree")

        self.assertEqual(result.returncode, 0, result.stderr)

    def test_wrong_scanner_version_fails_closed(self) -> None:
        self.commit_file("README.md", "safe\n")

        result = self.scan(env={"FAKE_GITLEAKS_VERSION": "8.30.1"})

        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("must report exactly version 8.29.1", result.stderr)

    def test_detection_canary_failure_fails_closed(self) -> None:
        self.commit_file("README.md", "safe\n")

        result = self.scan(env={"FAKE_GITLEAKS_DISABLE_DETECTION": "1"})

        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("detection self-test failed", result.stderr)

    def test_shallow_history_scan_fails_closed(self) -> None:
        self.commit_file("README.md", "safe\n")
        head = run(["git", "rev-parse", "HEAD"], cwd=self.repo).stdout.strip()
        (self.repo / ".git" / "shallow").write_text(f"{head}\n", encoding="ascii")

        result = self.scan("--scope", "history")

        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("non-shallow clone", result.stderr)

    def test_broad_ignore_expression_fails_closed(self) -> None:
        self.commit_file("README.md", "safe\n")
        (self.repo / ".gitleaksignore").write_text(".*\n", encoding="utf-8")

        result = self.scan("--scope", "worktree")

        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("only exact current/history fingerprints", result.stderr)

    def test_custom_config_fails_closed(self) -> None:
        self.commit_file("README.md", "safe\n")
        (self.repo / ".gitleaks.toml").write_text(
            "[extend]\nuseDefault = false\n", encoding="utf-8"
        )

        result = self.scan("--scope", "worktree")

        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("custom .gitleaks.toml is not allowed", result.stderr)


if __name__ == "__main__":
    unittest.main()
