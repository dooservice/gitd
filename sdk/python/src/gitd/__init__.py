"""Client for gitd: instant, secretless git checkouts for agent sandboxes."""

from .client import Gitd
from .connection import DEFAULT_SOCKET, SOCKET_VARIABLE
from .errors import GitdError
from .kept_credentials import AppToCreate, Connected, CredentialKind, KeptCredential, SignIn
from .models import (
    Access,
    AuditEntry,
    ConfigurationChange,
    Description,
    Info,
    Layers,
    PendingSubmodule,
    Probe,
    Progress,
    Published,
    PublishedRepository,
    Refusal,
    Strategy,
    Submodule,
    Workspace,
)
from .publishing_settings import (
    OpenedPullRequest,
    Opening,
    Preset,
    PublishSettings,
    PullRequestOptions,
    PullRequestSettings,
)

__all__ = [
    "AppToCreate",
    "ConfigurationChange",
    "Connected",
    "CredentialKind",
    "KeptCredential",
    "OpenedPullRequest",
    "Opening",
    "Preset",
    "PublishSettings",
    "PullRequestOptions",
    "PullRequestSettings",
    "SignIn",
    "DEFAULT_SOCKET",
    "SOCKET_VARIABLE",
    "Access",
    "AuditEntry",
    "Description",
    "Gitd",
    "GitdError",
    "Info",
    "Layers",
    "PendingSubmodule",
    "Probe",
    "Progress",
    "Published",
    "PublishedRepository",
    "Refusal",
    "Strategy",
    "Submodule",
    "Workspace",
]
