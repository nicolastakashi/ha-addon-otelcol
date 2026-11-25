"""OTel Logs: sends Home Assistant's Python log records to the add-on as OTLP logs, with
the trace context of the automation or script run that wrote them.

Installed and set up by the OTel Collector add-on when its `core_logs` option is on.
"""

import logging

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant

from .exporter import OtlpLogHandler

# The add-on's local OTLP receiver; Home Assistant Core and the add-on share the host network.
ENDPOINT = "http://127.0.0.1:4318/v1/logs"


async def async_setup_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    handler = OtlpLogHandler(hass, ENDPOINT)
    root = logging.getLogger()
    root.addHandler(handler)
    handler.start()

    def _unload() -> None:
        root.removeHandler(handler)
        handler.stop()

    entry.async_on_unload(_unload)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    return True
