"""Adrient counters as sensors."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass

from homeassistant.components.sensor import (
    SensorEntity,
    SensorEntityDescription,
    SensorStateClass,
)
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback
from homeassistant.util import dt as dt_util

from .coordinator import AdrientConfigEntry, AdrientCoordinator, AdrientData
from .entity import AdrientEntity

PARALLEL_UPDATES = 0


@dataclass(frozen=True, kw_only=True)
class AdrientSensorDescription(SensorEntityDescription):
    """Describes a Adrient counter."""

    value_fn: Callable[[AdrientData], int]


SENSORS: tuple[AdrientSensorDescription, ...] = (
    AdrientSensorDescription(
        key="unread_notifications",
        translation_key="unread_notifications",
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda data: data.unread_notifications,
    ),
    AdrientSensorDescription(
        key="open_tasks",
        translation_key="open_tasks",
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda data: data.open_tasks,
    ),
    AdrientSensorDescription(
        key="overdue_tasks",
        translation_key="overdue_tasks",
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=lambda data: data.overdue_tasks(dt_util.utcnow()),
    ),
)


async def async_setup_entry(
    hass: HomeAssistant,
    entry: AdrientConfigEntry,
    async_add_entities: AddConfigEntryEntitiesCallback,
) -> None:
    """Set up Adrient sensors."""
    coordinator = entry.runtime_data.coordinator
    async_add_entities(AdrientSensor(coordinator, description) for description in SENSORS)


class AdrientSensor(AdrientEntity, SensorEntity):
    """A Adrient counter."""

    entity_description: AdrientSensorDescription

    def __init__(self, coordinator: AdrientCoordinator, description: AdrientSensorDescription) -> None:
        super().__init__(coordinator, description.key)
        self.entity_description = description

    @property
    def native_value(self) -> int:
        return self.entity_description.value_fn(self.coordinator.data)
