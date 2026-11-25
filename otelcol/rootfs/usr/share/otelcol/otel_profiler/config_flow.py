from homeassistant.config_entries import ConfigFlow


class OtelProfilerConfigFlow(ConfigFlow, domain="otel_profiler"):
    """No options: one click (or the add-on) creates the entry."""

    VERSION = 1

    async def async_step_user(self, user_input=None):
        return self.async_create_entry(title="OTel Profiler", data={})
