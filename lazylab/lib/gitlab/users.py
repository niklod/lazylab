from lazylab.lib.context import LazyLabContext
from lazylab.models.gitlab import User


async def get_current_user() -> User:
    return await LazyLabContext.client.get_current_user()
