"""The channel to gitd — its Unix socket, or its address on the network — and how a call on
it is made and read."""

from __future__ import annotations

import os
from collections.abc import Callable, Iterator
from typing import Any

import grpc
from google.protobuf.message import Message

from .errors import GitdError
from .methods import Method
from .models import Progress

DEFAULT_SOCKET = "/run/gitd/gitd.sock"
SOCKET_VARIABLE = "GITD_SOCKET"
URL_VARIABLE = "GITD_URL"
KEY_VARIABLE = "GITD_KEY"
KEY_FILE_VARIABLE = "GITD_KEY_FILE"
CA_FILE_VARIABLE = "GITD_CA_FILE"

OnProgress = Callable[[Progress], None]


class Connection:
    """A channel to gitd: at url on the network (default $GITD_URL: https://host:port, or
    http:// on a private network), else on its socket (default $GITD_SOCKET, then
    /run/gitd/gitd.sock). key names the caller's tenant (default the file $GITD_KEY_FILE names,
    then $GITD_KEY); ca_file is the authority of a certificate the system does not trust
    (default $GITD_CA_FILE). The timeout, in seconds, bounds every call, which gitd otherwise
    bounds itself."""

    def __init__(
        self,
        socket: str | None,
        timeout: float | None,
        url: str | None = None,
        key: str | None = None,
        ca_file: str | None = None,
    ) -> None:
        self.socket = socket or os.environ.get(SOCKET_VARIABLE) or DEFAULT_SOCKET
        self.timeout = timeout
        key = key or self.key_from_environment()
        self.metadata = (("authorization", f"Bearer {key}"),) if key else ()
        url = url or os.environ.get(URL_VARIABLE)
        if not url:
            self.channel = grpc.insecure_channel(f"unix:{self.socket}")
            return
        secure, target = self.target_of(url)
        if not secure:
            self.channel = grpc.insecure_channel(target)
            return
        ca_file = ca_file or os.environ.get(CA_FILE_VARIABLE)
        roots = open(ca_file, "rb").read() if ca_file else None  # noqa: SIM115
        self.channel = grpc.secure_channel(target, grpc.ssl_channel_credentials(root_certificates=roots))

    @staticmethod
    def key_from_environment() -> str:
        key_file = os.environ.get(KEY_FILE_VARIABLE)
        if key_file:
            with open(key_file) as opened:
                return opened.read().strip()
        return os.environ.get(KEY_VARIABLE, "").strip()

    @staticmethod
    def target_of(url: str) -> tuple[bool, str]:
        """Whether url is served over TLS, and the host:port gRPC dials."""
        for scheme, secure, port in (("https://", True, 443), ("http://", False, 80)):
            if url.startswith(scheme):
                host = url[len(scheme) :].rstrip("/")
                return secure, host if ":" in host else f"{host}:{port}"
        raise GitdError("BAD_ADDRESS", f"{url!r} is not the address of a gitd: https://host:port")

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
            return invoke(request, timeout=self.timeout, metadata=self.metadata)
        except grpc.RpcError as error:
            raise GitdError.from_rpc_error(error) from None

    def stream(self, method: Method, request: Message) -> Iterator[Any]:
        """Makes a call that streams messages until gitd or the caller ends it."""
        invoke: grpc.UnaryStreamMultiCallable[Message, Any] = self.channel.unary_stream(
            method.path,
            request_serializer=self.serialize,
            response_deserializer=method.response.FromString,
        )
        try:
            yield from invoke(request, timeout=self.timeout, metadata=self.metadata)
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
            for response in invoke(request, timeout=self.timeout, metadata=self.metadata):
                event = response.WhichOneof("event")
                if event == "progress" and on_progress is not None:
                    on_progress(Progress.from_message(response.progress))
                elif event == "result":
                    return response.result
        except grpc.RpcError as error:
            raise GitdError.from_rpc_error(error) from None
        raise GitdError("", "gitd ended the call without a result")
