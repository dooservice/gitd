"""The gitd methods this client calls, by their gRPC path and the message each answers with."""

from __future__ import annotations

from dataclasses import dataclass

from google.protobuf.message import Message

from . import messages
from .contract import PACKAGE


@dataclass(frozen=True)
class Method:
    service: str
    name: str
    response: type[Message]

    @property
    def path(self) -> str:
        return f"/{PACKAGE}.{self.service}/{self.name}"


INFO = Method("ServerService", "Info", messages.InfoResponse)
AUDIT = Method("ServerService", "Audit", messages.AuditResponse)
PROBE = Method("RepositoryService", "Probe", messages.ProbeResponse)
PREPARE = Method("WorkspaceService", "Prepare", messages.PrepareResponse)
UPDATE = Method("WorkspaceService", "Update", messages.UpdateResponse)
CHECKOUT = Method("WorkspaceService", "Checkout", messages.CheckoutResponse)
DESCRIBE = Method("WorkspaceService", "Describe", messages.DescribeResponse)
PUBLISH = Method("WorkspaceService", "Publish", messages.PublishResponse)
REMOVE = Method("WorkspaceService", "Remove", messages.RemoveResponse)
