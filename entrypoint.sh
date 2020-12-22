#!/bin/sh
# entrypoint.sh
# echo commands to the terminal output
set -ex
exec /sbin/tini -s -- /usr/bin/spark-ui-controller-envoy "$@"
