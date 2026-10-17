"""The platform's side of the OpenHands example: gitd makes the workspace, OpenHands works in
it inside a sandbox that holds no credential, and gitd publishes what it committed as a pull
request, under the publishing settings, with where the work came from.

    GITD_SOCKET=... REPOSITORY=https://github.com/acme/web ROOT=/var/lib/gitd/workspaces \\
        python openhands_task.py

RUN names the branch and the note (default: the time); REVIEWER is asked to review;
LLM_MODEL and LLM_API_KEY, when set, have a real model do TASK instead of the scripted one.
"""

from __future__ import annotations

import os
import subprocess
import sys
import time
from dataclasses import dataclass
from pathlib import Path

from gitd import Gitd, Provenance, PullRequestOptions

OPENHANDS_VERSION = "1.55.1"
MODEL_VARIABLES = ("LLM_MODEL", "LLM_API_KEY", "LLM_BASE_URL", "TASK")


@dataclass(frozen=True)
class Sandbox:
    """The agent's container: the workspace bound at /workspace, run as the workspace's
    owner, with no gitd socket, no credential, and no network unless a real model needs it.
    The pool the workspace borrows its history from is bound read-only where the workspace
    names it, so git finds it there."""

    image: str
    environment: dict[str, str]

    @staticmethod
    def borrowed(path: str) -> list[str]:
        alternates = Path(path, ".git", "objects", "info", "alternates")
        return [line for line in alternates.read_text().splitlines() if line] if alternates.exists() else []

    def work(self, path: str, run: str) -> None:
        with_model = bool(self.environment.get("LLM_API_KEY"))
        command = ["docker", "run", "--rm", "--user", f"{os.getuid()}:{os.getgid()}"]
        command += ["--env", "HOME=/tmp", "--env", f"RUN={run}", "--volume", f"{path}:/workspace"]
        for objects in self.borrowed(path):
            command += ["--volume", f"{objects}:{objects}:ro"]
        command += [] if with_model else ["--network", "none"]
        for name in MODEL_VARIABLES:
            if self.environment.get(name):
                command += ["--env", name]
        subprocess.run(command + [self.image], check=True, env=self.environment)


@dataclass(frozen=True)
class Platform:
    gitd: Gitd
    repository: str
    root: str
    run: str
    reviewer: str
    sandbox: Sandbox

    @classmethod
    def from_environment(cls, gitd: Gitd) -> Platform:
        environment = dict(os.environ)
        return cls(
            gitd=gitd,
            repository=environment["REPOSITORY"],
            root=environment["ROOT"],
            run=environment.get("RUN", str(int(time.time()))),
            reviewer=environment.get("REVIEWER", ""),
            sandbox=Sandbox(
                image=environment.get("SANDBOX_IMAGE", "gitd-openhands-sandbox"), environment=environment
            ),
        )

    def step(self, said: str) -> None:
        print(f"\n== {said}", flush=True)

    def work(self) -> None:
        path = str(Path(self.root, f"openhands-{self.run}"))
        self.step("gitd makes the workspace")
        workspace = self.gitd.prepare(self.repository, path)
        print(f"{path} at {workspace.commit[:12]} on {workspace.branch}, made by {workspace.strategy.value}")

        self.step("OpenHands works in it, in a sandbox with no credential")
        self.sandbox.work(path, self.run)

        self.step("what the agent changed, as gitd reads it")
        changes = self.gitd.changes(path)
        for changed in changes.files:
            print(f"{changed.kind.value:9} {changed.path} +{changed.added_lines} -{changed.removed_lines}")

        self.step("no token in the workspace")
        config = Path(path, ".git", "config").read_text()
        if "ghs_" in config or "x-access-token" in config:
            sys.exit("a token is in the workspace's .git/config")
        print("none in .git/config")

        self.step("gitd publishes it as a pull request")
        model = self.sandbox.environment.get("LLM_MODEL") or "scripted"
        published = self.gitd.publish(
            path,
            f"agents/openhands-{self.run}",
            requested_by=self.reviewer,
            pull_request=PullRequestOptions(
                open=True,
                title=f"OpenHands: note for run {self.run}",
                body="An OpenHands agent worked in a workspace gitd made; its sandbox held no token.",
            ),
            provenance=Provenance(
                agent=f"OpenHands {OPENHANDS_VERSION}", model=model, session=f"openhands-{self.run}"
            ),
        )
        if published.pull_request is None:
            sys.exit(f"no pull request was opened: {published.pull_request_refused}")
        print(published.pull_request.url)
        self.gitd.remove(path)


if __name__ == "__main__":
    with Gitd() as connected:
        Platform.from_environment(connected).work()
