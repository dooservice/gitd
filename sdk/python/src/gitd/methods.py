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

LIST_CONNECTIONS = Method("ConnectionService", "ListConnections", messages.ListConnectionsResponse)
REMOVE_CONNECTION = Method("ConnectionService", "RemoveConnection", messages.RemoveConnectionResponse)
ADD_TOKEN = Method("ConnectionService", "AddToken", messages.AddTokenResponse)
ADD_GITHUB_APP = Method("ConnectionService", "AddGitHubApp", messages.AddGitHubAppResponse)
BEGIN_GITHUB_APP = Method("ConnectionService", "BeginGitHubApp", messages.BeginGitHubAppResponse)
COMPLETE_GITHUB_APP = Method("ConnectionService", "CompleteGitHubApp", messages.CompleteGitHubAppResponse)
START_GITHUB_LOGIN = Method("ConnectionService", "StartGitHubLogin", messages.StartGitHubLoginResponse)
FINISH_GITHUB_LOGIN = Method("ConnectionService", "FinishGitHubLogin", messages.FinishGitHubLoginResponse)

GET_SETTINGS = Method("PublishingService", "GetSettings", messages.GetSettingsResponse)
SET_SETTINGS = Method("PublishingService", "SetSettings", messages.SetSettingsResponse)
REMOVE_SETTINGS = Method("PublishingService", "RemoveSettings", messages.RemoveSettingsResponse)
