import json
import os
from pathlib import Path
from typing import Iterable, TypeVar

from pydantic import BaseModel, ValidationError

from lazylab.lib.logging import ll

T = TypeVar("T", bound=BaseModel)


def _cache_path(cache_dir: Path, project_path: str | None, cache_name: str) -> Path:
    if project_path:
        filename = f"{project_path.replace('/', '_')}_{cache_name}.json"
    else:
        filename = f"{cache_name}.json"
    return cache_dir / filename


def load_models_from_cache(
    cache_dir: Path, project_path: str | None, cache_name: str, expect_type: type[T]
) -> list[T]:
    path = _cache_path(cache_dir, project_path, cache_name)
    if not path.is_file():
        return []

    try:
        cached_objects = json.loads(path.read_text())
        return [expect_type(**raw_obj) for raw_obj in cached_objects]
    except json.JSONDecodeError as e:
        ll.warning(f"Failed to parse cache file '{path}' as JSON: {e}")
    except ValidationError as e:
        ll.warning(f"Cache schema mismatch in '{path}' for {expect_type.__name__}: {e}")
    except Exception as e:
        ll.warning(f"Unexpected error loading cache from '{path}': {e}")

    return []


def save_models_to_cache(
    cache_dir: Path, project_path: str | None, cache_name: str, objects: Iterable[T]
) -> None:
    path = _cache_path(cache_dir, project_path, cache_name)
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    path.write_text(json.dumps([o.model_dump(mode="json") for o in objects]))
    os.chmod(path, 0o600)
