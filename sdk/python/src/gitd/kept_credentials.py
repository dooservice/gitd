"""The credentials gitd keeps, and the steps of connecting GitHub, as plain dataclasses
built from the messages gitd answers. None of them ever holds a secret."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from typing import Any

from . import messages
from .timestamps import Timestamp

KIND_PREFIX = "KIND_"


class CredentialKind(StrEnum):
    """What a credential is: a token, an SSH key, a GitHub App, or a person signed in."""

    TOKEN = "token"
    SSH_KEY = "ssh_key"
    GITHUB_APP = "github_app"
    GITHUB_LOGIN = "github_login"

    @classmethod
    def from_message(cls, number: int) -> CredentialKind:
        name = messages.CONNECTION_KIND.name(number).removeprefix(KIND_PREFIX).lower()
        return cls(name) if name in {member.value for member in cls} else cls.TOKEN


@dataclass(frozen=True)
class KeptCredential:
    """One credential gitd keeps: its id to remove it by, what it is, the repositories it
    opens, whom it acts as (an App's bot or a person), an App's id and where it is managed,
    when a sign-in stops renewing, and whether gitd.toml declares it."""

    id: str
    kind: CredentialKind
    url_prefix: str
    acts_as: str
    app_id: int
    manage_url: str
    expires: datetime | None
    from_file: bool
    created: datetime | None

    @classmethod
    def from_message(cls, connection: Any) -> KeptCredential:
        return cls(
            id=connection.id,
            kind=CredentialKind.from_message(connection.kind),
            url_prefix=connection.url_prefix,
            acts_as=connection.acts_as,
            app_id=connection.app_id,
            manage_url=connection.manage_url,
            expires=Timestamp.to_datetime(connection.expires) if connection.HasField("expires") else None,
            from_file=connection.from_file,
            created=Timestamp.to_datetime(connection.created) if connection.HasField("created") else None,
        )


@dataclass(frozen=True)
class Connected:
    """A GitHub App or sign-in gitd now keeps, and where it is installed on repositories."""

    credential: KeptCredential
    install_url: str

    @classmethod
    def from_message(cls, response: Any) -> Connected:
        return cls(
            credential=KeptCredential.from_message(response.connection), install_url=response.install_url
        )


@dataclass(frozen=True)
class AppToCreate:
    """What a browser posts to GitHub to create an App: where (form_action), the manifest,
    as a form field named manifest, and the state GitHub sends back with the code."""

    form_action: str
    manifest: str
    state: str

    @classmethod
    def from_message(cls, response: Any) -> AppToCreate:
        return cls(form_action=response.form_action, manifest=response.manifest, state=response.state)


@dataclass(frozen=True)
class SignIn:
    """A sign-in waiting for a person: the code they enter at verification_url, for how
    long, and the id to finish it by."""

    user_code: str
    verification_url: str
    login_id: str
    expires_in_seconds: int

    @classmethod
    def from_message(cls, response: Any) -> SignIn:
        return cls(
            user_code=response.user_code,
            verification_url=response.verification_url,
            login_id=response.login_id,
            expires_in_seconds=response.expires_in_seconds,
        )
