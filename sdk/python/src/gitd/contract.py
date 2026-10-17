"""gitd's contract, read at run time from contract.json, which `make generate` builds from
proto/gitd/v1/gitd.proto. Only protobuf's public API is used: no code is generated."""

from __future__ import annotations

from importlib import resources

from google.protobuf import descriptor_pb2, descriptor_pool, json_format, message_factory
from google.protobuf.descriptor import EnumDescriptor
from google.protobuf.message import Message

PACKAGE = "gitd.v1"
CONTRACT_FILE = "contract.json"


class Enumeration:
    """An enumeration of the contract, by the names and numbers of its values."""

    def __init__(self, descriptor: EnumDescriptor) -> None:
        self.descriptor = descriptor

    def number(self, name: str) -> int:
        return self.descriptor.values_by_name[name].number

    def name(self, number: int) -> str:
        """The value's name, or an empty one for a value newer than this client."""
        value = self.descriptor.values_by_number.get(number)
        return value.name if value else ""


class Contract:
    """The messages and enumerations of gitd's API."""

    def __init__(self, pool: descriptor_pool.DescriptorPool) -> None:
        self.pool = pool

    @classmethod
    def load(cls) -> Contract:
        text = resources.files(__package__).joinpath(CONTRACT_FILE).read_text(encoding="utf-8")
        files = json_format.Parse(text, descriptor_pb2.FileDescriptorSet())
        pool = descriptor_pool.DescriptorPool()
        for file in files.file:
            pool.Add(file)
        return cls(pool)

    def message(self, name: str) -> type[Message]:
        return message_factory.GetMessageClass(self.pool.FindMessageTypeByName(f"{PACKAGE}.{name}"))

    def enumeration(self, name: str) -> Enumeration:
        return Enumeration(self.pool.FindEnumTypeByName(f"{PACKAGE}.{name}"))
