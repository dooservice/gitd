"""The calls that read and set the rules publishing follows: for a platform's own pages,
never for an agent."""

from __future__ import annotations

from . import messages, methods
from .connection import Connection
from .publishing_settings import PublishSettings


class Publishing:
    """The publishing settings, per repository prefix, as gitd.publishing.

    gitd.publishing.set(PublishSettings(url_prefix="https://github.com/acme", preset=Preset.RECOMMENDED,
                                        branch_prefix="agents/"))
    """

    def __init__(self, connection: Connection) -> None:
        self.connection = connection

    def settings(self) -> list[PublishSettings]:
        """Every setting, gitd.toml's included."""
        response = self.connection.call(methods.GET_SETTINGS, messages.GetSettingsRequest())
        return [PublishSettings.from_message(settings) for settings in response.settings]

    def set(self, settings: PublishSettings) -> PublishSettings:
        """Keeps the settings of a prefix, replacing those it had, as gitd checked them."""
        request = messages.SetSettingsRequest(settings=settings.to_message())
        return PublishSettings.from_message(self.connection.call(methods.SET_SETTINGS, request).settings)

    def remove(self, url_prefix: str) -> None:
        """Forgets the settings of a prefix set through the API."""
        self.connection.call(methods.REMOVE_SETTINGS, messages.RemoveSettingsRequest(url_prefix=url_prefix))
