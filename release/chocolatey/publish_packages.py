#!/usr/bin/env python3
"""Submit the exact inspected run artifact via NuGet v2; never retry mutations.

Uses the same multipart PUT /api/v2/package and X-NuGet-ApiKey as choco's
NugetPush/PackageUpdateResource, without CLI arguments, persisted key config,
NuGet's HTTP retries, redirects, or raw server output. Only HTTP 201 is success.
"""
import argparse
import hashlib
import http.client
import json
import os
from pathlib import Path
import sys
import uuid

import pack_packages
import render_packages

HOST = "push.chocolatey.org"
ENDPOINT = "/api/v2/package"
ValidationError = render_packages.ValidationError


def submit(name, data, key):
    boundary = uuid.uuid4().hex
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="package"; '
            'filename="package.nupkg"\r\nContent-Type: application/octet-stream\r\n\r\n').encode()
    body += data + f"\r\n--{boundary}--\r\n".encode()
    connection = http.client.HTTPSConnection(HOST, timeout=120)
    try:
        connection.request("PUT", ENDPOINT, body=body, headers={
            "Content-Type": f"multipart/form-data; boundary={boundary}",
            "X-NuGet-ApiKey": key,
        })
        response = connection.getresponse()
        # Never follow redirects, retry, or expose response bodies/reason phrases.
        if response.status != 201:
            raise ValidationError(f"{name}: submission not confirmed (HTTP {response.status}); stopped, no retry")
    except ValidationError:
        raise
    except Exception:
        raise ValidationError(f"{name}: ambiguous submission outcome; stopped, no retry") from None
    finally:
        connection.close()


def publish(version, package_version, packages, digests, key, send=submit):
    version = render_packages.parse_version(version)
    package_version = render_packages.parse_package_version(package_version or version, version)
    if not key or not key.isascii() or any(c.isspace() or ord(c) < 33 for c in key):
        raise ValidationError("CHOCOLATEY_API_KEY missing or invalid")
    names = [f"{p}.{package_version}.nupkg" for p in pack_packages.PACKAGE_IDS]
    root = Path(packages)
    expected_files = set(names) | {n + ".sha256" for n in names}
    if set(digests) != set(names) or {p.name for p in root.iterdir()} != expected_files:
        raise ValidationError("expected exactly this run's three packages and checksums")
    # Validate ALL packages before the first mutation. Retain those exact bytes
    # so even a local file replacement cannot change the submitted payload.
    payloads = []
    for name in names:
        path = root / name
        if path.is_symlink() or not path.is_file():
            raise ValidationError("package must be a regular file")
        data = path.read_bytes()
        digest = render_packages.parse_digest(digests[name])
        if hashlib.sha256(data).hexdigest() != digest:
            raise ValidationError(f"{name}: run digest mismatch")
        if (root / (name + ".sha256")).read_text(encoding="ascii") != f"{digest}  {name}\n":
            raise ValidationError(f"{name}: checksum sidecar mismatch")
        payloads.append((name, data))
    for name, data in payloads:
        send(name, data, key)
        print(f"Submitted {name}; moderation approval is separate and not established.", flush=True)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--package-version")
    parser.add_argument("--packages", required=True)
    args = parser.parse_args(argv)
    try:
        publish(args.version, args.package_version, args.packages,
                json.loads(os.environ.get("PACKAGE_DIGESTS", "{}")),
                os.environ.pop("CHOCOLATEY_API_KEY", ""))
    except Exception:
        # No traceback, request headers, credential, server text, or arbitrary
        # exception contents can reach CI logs. Prior successful IDs remain logged.
        print("Submission stopped; outcome may be partial/ambiguous. Inspect repository state manually; do not blindly rerun.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
