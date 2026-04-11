import click

from lazylab.version import __version__


@click.group(invoke_without_command=True)
@click.pass_context
def cli(ctx: click.Context) -> None:
    """gt - A Terminal UI for interacting with GitLab"""
    if ctx.invoked_subcommand is None:
        ctx.invoke(run)


@cli.command()
def run() -> None:
    """Run LazyLab TUI"""
    from lazylab.ui.app import LazyLab

    app = LazyLab()
    app.run()


@cli.command()
def version() -> None:
    """Show LazyLab version"""
    click.echo(f"gt {__version__}")


@cli.command()
def dump_config() -> None:
    """Print current configuration (token redacted)"""
    import yaml

    from lazylab.lib.config import Config

    config = Config.load_config()
    data = config.model_dump(mode="json")
    if data.get("gitlab", {}).get("token"):
        data["gitlab"]["token"] = "<redacted>"
    click.echo(yaml.dump(data, default_flow_style=False, sort_keys=False))


@cli.command()
def clear_cache() -> None:
    """Clear the cache directory"""
    import shutil

    from lazylab.lib.config import Config

    config = Config.load_config()
    cache_dir = config.cache.directory
    if cache_dir.exists():
        shutil.rmtree(cache_dir)
        click.echo(f"Cleared cache at {cache_dir}")
    else:
        click.echo("No cache directory found")
