#!/bin/sh
set -eu

endpoint="${EXPLORER_DEFAULT_ENDPOINT:-http://localhost:35997}"

case "$endpoint" in
  http://*|https://*) ;;
  *)
    echo "EXPLORER_DEFAULT_ENDPOINT must start with http:// or https://" >&2
    exit 1
    ;;
esac

case "$endpoint" in
  *[[:cntrl:]]*)
    echo "EXPLORER_DEFAULT_ENDPOINT must not contain control characters" >&2
    exit 1
    ;;
esac

endpoint_json=$(printf '%s' "$endpoint" | sed 's/\\/\\\\/g; s/"/\\"/g')

cat > /usr/share/nginx/html/devnet-endpoint.js <<EOF
(function () {
  var endpoint = "${endpoint_json}";
  try {
    var nodes = JSON.parse(localStorage.getItem("nodes") || "[]");
    if (!Array.isArray(nodes)) {
      nodes = [];
    }
    nodes = nodes.filter(function (node) {
      return node && node !== endpoint;
    });
    nodes.unshift(endpoint);
    localStorage.setItem("nodes", JSON.stringify(nodes));
    localStorage.setItem("defaultEndpoint", endpoint);
    window.__ZENON_DEVNET_ENDPOINT__ = endpoint;
  } catch (err) {
    console.warn("Unable to configure devnet explorer endpoint", err);
  }
})();
EOF
