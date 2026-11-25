from homeassistant.config_entries import ConfigFlow


class OtelLogsConfigFlow(ConfigFlow, domain="otel_logs"):
    """No options: one click (or the add-on) creates the entry."""

    VERSION = 1

    async def async_step_user(self, user_input=None):
        return self.async_create_entry(title="OTel Logs", data={})
