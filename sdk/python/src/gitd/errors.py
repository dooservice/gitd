"""A refusal from gitd."""

from __future__ import annotations

from typing import Any

import grpc
from grpc_status import rpc_status

from . import messages
from .models import Refusal


class GitdError(Exception):
    """A refusal from gitd: its reason (AUTH_REQUIRED, NOT_FOUND, PATH_REFUSED, BUSY, …) and
    what git said. Tell users what happened from the reason, never by parsing the message."""

    def __init__(self, reason: str, said: str, code: grpc.StatusCode | None = None) -> None:
        super().__init__(f"{reason}: {said}" if reason else said)
        self.reason = reason
        self.said = said
        self.code = code

    @classmethod
    def from_rpc_error(cls, error: grpc.RpcError) -> GitdError:
        """Reads the GitFailure gitd attaches to a refused call, if there is one."""
        if not isinstance(error, grpc.Call):
            return cls("", str(error))
        status = rpc_status.from_call(error)
        details = list(status.details) if status else []
        for detail in details:
            if detail.Is(messages.GitFailure.DESCRIPTOR):
                failure: Any = messages.GitFailure()
                detail.Unpack(failure)
                return cls(Refusal.reason_of(failure), failure.git_said, error.code())
        return cls("", error.details() or "", error.code())
