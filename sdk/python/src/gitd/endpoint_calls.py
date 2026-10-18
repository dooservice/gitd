"""The calls of gitd's endpoint: tokens for sandboxes elsewhere. What a sandbox pushes is
published with Gitd.publish and its sandbox."""

from __future__ import annotations

from datetime import timedelta

from . import messages, methods
from .connection import Connection
from .endpoint_tokens import IssuedToken, TokenInfo


class Endpoint:
    """gitd's endpoint, as gitd.endpoint.

    issued = gitd.endpoint.issue("https://github.com/acme/web", lifetime=timedelta(hours=2))
    # in the sandbox: git clone issued.authenticated_url() …; git push origin HEAD:agents/fix
    gitd.publish("", "agents/fix", sandbox=issued.info.sandbox)
    """

    def __init__(self, connection: Connection) -> None:
        self.connection = connection

    def issue(
        self,
        url: str,
        *,
        branch: str = "",
        lifetime: timedelta | None = None,
        read_only: bool = False,
        sandbox: str = "",
    ) -> IssuedToken:
        """A token for a sandbox: it reads url and pushes to its own sandbox repository (an
        earlier one when sandbox names it), for lifetime (an hour by default, a day at most)."""
        request = messages.IssueTokenRequest(
            url=url,
            branch=branch,
            lifetime_seconds=int(lifetime.total_seconds()) if lifetime else 0,
            read_only=read_only,
            sandbox=sandbox,
        )
        return IssuedToken.from_message(self.connection.call(methods.ISSUE_TOKEN, request))

    def revoke(self, token_id: str, *, remove_sandbox: bool = False) -> None:
        """Ends a token now, and removes its sandbox repository when asked."""
        request = messages.RevokeTokenRequest(id=token_id, remove_sandbox=remove_sandbox)
        self.connection.call(methods.REVOKE_TOKEN, request)

    def tokens(self) -> list[TokenInfo]:
        """The tokens alive, never their secrets."""
        response = self.connection.call(methods.LIST_TOKENS, messages.ListTokensRequest())
        return [TokenInfo.from_message(info) for info in response.tokens]
