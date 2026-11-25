"""Push Home Assistant notifications into Adrient.

``notify.send_message`` on this entity creates a notification for the Adrient
user who owns the API token (POST /api/v1/notifications/push); it shows up in
Adrient's notification bell like any other.
"""

from __future__ import annotations

from homeassistant.components.notify import NotifyEntity, NotifyEntityFeature
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import HomeAssistantError
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .api import (
    AdrientApiError,
    AdrientAuthError,
    AdrientError,
    AdrientNotFoundError,
    AdrientRateLimitError,
)
from .const import DEFAULT_PUSH_TITLE, PUSH_MESSAGE_MAX, PUSH_TITLE_MAX
from .coordinator import AdrientConfigEntry, AdrientCoordinator
from .entity import AdrientEntity

NO_PUSH_ENDPOINT = (
    "This Adrient server has no notification push endpoint (Adrient 0.4 or later is needed)"
)

# Sends are serialized: Adrient rate-limits pushes per token.
PARALLEL_UPDATES = 1


def _clip(text: str, limit: int) -> str:
    text = text.strip()
    return text if len(text) <= limit else text[: limit - 1] + "…"


async def async_setup_entry(
    hass: HomeAssistant,
    entry: AdrientConfigEntry,
    async_add_entities: AddConfigEntryEntitiesCallback,
) -> None:
    """Set up the Adrient notify entity."""
    async_add_entities([AdrientNotifyEntity(entry.runtime_data.coordinator)])


class AdrientNotifyEntity(AdrientEntity, NotifyEntity):
    """Sends a notification to the connected Adrient user."""

    _attr_translation_key = "notifications"
    _attr_supported_features = NotifyEntityFeature.TITLE

    def __init__(self, coordinator: AdrientCoordinator) -> None:
        super().__init__(coordinator, "notify")

    async def async_send_message(self, message: str, title: str | None = None) -> None:
        """Push one notification into Adrient."""
        title = _clip(title or DEFAULT_PUSH_TITLE, PUSH_TITLE_MAX) or DEFAULT_PUSH_TITLE
        body = _clip(message, PUSH_MESSAGE_MAX)
        try:
            await self.coordinator.client.async_push_notification(title, body)
        except AdrientRateLimitError as err:
            wait = f"; retry in {err.retry_after}s" if err.retry_after else ""
            raise HomeAssistantError(
                f"Adrient is rate-limiting notifications from this token (60 per minute{wait})"
            ) from err
        except AdrientAuthError as err:
            self.coordinator.config_entry.async_start_reauth(self.hass)
            raise HomeAssistantError(
                "Adrient rejected the API token (it needs the notifications:write scope)"
            ) from err
        except AdrientNotFoundError as err:
            raise HomeAssistantError(NO_PUSH_ENDPOINT) from err
        except AdrientApiError as err:
            # Pre-0.4 grpc-gateway answers the unknown route with 405/501.
            if err.status in (405, 501):
                raise HomeAssistantError(NO_PUSH_ENDPOINT) from err
            raise HomeAssistantError(f"Could not send the notification to Adrient: {err}") from err
        except AdrientError as err:
            raise HomeAssistantError(f"Could not send the notification to Adrient: {err}") from err
