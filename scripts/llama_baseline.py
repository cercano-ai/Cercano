"""Offline baseline metadata gate. Never inspects a host or attests payload bytes."""
import argparse
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1] / "release" / "llama"


def read_json(path):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            if key in result:
                raise ValueError(f"duplicate JSON key: {key}")
            result[key] = value
        return result
    return json.loads(Path(path).read_text(), object_pairs_hook=unique,
                      parse_constant=lambda value: fail(f"invalid JSON constant: {value}"))


def fail(message):
    raise ValueError(message)


def validate_schema(value, schema, path="baseline"):
    """Only the checked-in schema's keyword subset; unknown keywords fail closed.

    No remote refs or third-party dependency. This is not a general JSON Schema
    implementation. Schema evolution must explicitly extend this gate and tests.
    """
    allowed = {"$schema", "$id", "title", "type", "const", "enum", "properties",
               "additionalProperties", "required", "items", "minItems", "maxItems",
               "pattern", "minLength", "minimum"}
    if set(schema) - allowed:
        fail(f"{path}: unsupported schema keywords")
    types = {"object": dict, "array": list, "string": str, "integer": int,
             "null": type(None)}
    if "type" in schema and type(value) is not types[schema["type"]]:
        fail(f"{path}: wrong type")
    if "const" in schema and (type(value) is not type(schema["const"]) or value != schema["const"]):
        fail(f"{path}: wrong constant")
    if "enum" in schema and not any(type(value) is type(v) and value == v for v in schema["enum"]):
        fail(f"{path}: unknown value")
    if isinstance(value, dict):
        if set(schema.get("required", [])) - value.keys():
            fail(f"{path}: missing fields")
        properties = schema.get("properties", {})
        if schema.get("additionalProperties") is False and value.keys() - properties.keys():
            fail(f"{path}: unknown fields")
        for key, item in value.items():
            if key in properties:
                validate_schema(item, properties[key], f"{path}.{key}")
    if isinstance(value, list):
        if not schema.get("minItems", 0) <= len(value) <= schema.get("maxItems", len(value)):
            fail(f"{path}: wrong item count")
        for i, item in enumerate(value):
            if "items" in schema:
                validate_schema(item, schema["items"], f"{path}[{i}]")
    if isinstance(value, str):
        if len(value) < schema.get("minLength", 0):
            fail(f"{path}: empty string")
        if "pattern" in schema and re.fullmatch(schema["pattern"], value) is None:
            fail(f"{path}: invalid format")
    if type(value) is int and value < schema.get("minimum", value):
        fail(f"{path}: below minimum")


def validate(lock):
    validate_schema(lock, read_json(ROOT / "baseline.schema.json"))
    if not lock["source"]["revision"].startswith(lock["observed_build"]["version_revision"]):
        fail("source revision disagrees with observed version revision")
    deps = lock["observed_build"]["runtime_dependencies"]
    if len({dep["name"] for dep in deps}) != len(deps):
        fail("duplicate runtime dependency")
    return lock


def load(path=ROOT / "baseline.lock.json"):
    return validate(read_json(path))


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("lock", nargs="?", type=Path, default=ROOT / "baseline.lock.json")
    args = parser.parse_args()
    try:
        load(args.lock)
    except (ValueError, OSError) as exc:
        parser.exit(1, f"llama baseline: {exc}\n")
    print("llama baseline metadata valid; no bundled payload or platform qualification asserted")
