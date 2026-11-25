"""OTel Profiler: samples Home Assistant's Python threads and pushes pprof to the add-on.

Installed and set up by the OTel Collector add-on when its `profiles` option is on.
"""

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant

from .profiler import Sampler

# The add-on's pprof receiver; Home Assistant Core and the add-on share the host network.
ENDPOINT = "http://127.0.0.1:4040/v1/pprof"


async def async_setup_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    sampler = Sampler(ENDPOINT)
    sampler.start()
    entry.async_on_unload(sampler.stop)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: ConfigEntry) -> bool:
    return True
