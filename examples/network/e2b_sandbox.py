"""An E2B sandbox clones from gitd's endpoint, commits and pushes, and gitd publishes it.

Needs `pip install gitd e2b`, E2B_API_KEY, and a gitd E2B can reach over the internet:
GITD_URL (https://…), GITD_KEY_FILE (the tenant's key), REPOSITORY and BRANCH. Not run in CI.
"""

from __future__ import annotations

import os
import shlex

from e2b import Sandbox
from gitd import Gitd


class E2BRun:
    """One agent run in an E2B sandbox, on a repository gitd keeps the credential of."""

    def __init__(self, repository: str, branch: str) -> None:
        self.repository = repository
        self.branch = branch

    def work(self) -> None:
        with Gitd() as gitd:
            issued = gitd.endpoint.issue(self.repository)
            sandbox = Sandbox.create()
            try:
                script = " && ".join(
                    [
                        f"git clone -q {shlex.quote(issued.authenticated_url())} /home/user/work",
                        "cd /home/user/work",
                        "git -c user.name=agent -c user.email=agent@example.com"
                        " commit -q --allow-empty -m 'from e2b'",
                        f"git push -q origin HEAD:refs/heads/{shlex.quote(self.branch)}",
                    ]
                )
                print(sandbox.commands.run(script).stdout)
            finally:
                sandbox.kill()
            published = gitd.publish("", self.branch, sandbox=issued.info.sandbox)
            print(published.pull_request.url if published.pull_request else published.branch)
            gitd.endpoint.revoke(issued.info.id, remove_sandbox=True)


if __name__ == "__main__":
    E2BRun(os.environ["REPOSITORY"], os.environ.get("BRANCH", "agents/e2b")).work()
