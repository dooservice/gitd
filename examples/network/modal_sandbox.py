"""A Modal sandbox clones from gitd's endpoint, commits and pushes, and gitd publishes it.

Needs `pip install gitd modal`, Modal's token (`modal token new`), and a gitd Modal can
reach over the internet: GITD_URL (https://…), GITD_KEY_FILE (the tenant's key), REPOSITORY
and BRANCH. Not run in CI.
"""

from __future__ import annotations

import os
import shlex

import modal
from gitd import Gitd


class ModalRun:
    """One agent run in a Modal sandbox, on a repository gitd keeps the credential of."""

    def __init__(self, repository: str, branch: str) -> None:
        self.repository = repository
        self.branch = branch

    def work(self) -> None:
        app = modal.App.lookup("gitd-example", create_if_missing=True)
        image = modal.Image.debian_slim().apt_install("git")
        with Gitd() as gitd:
            issued = gitd.endpoint.issue(self.repository)
            sandbox = modal.Sandbox.create(app=app, image=image)
            try:
                script = " && ".join(
                    [
                        f"git clone -q {shlex.quote(issued.authenticated_url())} /work",
                        "cd /work",
                        "git -c user.name=agent -c user.email=agent@example.com"
                        " commit -q --allow-empty -m 'from modal'",
                        f"git push -q origin HEAD:refs/heads/{shlex.quote(self.branch)}",
                    ]
                )
                process = sandbox.exec("sh", "-c", script)
                process.wait()
                print(process.stdout.read())
            finally:
                sandbox.terminate()
            published = gitd.publish("", self.branch, sandbox=issued.info.sandbox)
            print(published.pull_request.url if published.pull_request else published.branch)
            gitd.endpoint.revoke(issued.info.id, remove_sandbox=True)


if __name__ == "__main__":
    ModalRun(os.environ["REPOSITORY"], os.environ.get("BRANCH", "agents/modal")).work()
