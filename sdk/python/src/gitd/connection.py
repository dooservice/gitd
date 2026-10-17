"""The channel to gitd's Unix socket, and how a call on it is made and read."""

from __future__ import annotations

import os
from collections.abc import Callable
from typing import Any

import grpc
from google.protobuf.message import Message

from .errors import GitdError
from .methods import Method
from .models import Progress

DEFAULT_SOCKET = "/run/gitd/gitd.sock"
SOCKET_VARIABLE = "GITD_SOCKET"

OnProgress = Callable[[Progress], None]


class Connection:
    """A channel to gitd. The socket defaults to $GITD_SOCKET, then /run/gitd/gitd.sock; the
    timeout, in seconds, bounds every call, which gitd otherwise bounds itself."""

    def __init__(self, socket: str | None, timeout: float | None) -> None:
        self.socket = socket or os.environ.get(SOCKET_VARIABLE) or DEFAULT_SOCKET
        self.timeout = timeout
        self.channel = grpc.insecure_channel(f"unix:{self.socket}")

    def close(self) -> None:
        self.channel.close()

    @staticmethod
    def serialize(message: Message) -> bytes:
        return message.SerializeToString()

    def call(self, method: Method, request: Message) -> Any:
        """Makes a call that answers once."""
        invoke: grpc.UnaryUnaryMultiCallable[Message, Any] = self.channel.unary_unary(
            method.path,
            request_serializer=self.serialize,
            response_deserializer=method.response.FromString,
        )
        try:
            return invoke(request, timeout=self.timeout)
        except grpc.RpcError as error:
            raise GitdError.from_rpc_error(error) from None

    def follow(
        self,
        method: Method,
        request: Message,
        on_progress: OnProgress | None,
    ) -> Any:
        """Makes a call that streams its progress, and reads it to its result."""
        invoke: grpc.UnaryStreamMultiCallable[Message, Any] = self.channel.unary_stream(
            method.path,
            request_serializer=self.serialize,
            response_deserializer=method.response.FromString,
        )
        try:
            for response in invoke(request, timeout=self.timeout):
                event = response.WhichOneof("event")
                if event == "progress" and on_progress is not None:
                    on_progress(Progress.from_message(response.progress))
                elif event == "result":
                    return response.result
        except grpc.RpcError as error:
            raise GitdError.from_rpc_error(error) from None
        raise GitdError("", "gitd ended the call without a result")
