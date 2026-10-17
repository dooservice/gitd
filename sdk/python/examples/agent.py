"""An agent's round trip with gitd:

  1. gitd prepares a workspace of the repository, in milliseconds after the first time;
  2. the agent works in it with plain git, and commits;
  3. gitd publishes the commit as a new branch, with the token it keeps.

The agent never holds a credential.

  python examples/agent.py https://github.com/your-org/your-repo /var/lib/gitd/workspaces/agent-1
"""

from __future__ import annotations

import subprocess
import sys
import time
from pathlib import Path

from gitd import Gitd, GitdError, Progress, Workspace

AUTHOR = ["-c", "user.name=agent", "-c", "user.email=agent@example.com"]
BRANCH_PREFIX = "agents/example-"
NOTE_FILE = "AGENT.md"


class Agent:
    """Works on one repository in its own workspace, and publishes what it commits."""

    def __init__(self, gitd: Gitd, url: str, path: str) -> None:
        self.gitd = gitd
        self.url = url
        self.path = path

    def run(self) -> None:
        workspace = self.prepare()
        try:
            self.commit_a_change(workspace)
            self.publish(workspace)
        finally:
            self.gitd.remove(workspace.path)

    def prepare(self) -> Workspace:
        workspace = self.gitd.prepare(self.url, self.path, on_progress=self.show_progress)
        print(file=sys.stderr)
        print(f"Prepared {workspace.path} at {workspace.commit[:12]}, by {workspace.strategy.value}.")
        return workspace

    def commit_a_change(self, workspace: Workspace) -> None:
        Path(workspace.path, NOTE_FILE).write_text("Written by an agent.\n")
        self.git(workspace, "add", NOTE_FILE)
        self.git(workspace, "commit", "--quiet", "--message", "An agent was here")
        print(f"Committed {NOTE_FILE}.")

    def publish(self, workspace: Workspace) -> None:
        published = self.gitd.publish(workspace.path, f"{BRANCH_PREFIX}{int(time.time())}")
        repository = published.repositories[0]
        print(f"Published {published.branch} to {repository.url}: {', '.join(repository.subjects)}.")

    def git(self, workspace: Workspace, *arguments: str) -> None:
        subprocess.run(["git", *AUTHOR, *arguments], cwd=workspace.path, check=True)

    def show_progress(self, progress: Progress) -> None:
        print(f"\r{progress.phase} {progress.percent}%", end="", file=sys.stderr)


def main(arguments: list[str]) -> int:
    if len(arguments) != 2:
        print("usage: python examples/agent.py <repository url> <workspace path>", file=sys.stderr)
        return 2
    url, path = arguments
    with Gitd() as gitd:
        try:
            Agent(gitd, url, path).run()
        except GitdError as error:
            print(f"gitd refused: {error.reason}: {error.said}", file=sys.stderr)
            return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
