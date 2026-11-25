"""The Adrient Workspace integration.

Connects Home Assistant to a Adrient Workspace server with a personal API token:
Adrient Calendar becomes a calendar entity, every Adrient task list a to-do list,
unread notifications / open tasks become sensors, and a notify entity lets
automations push notifications into Adrient.
"""

from __future__ import annotations

import aiohttp

from homeassistant.const import CONF_TOKEN, CONF_URL, Platform
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import ConfigEntryAuthFailed, ConfigEntryNotReady
from homeassistant.helpers.aiohttp_client import async_create_clientsession

from .api import AdrientAuthError, AdrientClient, AdrientError
from .coordinator import AdrientConfigEntry, AdrientCoordinator, AdrientRuntimeData

PLATFORMS: list[Platform] = [
    Platform.CALENDAR,
    Platform.NOTIFY,
    Platform.SENSOR,
    Platform.TODO,
]


async def async_setup_entry(hass: HomeAssistant, entry: AdrientConfigEntry) -> bool:
    """Set up Adrient from a config entry."""
    # Own session with no cookie jar: Adrient authenticates a session cookie in
    # preference to the bearer token, so cookies must never be replayed. HA
    # detaches it when the entry unloads.
    session = async_create_clientsession(hass, cookie_jar=aiohttp.DummyCookieJar())
    client = AdrientClient(session, entry.data[CONF_URL], entry.data[CONF_TOKEN])
    try:
        # None on Adrient older than 0.4 (no info/push endpoints): everything but
        # the identity lookup and notify still works.
        account = await client.async_get_account_or_none()
    except AdrientAuthError as err:
        raise ConfigEntryAuthFailed(f"Adrient rejected the API token: {err}") from err
    except AdrientError as err:
        raise ConfigEntryNotReady(f"Cannot reach Adrient: {err}") from err

    coordinator = AdrientCoordinator(hass, entry, client)
    await coordinator.async_config_entry_first_refresh()
    entry.runtime_data = AdrientRuntimeData(client=client, coordinator=coordinator, account=account)

    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: AdrientConfigEntry) -> bool:
    """Unload a config entry."""
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
