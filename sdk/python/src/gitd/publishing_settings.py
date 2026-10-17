"""The rules publishing follows, and what a publish says of its pull request, as plain
dataclasses that write the messages gitd takes and read those it answers."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum
from typing import Any

from . import messages

OPENING_PREFIX = "OPEN_"


class Opening(StrEnum):
    """When publishes open pull requests: only when a publish asks, or always."""

    WHEN_ASKED = "when_asked"
    ALWAYS = "always"

    @classmethod
    def from_message(cls, number: int) -> Opening:
        name = messages.OPENING.name(number).removeprefix(OPENING_PREFIX).lower()
        return cls(name) if name in {member.value for member in cls} else cls.WHEN_ASKED

    def to_message(self) -> int:
        return messages.OPENING.number(OPENING_PREFIX + self.value.upper())


@dataclass(frozen=True)
class PullRequestSettings:
    """How publishes open pull requests: when, as drafts or not, with which labels, and
    whether the person a publish names as requester is asked to review."""

    opening: Opening = Opening.WHEN_ASKED
    draft: bool = False
    labels: list[str] = field(default_factory=list)
    request_review_from_requester: bool = False

    @classmethod
    def from_message(cls, settings: Any) -> PullRequestSettings:
        return cls(
            opening=Opening.from_message(settings.open),
            draft=settings.draft,
            labels=list(settings.labels),
            request_review_from_requester=settings.request_review_from_requester,
        )

    def to_message(self) -> Any:
        return messages.PullRequestSettings(
            open=self.opening.to_message(),
            draft=self.draft,
            labels=self.labels,
            request_review_from_requester=self.request_review_from_requester,
        )


@dataclass(frozen=True)
class PublishSettings:
    """The rules of the repositories whose address starts with url_prefix (empty for every
    repository): the name of a preset whose rules they take ("recommended" is gitd's own),
    paths a publish may not change and paths it may not delete, as git's glob pathspecs, at
    most how many files it may change, the branch prefix, and how pull requests are opened.
    from_file says gitd.toml declares them."""

    url_prefix: str = ""
    preset: str = ""
    protected_paths: list[str] = field(default_factory=list)
    kept_paths: list[str] = field(default_factory=list)
    max_changed_files: int | None = None
    branch_prefix: str = ""
    pull_requests: PullRequestSettings | None = None
    from_file: bool = False

    @classmethod
    def from_message(cls, settings: Any) -> PublishSettings:
        return cls(
            url_prefix=settings.url_prefix,
            preset=settings.preset,
            protected_paths=list(settings.protected_paths),
            kept_paths=list(settings.kept_paths),
            max_changed_files=settings.max_changed_files if settings.HasField("max_changed_files") else None,
            branch_prefix=settings.branch_prefix,
            pull_requests=(
                PullRequestSettings.from_message(settings.pull_requests)
                if settings.HasField("pull_requests")
                else None
            ),
            from_file=settings.from_file,
        )

    def to_message(self) -> Any:
        message: Any = messages.PublishSettings(
            url_prefix=self.url_prefix,
            preset=self.preset,
            protected_paths=self.protected_paths,
            kept_paths=self.kept_paths,
            branch_prefix=self.branch_prefix,
        )
        if self.max_changed_files is not None:
            message.max_changed_files = self.max_changed_files
        if self.pull_requests is not None:
            message.pull_requests.CopyFrom(self.pull_requests.to_message())
        return message


@dataclass(frozen=True)
class PullRequestOptions:
    """A publish's own say on its pull request: open one even when the settings open them
    only when asked, its title (the newest commit's subject when empty), the text before
    what gitd checked, the branch it merges into, and whether it is a draft."""

    open: bool = False
    title: str = ""
    body: str = ""
    base: str = ""
    draft: bool | None = None

    def to_message(self) -> Any:
        message: Any = messages.PullRequestOptions(
            open=self.open, title=self.title, body=self.body, base=self.base
        )
        if self.draft is not None:
            message.draft = self.draft
        return message


@dataclass(frozen=True)
class OpenedPullRequest:
    """A pull request gitd opened."""

    url: str
    number: int

    @classmethod
    def from_message(cls, opened: Any) -> OpenedPullRequest:
        return cls(url=opened.url, number=opened.number)
