# shellcheck shell=bash
# Sourced by scripts/test-integration.sh and scripts/doctor.sh: the one spelling of the
# Docker admission lock path. It is SemStreams' lock, adopted byte-compatibly (owner
# ruling Q1, 2026-09-30) so either repository's runner sees the other as an ordinary
# owner. A joint rename is this one line here and one in SemStreams'
# scripts/run-integration-tests.sh.
readonly admission_lock_default="/tmp/semstreams-integration.lock"
