"""The calls that manage the credentials gitd keeps: for a platform's own pages, never for
an agent."""

from __future__ import annotations

from . import messages, methods
from .connection import Connection
from .kept_credentials import AppToCreate, Connected, KeptCredential, SignIn


class Credentials:
    """The credentials gitd keeps, as gitd.credentials.

    app = gitd.credentials.begin_github_app("acme-agents", "https://platform.example.com/github/created",
                                            organization="acme")
    # the browser posts app.manifest to app.form_action; GitHub sends it back with a code
    connected = gitd.credentials.complete_github_app(code, app.state)
    """

    def __init__(self, connection: Connection) -> None:
        self.connection = connection

    def list(self) -> list[KeptCredential]:
        """Every credential gitd keeps, gitd.toml's included, never a secret."""
        response = self.connection.call(methods.LIST_CONNECTIONS, messages.ListConnectionsRequest())
        return [KeptCredential.from_message(connection) for connection in response.connections]

    def remove(self, credential_id: str) -> None:
        """Forgets a credential kept through the API."""
        self.connection.call(methods.REMOVE_CONNECTION, messages.RemoveConnectionRequest(id=credential_id))

    def add_token(self, url_prefix: str, token: str, *, user: str = "") -> KeptCredential:
        """Keeps a token for the repositories under url_prefix, which names the host."""
        request = messages.AddTokenRequest(url_prefix=url_prefix, token=token, user=user)
        return KeptCredential.from_message(self.connection.call(methods.ADD_TOKEN, request).connection)

    def add_github_app(
        self, app_id: int, private_key: str, *, host: str = "", url_prefix: str = ""
    ) -> KeptCredential:
        """Keeps a GitHub App a platform already has, once GitHub takes the key as its own."""
        request = messages.AddGitHubAppRequest(
            host=host, app_id=app_id, private_key=private_key, url_prefix=url_prefix
        )
        return KeptCredential.from_message(self.connection.call(methods.ADD_GITHUB_APP, request).connection)

    def begin_github_app(
        self, name: str, redirect_url: str, *, organization: str = "", host: str = ""
    ) -> AppToCreate:
        """Starts creating the organisation's own GitHub App, named name; GitHub sends the
        person back to redirect_url with a code and the state."""
        request = messages.BeginGitHubAppRequest(
            host=host, organization=organization, name=name, redirect_url=redirect_url
        )
        return AppToCreate.from_message(self.connection.call(methods.BEGIN_GITHUB_APP, request))

    def complete_github_app(self, code: str, state: str) -> Connected:
        """Finishes creating the App with the code and state GitHub sent back."""
        request = messages.CompleteGitHubAppRequest(code=code, state=state)
        return Connected.from_message(self.connection.call(methods.COMPLETE_GITHUB_APP, request))

    def start_github_login(self, *, host: str = "") -> SignIn:
        """Asks GitHub for the code a person enters to sign in through gitd's public App."""
        request = messages.StartGitHubLoginRequest(host=host)
        return SignIn.from_message(self.connection.call(methods.START_GITHUB_LOGIN, request))

    def finish_github_login(self, login_id: str) -> Connected:
        """Waits until the person has entered the code, and keeps the sign-in."""
        request = messages.FinishGitHubLoginRequest(login_id=login_id)
        return Connected.from_message(self.connection.call(methods.FINISH_GITHUB_LOGIN, request))
