"""Tokens of gitd's endpoint, for sandboxes that clone over HTTP instead of mounting a
workspace: a microVM, a remote sandbox, a CI runner elsewhere."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Any

from .timestamps import Timestamp


@dataclass(frozen=True)
class TokenInfo:
    """A token as gitd shows it, never its secret: what it reads, the sandbox repository it
    pushes to when writable, and until when."""

    id: str
    url: str
    sandbox: str
    writable: bool
    issued_at: datetime
    expires_at: datetime

    @classmethod
    def from_message(cls, info: Any) -> TokenInfo:
        return cls(
            id=info.id,
            url=info.url,
            sandbox=info.sandbox,
            writable=info.writable,
            issued_at=Timestamp.to_datetime(info.issued_at),
            expires_at=Timestamp.to_datetime(info.expires_at),
        )


@dataclass(frozen=True)
class IssuedToken:
    """A token just issued: its secret, shown this once, and where the sandbox clones from —
    git clone https://x:<secret>@… — never the git host's credential."""

    secret: str
    clone_url: str
    info: TokenInfo

    @classmethod
    def from_message(cls, response: Any) -> IssuedToken:
        return cls(
            secret=response.token, clone_url=response.clone_url, info=TokenInfo.from_message(response.info)
        )

    def authenticated_url(self) -> str:
        """The clone URL with the secret in it, for the sandbox's git."""
        scheme, rest = self.clone_url.split("://", 1)
        return f"{scheme}://x:{self.secret}@{rest}"
