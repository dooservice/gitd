"""Client for gitd: instant, secretless git checkouts for agent sandboxes."""

from .client import Gitd
from .connection import DEFAULT_SOCKET, SOCKET_VARIABLE
from .errors import GitdError
from .models import (
    Access,
    Description,
    Info,
    Layers,
    PendingSubmodule,
    Probe,
    Progress,
    Published,
    PublishedRepository,
    Strategy,
    Submodule,
    Workspace,
)

__all__ = [
    "DEFAULT_SOCKET",
    "SOCKET_VARIABLE",
    "Access",
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
    "Strategy",
    "Submodule",
    "Workspace",
]
