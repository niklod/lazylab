from textual.app import App

from lazylab.lib.bindings import LazyLabBindings
from lazylab.lib.context import LazyLabContext
from lazylab.lib.logging import ll


class LazyLab(App):
    """LazyLab - A Terminal UI for interacting with GitLab"""

    TITLE = "LazyLab"

    BINDINGS = [
        LazyLabBindings.QUIT_APP,
        LazyLabBindings.OPEN_COMMAND_PALETTE,
        LazyLabBindings.OPEN_HELP,
    ]

    async def on_ready(self) -> None:
        config = LazyLabContext.config
        if not config.gitlab.token:
            self.notify(
                "No GitLab token configured. Edit ~/.config/gitlab-tui/config.yaml",
                title="Configuration Required",
                severity="error",
                timeout=10,
            )
            ll.error("No GitLab token configured")
            return

        try:
            user = await LazyLabContext.client.get_current_user()
            ll.info(f"Authenticated as {user.username}")
            self.notify(f"Authenticated as {user.username}", title="LazyLab")
        except Exception:
            self.notify(
                "Authentication failed. Check your token and GitLab URL in ~/.config/gitlab-tui/config.yaml",
                title="Error",
                severity="error",
                timeout=10,
            )
            ll.exception("Authentication failed")
            return

        from lazylab.ui.screens.primary import LazyLabMainScreen

        main_screen = LazyLabMainScreen()
        await self.push_screen(main_screen)

    def on_mount(self) -> None:
        self.theme = LazyLabContext.config.appearance.theme.name
