"""Warm layers: what installing a repository's dependencies wrote, sealed from one
workspace for the next ones to stand on."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any


@dataclass(frozen=True)
class WarmLayer:
    """A layer sealed from a workspace. Workspaces stand on it at commit only; changed_files
    are the files it changes that git does not ignore, and users the workspaces on it now."""

    url: str
    name: str
    commit: str
    sealed_at: datetime
    sealed_from: str
    changed_files: int
    users: int

    @classmethod
    def from_message(cls, warm: Any) -> WarmLayer:
        return cls(
            url=warm.url,
            name=warm.name,
            commit=warm.commit,
            sealed_at=datetime.fromtimestamp(warm.sealed_at, tz=UTC),
            sealed_from=warm.sealed_from,
            changed_files=warm.changed_files,
            users=warm.users,
        )
