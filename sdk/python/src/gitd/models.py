"""What gitd answers, as plain dataclasses, each built from the message gitd sends. The
messages are made from the contract at run time, so their fields are typed here as Any."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from enum import StrEnum
from typing import Any

from . import messages
from .publishing_settings import OpenedPullRequest
from .timestamps import Timestamp

STRATEGY_PREFIX = "STRATEGY_"
ACTION_PREFIX = "ACTION_"
ACCESS_PREFIX = "ACCESS_"
REASON_PREFIX = "REASON_"


class Strategy(StrEnum):
    """How a workspace is made: a layer on a shared template, a block clone, or every file written."""

    OVERLAY = "overlay"
    REFLINK = "reflink"
    COPY = "copy"

    @classmethod
    def from_message(cls, number: int) -> Strategy:
        name = messages.STRATEGY.name(number).removeprefix(STRATEGY_PREFIX).lower()
        return cls(name) if name in {member.value for member in cls} else cls.COPY

    @classmethod
    def preference(cls, strategy: Strategy | None) -> int:
        """The number a request carries: the strategy asked for, or none, for gitd to choose."""
        name = strategy.value.upper() if strategy else "UNSPECIFIED"
        return messages.STRATEGY.number(STRATEGY_PREFIX + name)


class Access(StrEnum):
    """Whether a repository can be read: by anyone, with a credential gitd keeps, or not at all."""

    PUBLIC = "public"
    REACHABLE = "reachable"
    DENIED = "denied"

    @classmethod
    def from_message(cls, number: int) -> Access:
        name = messages.ACCESS.name(number).removeprefix(ACCESS_PREFIX).lower()
        return cls(name) if name in {member.value for member in cls} else cls.DENIED


@dataclass(frozen=True)
class Progress:
    """How far a fetch or checkout has gone; repository is the submodule's URL, empty for the
    workspace's own repository."""

    phase: str
    percent: int
    repository: str

    @classmethod
    def from_message(cls, progress: Any) -> Progress:
        return cls(phase=progress.phase, percent=progress.percent, repository=progress.repository)


@dataclass(frozen=True)
class Layers:
    """An overlay workspace's template (lower) and its own layer (upper, work)."""

    lower: str
    upper: str
    work: str

    @classmethod
    def from_message(cls, layers: Any) -> Layers:
        return cls(lower=layers.lower, upper=layers.upper, work=layers.work)


@dataclass(frozen=True)
class PendingSubmodule:
    """A submodule that could not be checked out, and what git said."""

    path: str
    url: str
    reason: str

    @classmethod
    def from_message(cls, submodule: Any) -> PendingSubmodule:
        return cls(path=submodule.path, url=submodule.url, reason=submodule.failure.git_said)


@dataclass(frozen=True)
class Workspace:
    """Where a workspace ended up. branch is empty when it is detached at a commit;
    lfs_patterns are the files the repository keeps in git LFS, which are pointers here, as
    gitd does not download them."""

    path: str
    commit: str
    branch: str
    strategy: Strategy
    layers: Layers | None = None
    pending_submodules: list[PendingSubmodule] = field(default_factory=list)
    lfs_patterns: list[str] = field(default_factory=list)

    @classmethod
    def from_message(cls, path: str, result: Any) -> Workspace:
        return cls(
            path=path,
            commit=result.commit,
            branch=result.branch,
            strategy=Strategy.from_message(result.strategy),
            layers=Layers.from_message(result.layers) if result.HasField("layers") else None,
            pending_submodules=[
                PendingSubmodule.from_message(pending) for pending in result.pending_submodules
            ],
            lfs_patterns=list(result.lfs_patterns),
        )


@dataclass(frozen=True)
class Submodule:
    path: str
    url: str
    commit: str
    checked_out: bool

    @classmethod
    def from_message(cls, submodule: Any) -> Submodule:
        return cls(
            path=submodule.path,
            url=submodule.url,
            commit=submodule.commit,
            checked_out=submodule.checked_out,
        )


@dataclass(frozen=True)
class Description:
    """What a workspace is on disk. modified says whether it has uncommitted changes;
    lfs_patterns are the files kept in git LFS, which are pointers here."""

    url: str
    commit: str
    branch: str
    modified: bool
    strategy: Strategy
    submodules: list[Submodule] = field(default_factory=list)
    lfs_patterns: list[str] = field(default_factory=list)

    @classmethod
    def from_message(cls, response: Any) -> Description:
        return cls(
            url=response.url,
            commit=response.commit,
            branch=response.branch,
            modified=response.modified,
            strategy=Strategy.from_message(response.strategy),
            submodules=[Submodule.from_message(submodule) for submodule in response.submodules],
            lfs_patterns=list(response.lfs_patterns),
        )


