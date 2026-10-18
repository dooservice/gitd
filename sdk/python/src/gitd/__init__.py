"""Client for gitd: instant, secretless git checkouts for agent sandboxes."""

from .changes import ChangedFile, ChangeKind, Changes, Diff
from .client import Gitd
from .comparison import Comparison, Conflict, Event, EventKind, Side
from .connection import DEFAULT_SOCKET, SOCKET_VARIABLE
from .endpoint_calls import Endpoint
from .endpoint_tokens import IssuedToken, TokenInfo
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
    PresetOrigin,
    Provenance,
    PublishSettings,
    PullRequestOptions,
    PullRequestSettings,
)
from .warm_layers import WarmLayer

__all__ = [
    "Comparison",
    "Conflict",
    "Event",
    "EventKind",
    "Side",
    "ChangeKind",
    "ChangedFile",
    "Changes",
    "Diff",
    "AppToCreate",
    "ConfigurationChange",
    "Connected",
    "CredentialKind",
    "KeptCredential",
    "OpenedPullRequest",
    "Opening",
    "Preset",
    "PresetOrigin",
    "Provenance",
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
    "WarmLayer",
    "Endpoint",
    "IssuedToken",
    "TokenInfo",
]
