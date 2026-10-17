"""What was changed in a workspace, and its diff, as plain dataclasses built from the
messages gitd answers."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum
from typing import Any

from . import messages

KIND_PREFIX = "KIND_"


class ChangeKind(StrEnum):
    """How a file changed: added, modified or deleted."""

    ADDED = "added"
    MODIFIED = "modified"
    DELETED = "deleted"

    @classmethod
    def from_message(cls, number: int) -> ChangeKind:
        name = messages.CHANGE_KIND.name(number).removeprefix(KIND_PREFIX).lower()
        return cls(name) if name in {member.value for member in cls} else cls.MODIFIED


@dataclass(frozen=True)
class ChangedFile:
    """A file changed, relative to the workspace, with its lines; a binary one counts none."""

    path: str
    kind: ChangeKind
    added_lines: int
    removed_lines: int
    binary: bool

    @classmethod
    def from_message(cls, file: Any) -> ChangedFile:
        return cls(
            path=file.path,
            kind=ChangeKind.from_message(file.kind),
            added_lines=file.added_lines,
            removed_lines=file.removed_lines,
            binary=file.binary,
        )


@dataclass(frozen=True)
class Changes:
    """What was changed in a workspace since gitd put it on base, committed or not.
    from_layer says it was read from the agent's own layer, with no git run in the workspace."""

    base: str
    from_layer: bool
    files: list[ChangedFile] = field(default_factory=list)

    @classmethod
    def from_message(cls, response: Any) -> Changes:
        return cls(
            base=response.base,
            from_layer=response.from_layer,
            files=[ChangedFile.from_message(file) for file in response.files],
        )


@dataclass(frozen=True)
class Diff:
    """The unified diff of a workspace's changes; truncated says it was cut short."""

    base: str
    diff: str
    truncated: bool

    @classmethod
    def from_message(cls, response: Any) -> Diff:
        return cls(base=response.base, diff=response.diff, truncated=response.truncated)
