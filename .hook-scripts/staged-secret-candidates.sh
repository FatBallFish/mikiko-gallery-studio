#!/usr/bin/env bash
set -euo pipefail

awk '
  /(^|\/)\.env(\.[^\/]*)?\.example$/ { next }
  /(^|\/)\.env($|\.|\/)/ { print; next }
  /^\.hook-scripts\/staged-secret-candidates\.sh$/ { next }
  /^scripts\/workflow\/staged-secret-candidates-contract-test\.sh$/ { next }
  /^internal\/repository\/ent\/schema\/clustertoken\.go$/ { next }
  /^internal\/repository\/ent\/clustertoken\.go$/ { next }
  /^internal\/repository\/ent\/clustertoken_(create|delete|query|update)\.go$/ { next }
  /^internal\/repository\/ent\/clustertoken\/(clustertoken|where)\.go$/ { next }
  /^internal\/http\/setupui\/admin_design_tokens\.generated\.go$/ { next }
  /^web\/shared\/admin-design-tokens\.css$/ { next }
  {
    name=$0
    sub(/^.*\//, "", name)
    lower=tolower(name)
    if (lower == "tokens.css") next
    if (lower ~ /id_rsa|private[._-]?key|secret|token/) print
  }
'
