"""The gitd client."""

from __future__ import annotations

from . import messages, methods
from .connection import Connection, OnProgress
from .models import Description, Info, Probe, Published, Strategy, Workspace


class Gitd:
    """A gitd on its Unix socket. No call takes a credential: gitd reads and publishes with
    the ones it keeps.

        with Gitd() as gitd:
            workspace = gitd.prepare("https://github.com/acme/web", "/var/lib/gitd/workspaces/agent-1")
    """

    def __init__(self, socket: str | None = None, *, timeout: float | None = None) -> None:
        self.connection = Connection(socket, timeout)

    def __enter__(self) -> Gitd:
        return self

    def __exit__(self, *exception: object) -> None:
        self.close()

    def close(self) -> None:
        self.connection.close()

    def info(self) -> Info:
        """Which gitd answers, and how it makes workspaces here and why."""
        return Info.from_message(self.connection.call(methods.INFO, messages.InfoRequest()))

    def probe(self, url: str) -> Probe:
        """Whether a repository can be read with the credentials gitd keeps, and its branches."""
        return Probe.from_message(self.connection.call(methods.PROBE, messages.ProbeRequest(url=url)))

    def prepare(
        self,
        url: str,
        path: str,
        *,
        branch: str | None = None,
        commit: str | None = None,
        submodules: bool = False,
        full_history: bool = False,
        strategy: Strategy | None = None,
        on_progress: OnProgress | None = None,
    ) -> Workspace:
        """Makes a workspace of a repository at a path under gitd's roots, at a commit, a
        branch, or the default branch. strategy is a preference: gitd falls back to copy when
        it cannot make one that way here."""
        if commit:
            revision = messages.Revision(commit=commit)
        elif branch:
            revision = messages.Revision(branch=branch)
        else:
            revision = None
        request = messages.PrepareRequest(
            url=url,
            path=path,
            revision=revision,
            with_submodules=submodules,
            full_history=full_history,
            strategy=Strategy.preference(strategy),
        )
        return Workspace.from_message(path, self.connection.follow(methods.PREPARE, request, on_progress))

    def update(
        self,
        path: str,
        *,
        branch: str | None = None,
        submodules: bool = False,
        on_progress: OnProgress | None = None,
    ) -> Workspace:
        """Brings a workspace to the tip of its branch, or of the branch given."""
        request = messages.UpdateRequest(path=path, branch=branch or "", with_submodules=submodules)
        return Workspace.from_message(path, self.connection.follow(methods.UPDATE, request, on_progress))

    def checkout(
        self,
        path: str,
        commit: str,
        *,
        submodules: bool = False,
        on_progress: OnProgress | None = None,
    ) -> Workspace:
        """Moves a workspace to a commit, as a rollback does."""
        request = messages.CheckoutRequest(path=path, commit=commit, with_submodules=submodules)
        return Workspace.from_message(path, self.connection.follow(methods.CHECKOUT, request, on_progress))

    def describe(self, path: str) -> Description:
        """What a workspace is on disk, without the network."""
        response = self.connection.call(methods.DESCRIBE, messages.DescribeRequest(path=path))
        return Description.from_message(response)

    def publish(self, path: str, branch: str) -> Published:
        """Pushes what was committed in a workspace to a new branch, with the token gitd keeps."""
        response = self.connection.call(methods.PUBLISH, messages.PublishRequest(path=path, branch=branch))
        return Published.from_message(response)

    def remove(self, path: str) -> None:
        """Deletes a workspace; its pool stays."""
        self.connection.call(methods.REMOVE, messages.RemoveRequest(path=path))
