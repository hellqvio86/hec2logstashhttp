#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <coverage.out> <badge.svg>"
  exit 1
fi

coverage_file="$1"
badge_file="$2"

if [[ ! -f "$coverage_file" ]]; then
  echo "coverage file not found: $coverage_file"
  exit 1
fi

coverage_value="$(go tool cover -func="$coverage_file" | awk '/^total:/{gsub("%","",$3); print $3}')"
if [[ -z "$coverage_value" ]]; then
  echo "failed to parse total coverage from $coverage_file"
  exit 1
fi

coverage="$(printf "%.1f" "$coverage_value")"

color="red"
awk -v c="$coverage" 'BEGIN { exit !(c >= 90) }' && color="brightgreen"
awk -v c="$coverage" 'BEGIN { exit !(c >= 80 && c < 90) }' && color="green"
awk -v c="$coverage" 'BEGIN { exit !(c >= 70 && c < 80) }' && color="yellowgreen"
awk -v c="$coverage" 'BEGIN { exit !(c >= 60 && c < 70) }' && color="yellow"
awk -v c="$coverage" 'BEGIN { exit !(c >= 50 && c < 60) }' && color="orange"

mkdir -p "$(dirname "$badge_file")"

cat > "$badge_file" <<EOF
<svg xmlns="http://www.w3.org/2000/svg" width="132" height="20" role="img" aria-label="coverage: ${coverage}%">
  <linearGradient id="s" x2="0" y2="100%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <mask id="m">
    <rect width="132" height="20" rx="3" fill="#fff"/>
  </mask>
  <g mask="url(#m)">
    <rect width="72" height="20" fill="#555"/>
    <rect x="72" width="60" height="20" fill="${color}"/>
    <rect width="132" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="110">
    <text x="370" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)" lengthAdjust="spacing">coverage</text>
    <text x="370" y="140" transform="scale(.1)" lengthAdjust="spacing">coverage</text>
    <text x="1010" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)" lengthAdjust="spacing">${coverage}%</text>
    <text x="1010" y="140" transform="scale(.1)" lengthAdjust="spacing">${coverage}%</text>
  </g>
</svg>
EOF

echo "coverage badge updated: $badge_file (${coverage}%)"
