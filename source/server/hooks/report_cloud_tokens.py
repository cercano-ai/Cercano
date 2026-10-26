#!/usr/bin/env python3
"""
PostToolUse hook for Claude Code that captures cloud token usage from the
transcript and writes it to Cercano's telemetry database.

Reads hook input (JSON) from stdin, parses the transcript JSONL to extract
cumulative cloud token usage, computes the delta since last report, and
inserts into the cloud_usage table.

Data lives in the active Cercano profile, like the agent's:
~/.config/cercano/profiles/<profile>/{telemetry.db,hook_state.json}, where
<profile> is $CERCANO_PROFILE or "default". An invalid profile name makes the
hook exit quietly without writing anything; it must never break the host tool.

Usage in Claude Code settings.json:
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "mcp__cercano__.*",
        "command": "python3 /path/to/report_cloud_tokens.py"
      }
    ]
  }
}
"""

import json
import os
import re
import sqlite3
import sys

# Same rule as Go's paths.ValidateProfileName.
PROFILE_NAME_RE = re.compile(r"[a-z0-9][a-z0-9-]{0,31}")


def profile_dir():
    """Return the active profile's directory, or None if CERCANO_PROFILE is invalid."""
    name = os.environ.get("CERCANO_PROFILE", "") or "default"
    if not PROFILE_NAME_RE.fullmatch(name):
        return None
    return os.path.join(os.path.expanduser("~"), ".config", "cercano", "profiles", name)


def read_hook_input():
    """Read the hook input JSON from stdin."""
    try:
        return json.loads(sys.stdin.read())
    except (json.JSONDecodeError, EOFError):
        return None


def parse_transcript(transcript_path):
    """Parse transcript JSONL and return cumulative cloud token counts."""
    total_input = 0
    total_output = 0
    total_cache_creation = 0
    total_cache_read = 0

    try:
        with open(transcript_path, "r") as f:
            for line in f:
                line = line.strip()
                if not line:
                    continue
                try:
                    entry = json.loads(line)
                except json.JSONDecodeError:
                    continue

                if entry.get("type") != "assistant":
                    continue

                usage = entry.get("message", {}).get("usage", {})
                if not usage:
                    continue

                total_input += usage.get("input_tokens", 0)
                total_output += usage.get("output_tokens", 0)
                total_cache_creation += usage.get("cache_creation_input_tokens", 0)
                total_cache_read += usage.get("cache_read_input_tokens", 0)
    except (OSError, IOError):
        return None

    return {
        "input_tokens": total_input,
        "output_tokens": total_output,
        "cache_creation_input_tokens": total_cache_creation,
        "cache_read_input_tokens": total_cache_read,
        "total_tokens": total_input + total_output + total_cache_creation + total_cache_read,
    }


def load_state(state_file, session_id):
    """Load the last-reported cumulative totals for this session."""
    try:
        with open(state_file, "r") as f:
            state = json.load(f)
        return state.get(session_id, {})
    except (OSError, IOError, json.JSONDecodeError):
        return {}


def save_state(state_file, session_id, totals):
    """Save cumulative totals so next invocation can compute the delta."""
    state = {}
    try:
        with open(state_file, "r") as f:
            state = json.load(f)
    except (OSError, IOError, json.JSONDecodeError):
        pass

    state[session_id] = totals

    os.makedirs(os.path.dirname(state_file), exist_ok=True)
    with open(state_file, "w") as f:
        json.dump(state, f)


def write_to_db(db_path, delta_input, delta_output):
    """Insert cloud usage delta into Cercano's telemetry database."""
    if delta_input <= 0 and delta_output <= 0:
        return
    # The agent creates the database with its schema; never create an empty
    # one here.
    if not os.path.exists(db_path):
        return

    try:
        conn = sqlite3.connect(db_path)
        conn.execute(
            """INSERT INTO cloud_usage
               (timestamp, cloud_input_tokens, cloud_output_tokens, cloud_provider, cloud_model)
               VALUES (datetime('now'), ?, ?, 'anthropic', 'claude-code')""",
            (delta_input, delta_output),
        )
        conn.commit()
        conn.close()
    except sqlite3.Error as e:
        print(f"cercano hook: db error: {e}", file=sys.stderr)


def main():
    directory = profile_dir()
    if directory is None:
        return
    state_file = os.path.join(directory, "hook_state.json")
    db_path = os.path.join(directory, "telemetry.db")

    hook_input = read_hook_input()
    if not hook_input:
        return

    transcript_path = hook_input.get("transcript_path", "")
    session_id = hook_input.get("session_id", "unknown")

    if not transcript_path or not os.path.exists(transcript_path):
        return

    # Parse current cumulative totals from transcript
    totals = parse_transcript(transcript_path)
    if totals is None:
        return

    # Load previous state and compute delta
    prev = load_state(state_file, session_id)
    delta_input = totals["input_tokens"] - prev.get("input_tokens", 0)
    delta_output = totals["output_tokens"] - prev.get("output_tokens", 0)

    # Write delta to telemetry DB
    write_to_db(db_path, delta_input, delta_output)

    # Save current state for next invocation
    save_state(state_file, session_id, totals)


if __name__ == "__main__":
    main()
