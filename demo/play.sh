#!/bin/sh
# Replays the demo session line by line so claude-companion shows it arriving.
# Terminal 1:  sh demo/play.sh            (creates the replayed file, then appends a line every 0.8 s)
# Terminal 2:  CLAUDE_CONFIG_DIR="$PWD/demo" claude-companion demo1111
set -e
cd "$(dirname "$0")"
src=projects/-demo/demo0000-0000-0000-0000-000000000000.jsonl
live=projects/-demo/demo1111-1111-1111-1111-111111111111.jsonl
: > "$live"
while IFS= read -r line; do
  printf '%s\n' "$line" >> "$live"
  sleep 0.8
done < "$src"
