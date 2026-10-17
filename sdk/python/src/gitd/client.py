"""The gitd client."""

from __future__ import annotations

from collections.abc import Iterator
from datetime import datetime
from typing import Any

from . import messages, methods
from .changes import Changes, Diff
from .comparison import Comparison, Conflict, Event, Side
from .connection import Connection, OnProgress
from .credentials_calls import Credentials
from .models import AuditEntry, Description, Info, Probe, Published, Strategy, Workspace
from .publishing_calls import Publishing
from .publishing_settings import PullRequestOptions
from .timestamps import Timestamp


class Gitd:
    """A gitd on its Unix socket. No call on workspaces takes a credential: gitd reads and
    publishes with the ones it keeps, which a platform manages through gitd.credentials, and
    publishing follows the rules it sets through gitd.publishing.

        with Gitd() as gitd:
            workspace = gitd.prepare("https://github.com/acme/web", "/var/lib/gitd/workspaces/agent-1")
    """

    def __init__(self, socket: str | None = None, *, timeout: float | None = None) -> None:
        self.connection = Connection(socket, timeout)
        self.credentials = Credentials(self.connection)
        self.publishing = Publishing(self.connection)

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

    def publish(
        self,
        path: str,
        branch: str,
        *,
        requested_by: str = "",
        pull_request: PullRequestOptions | None = None,
    ) -> Published:
        """Pushes what was committed in a workspace to a new branch, with the credential gitd
        keeps, and opens its pull request when the publishing settings or pull_request say
        so. requested_by is who asked for the work, as their account on the host."""
        request: Any = messages.PublishRequest(path=path, branch=branch, requested_by=requested_by)
        if pull_request is not None:
            request.pull_request.CopyFrom(pull_request.to_message())
        return Published.from_message(self.connection.call(methods.PUBLISH, request))

    def changes(self, path: str, *, include_ignored: bool = False) -> Changes:
        """What was changed in a workspace since gitd put it on its commit, committed or not."""
        request = messages.ChangesRequest(path=path, include_ignored=include_ignored)
        return Changes.from_message(self.connection.call(methods.CHANGES, request))

    def diff(
        self,
        path: str,
        files: list[str] | None = None,
        *,
        max_bytes: int = 0,
        include_ignored: bool = False,
    ) -> Diff:
        """The unified diff of a workspace's changes, of some files or all, cut short at
        max_bytes (512 KiB by default)."""
        request = messages.DiffRequest(
            path=path, files=files or [], max_bytes=max_bytes, include_ignored=include_ignored
        )
        return Diff.from_message(self.connection.call(methods.DIFF, request))

    def compare(
        self,
        first: Side,
        second: Side,
        *,
        url: str = "",
        with_diff: bool = False,
        max_bytes: int = 0,
        include_ignored: bool = False,
    ) -> Comparison:
        """How two works of one repository differ, from first to second; url is the
        repository when both are branches."""
        request = messages.CompareRequest(
            first=first.to_message(),
            second=second.to_message(),
            url=url,
            with_diff=with_diff,
            max_bytes=max_bytes,
            include_ignored=include_ignored,
        )
        return Comparison.from_message(self.connection.call(methods.COMPARE, request))

    def conflicts(self, sides: list[Side], *, url: str = "") -> list[Conflict]:
        """Which pairs of works do not merge cleanly, and on which files; a workspace stands
        for what it committed."""
        request = messages.ConflictsRequest(sides=[side.to_message() for side in sides], url=url)
        response = self.connection.call(methods.CONFLICTS, request)
        return [Conflict.from_message(conflict) for conflict in response.conflicts]

    def watch(self, paths: list[str] | None = None) -> Iterator[Event]:
        """What happens to the workspaces at paths, or to every one, as it happens."""
        request = messages.WatchRequest(paths=paths or [])
        for event in self.connection.stream(methods.WATCH, request):
            yield Event.from_message(event)

    def remove(self, path: str) -> None:
        """Deletes a workspace; its pool stays."""
        self.connection.call(methods.REMOVE, messages.RemoveRequest(path=path))

    def audit(
        self,
        *,
        since: datetime | None = None,
        url: str | None = None,
        limit: int | None = None,
    ) -> list[AuditEntry]:
        """The audit log: every publish, done or refused, newest first; from since on, of
        the repository at url, at most limit of them (100 by default, never more than 1000)."""
        request: Any = messages.AuditRequest(url=url or "", limit=limit or 0)
        if since is not None:
            Timestamp.fill(request.since, since)
        response = self.connection.call(methods.AUDIT, request)
        return [AuditEntry.from_message(entry) for entry in response.entries]
