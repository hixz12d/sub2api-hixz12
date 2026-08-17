#!/usr/bin/env bash
set -euo pipefail

# Read-only deployment evidence for the OpenAI isolation boundary.
# Usage:
#   deploy/verify-openai-deployment.sh [container] [config-file]
#
# The command does not restart, exec into, or mutate a container. It records
# enough immutable identifiers to prove which commit/image/config is running.

container_name="${1:-sub2api}"
config_file="${2:-${SUB2API_CONFIG:-}}"
expected_revision="${EXPECTED_GIT_REVISION:-$(git rev-parse HEAD 2>/dev/null || true)}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required" >&2
  exit 2
fi

container_id="$(docker ps --filter "name=^/${container_name}$" --format '{{.ID}}' | head -n 1)"
if [[ -z "${container_id}" ]]; then
  echo "running container not found: ${container_name}" >&2
  exit 1
fi

inspect="docker inspect ${container_id}"
image_id="$(docker inspect --format '{{.Image}}' "${container_id}")"
image_name="$(docker inspect --format '{{.Config.Image}}' "${container_id}")"
container_started="$(docker inspect --format '{{.State.StartedAt}}' "${container_id}")"
container_revision="$(docker inspect --format '{{index .Config.Labels \"org.opencontainers.image.revision\"}}' "${container_id}")"
image_revision="$(docker image inspect --format '{{index .Config.Labels \"org.opencontainers.image.revision\"}}' "${image_id}" 2>/dev/null || true)"
repo_digests="$(docker image inspect --format '{{join .RepoDigests "\n"}}' "${image_id}" 2>/dev/null || true)"

printf 'container_name=%s\n' "${container_name}"
printf 'container_id=%s\n' "${container_id}"
printf 'container_started_at=%s\n' "${container_started}"
printf 'image_name=%s\n' "${image_name}"
printf 'image_id=%s\n' "${image_id}"
printf 'container_revision=%s\n' "${container_revision:-<unset>}"
printf 'image_revision=%s\n' "${image_revision:-<unset>}"
printf 'expected_revision=%s\n' "${expected_revision:-<unset>}"
if [[ -n "${expected_revision}" && ( "${container_revision}" == "${expected_revision}" || "${image_revision}" == "${expected_revision}" ) ]]; then
  printf 'revision_match=true\n'
else
  printf 'revision_match=false\n'
fi
printf 'repo_digests=%s\n' "${repo_digests:-<unset>}"

if [[ -n "${config_file}" ]]; then
  if [[ ! -f "${config_file}" ]]; then
    echo "config file not found: ${config_file}" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    config_sha256="$(sha256sum "${config_file}" | awk '{print $1}')"
  else
    config_sha256="$(shasum -a 256 "${config_file}" | awk '{print $1}')"
  fi
  printf 'config_file=%s\n' "${config_file}"
  printf 'config_sha256=%s\n' "${config_sha256}"
  printf 'config_openai_egress=\n'
  awk '
    /^openai_egress:/ { in_egress=1; print; next }
    in_egress && /^[^[:space:]]/ { in_egress=0 }
    in_egress { print }
  ' "${config_file}"
  printf 'config_account_scoped_identity=\n'
  awk '
    /^openai_account_scoped_identity:/ { in_identity=1; print; next }
    in_identity && /^[^[:space:]]/ { in_identity=0 }
    in_identity { print }
  ' "${config_file}"
fi

printf 'verification_commands=\n'
printf '%s\n' "${inspect}" "docker image inspect ${image_id}"