#!/usr/bin/bash

set -euo pipefail

get_logs() {
  kubectl -n bluesky logs deployment/pds
}

relavent_logs() {
  jq '. | select(
    .req != null
    and .res != null
    and .req.url != "/xrpc/_health"
  )'
}

top_urls_for_ip() {
  local ip="${1:-}"
  if [ -z "${ip:-}" ]; then
    jq -r '.
        | select(
          .req != null
          and .res != null
          and .req.url != "/xrpc/_health"
        )
        | "\(.req.method) \(.req.url)"
        | sub("\\?.*"; "")' \
      | sort \
      | uniq -c \
      | sort -rn
  else
    jq -r --arg IP "$ip" '.
        | select(
          .req != null
          and .res != null
          and .req.url != "/xrpc/_health"
          and .req.headers."cf-connecting-ip" == $IP
        )
        | "\(.req.url)"
        | sub("\\?.*"; "")' \
      | sort \
      | uniq -c \
      | sort -rn
  fi
}

if [ -n "$*" ]; then
  "$@"
else
  relavent_logs
fi