@dataclass(frozen=True)
class PublishedRepository:
    """A repository a publish pushed. path is the submodule's, empty for the workspace's own
    repository; subjects are the new commits' subjects, newest first."""

    url: str
    path: str
    commit: str
    subjects: list[str]

    @classmethod
    def from_message(cls, repository: Any) -> PublishedRepository:
        return cls(
            url=repository.url,
            path=repository.path,
            commit=repository.commit,
            subjects=list(repository.subjects),
        )


@dataclass(frozen=True)
class Refusal:
    """Why gitd refused a call: its reason (BRANCH_REFUSED, AUTH_REQUIRED, …) and what git said."""

    reason: str
    said: str

    @classmethod
    def from_message(cls, failure: Any) -> Refusal:
        return cls(reason=cls.reason_of(failure), said=failure.git_said)

    @staticmethod
    def reason_of(failure: Any) -> str:
        return messages.REASON.name(failure.reason).removeprefix(REASON_PREFIX)


@dataclass(frozen=True)
class Published:
    """What a publish pushed: the workspace's repository first, then each submodule; the
    pull request it opened, or why it could not, the branch being published either way."""

    branch: str
    repositories: list[PublishedRepository]
    pull_request: OpenedPullRequest | None = None
    pull_request_refused: Refusal | None = None

    @classmethod
    def from_message(cls, response: Any) -> Published:
        pushed = [response.repository, *response.submodules] if response.HasField("repository") else []
        return cls(
            branch=response.branch,
            repositories=[PublishedRepository.from_message(repository) for repository in pushed],
            pull_request=(
                OpenedPullRequest.from_message(response.pull_request)
                if response.HasField("pull_request")
                else None
            ),
            pull_request_refused=(
                Refusal.from_message(response.pull_request_failure)
                if response.HasField("pull_request_failure")
                else None
            ),
        )


@dataclass(frozen=True)
class Info:
    """Which gitd answers, and how it makes workspaces on its machine and why."""

    version: str
    git_version: str
    pools: int
    pools_bytes: int
    templates: int
    templates_bytes: int
    strategy: Strategy
    strategy_reason: str

    @classmethod
    def from_message(cls, response: Any) -> Info:
        return cls(
            version=response.version,
            git_version=response.git_version,
            pools=response.pools,
            pools_bytes=response.pools_bytes,
            templates=response.templates,
            templates_bytes=response.templates_bytes,
            strategy=Strategy.from_message(response.strategy),
            strategy_reason=response.strategy_reason,
        )


@dataclass(frozen=True)
class Probe:
    """Whether a repository can be read, its default branch, and each branch's commit.
    denied says why it cannot be read, when it cannot."""

    access: Access
    default_branch: str
    branches: dict[str, str]
    denied: str | None = None

    @classmethod
    def from_message(cls, response: Any) -> Probe:
        return cls(
            access=Access.from_message(response.access),
            default_branch=response.default_branch,
            branches=dict(response.commits),
            denied=response.failure.git_said if response.HasField("failure") else None,
        )


@dataclass(frozen=True)
class ConfigurationChange:
    """A change to the credentials or publishing settings: connection_added,
    connection_removed, settings_set or settings_removed; subject is the credential's id or
    the settings' prefix."""

    action: str
    subject: str
    url_prefix: str

    @classmethod
    def from_message(cls, change: Any) -> ConfigurationChange:
        action = messages.CHANGE_ACTION.name(change.action).removeprefix(ACTION_PREFIX).lower()
        return cls(action=action, subject=change.subject, url_prefix=change.url_prefix)


@dataclass(frozen=True)
class AuditEntry:
    """One publish, done or refused, or one change to credentials or settings, as the audit
    log keeps it. url is the workspace's repository, empty when the workspace could not be
    opened; repositories is what was pushed, the workspace's repository first, empty when
    refused; requested_by and the pull request are as the publish said and did."""

    time: datetime
    workspace: str
    url: str
    branch: str
    repositories: list[PublishedRepository] = field(default_factory=list)
    refused: Refusal | None = None
    requested_by: str = ""
    pull_request: OpenedPullRequest | None = None
    pull_request_refused: Refusal | None = None
    change: ConfigurationChange | None = None

    @classmethod
    def from_message(cls, entry: Any) -> AuditEntry:
        pushed = [entry.repository, *entry.submodules] if entry.HasField("repository") else []
        return cls(
            time=Timestamp.to_datetime(entry.time),
            workspace=entry.workspace,
            url=entry.url,
            branch=entry.branch,
            repositories=[PublishedRepository.from_message(repository) for repository in pushed],
            refused=Refusal.from_message(entry.refusal) if entry.HasField("refusal") else None,
            requested_by=entry.requested_by,
            pull_request=OpenedPullRequest.from_message(entry.pull_request)
            if entry.HasField("pull_request")
            else None,
            pull_request_refused=(
                Refusal.from_message(entry.pull_request_failure)
                if entry.HasField("pull_request_failure")
                else None
            ),
            change=ConfigurationChange.from_message(entry.change) if entry.HasField("change") else None,
        )
