import json
from pathlib import Path
from typing import Any

LOCALES_DIR = Path(__file__).parent.parent / "locales"

_cache: dict[str, dict[str, Any]] = {}


def _load(lang: str) -> dict[str, Any]:
    if lang not in _cache:
        path = LOCALES_DIR / f"{lang}.json"
        if not path.exists():
            if lang != "en":
                return _load("en")
            raise FileNotFoundError(f"Locale file not found: {path}")
        _cache[lang] = json.loads(path.read_text(encoding="utf-8"))
    return _cache[lang]


def get(key: str, lang: str = "en", **kwargs: Any) -> str:
    keys = key.split(".")
    value: Any = _load(lang)
    for k in keys:
        if k not in value:
            if lang != "en":
                value = _load("en")
                for k2 in keys:
                    value = value[k2]
                break
            raise KeyError(f"Missing locale key: {key}")
        value = value[k]
    if kwargs:
        return str(value).format(**kwargs)
    return str(value)
