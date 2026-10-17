"""Two works compared, their conflicts, and what happens to workspaces, as plain
dataclasses that write the messages gitd takes and read those it answers."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from enum import StrEnum
from typing import Any

from . import messages
from .changes import ChangedFile
from .timestamps import Timestamp

KIND_PREFIX = "KIND_"


@dataclass(frozen=True)
class Side:
    """One work: a workspace, by its path, or a branch gitd published or its pool holds."""

    workspace: str = ""
    branch: str = ""

    @classmethod
    def of_workspace(cls, path: str) -> Side:
        return cls(workspace=path)

    @classmethod
    def of_branch(cls, name: str) -> Side:
        return cls(branch=name)

    @classmethod
    def from_message(cls, side: Any) -> Side:
        return cls(workspace=side.workspace, branch=side.branch)

    def to_message(self) -> Any:
        if self.workspace:
            return messages.Side(workspace=self.workspace)
        return messages.Side(branch=self.branch)


@dataclass(frozen=True)
class Comparison:
    """How two works differ, from the first to the second, and the diff when asked."""

    files: list[ChangedFile] = field(default_factory=list)
    diff: str = ""
    truncated: bool = False

    @classmethod
    def from_message(cls, response: Any) -> Comparison:
        return cls(
            files=[ChangedFile.from_message(file) for file in response.files],
            diff=response.diff,
            truncated=response.truncated,
        )


@dataclass(frozen=True)
class Conflict:
    """Two works that do not merge cleanly, and the files they conflict on."""

    first: Side
    second: Side
    paths: list[str]

    @classmethod
    def from_message(cls, conflict: Any) -> Conflict:
        return cls(
            first=Side.from_message(conflict.first),
            second=Side.from_message(conflict.second),
            paths=list(conflict.paths),
        )


class EventKind(StrEnum):
    """What happened to a workspace."""

    MADE = "made"
    CHANGED = "changed"
    PUBLISHED = "published"
    REFUSED = "refused"
    REMOVED = "removed"

    @classmethod
    def from_message(cls, number: int) -> EventKind:
        name = messages.EVENT_KIND.name(number).removeprefix(KIND_PREFIX).lower()
        return cls(name) if name in {member.value for member in cls} else cls.CHANGED


@dataclass(frozen=True)
class Event:
    """What happened to a workspace, when; a publish's branch, and its pull request or why it
    was refused."""

    kind: EventKind
    path: str
    time: datetime
    branch: str
    detail: str

    @classmethod
    def from_message(cls, event: Any) -> Event:
        return cls(
            kind=EventKind.from_message(event.kind),
            path=event.path,
            time=Timestamp.to_datetime(event.time),
            branch=event.branch,
            detail=event.detail,
        )
