"""Between the contract's google.protobuf.Timestamp and Python's datetime."""

from __future__ import annotations

from datetime import UTC, datetime
from typing import Any

NANOSECONDS_PER_SECOND = 1_000_000_000


class Timestamp:
    """A Timestamp message read as a datetime, or written from one, always in UTC."""

    @staticmethod
    def to_datetime(message: Any) -> datetime:
        return datetime.fromtimestamp(message.seconds + message.nanos / NANOSECONDS_PER_SECOND, tz=UTC)

    @staticmethod
    def fill(message: Any, moment: datetime) -> None:
        seconds = moment.timestamp()
        message.seconds = int(seconds // 1)
        message.nanos = int(round((seconds % 1) * NANOSECONDS_PER_SECOND))
