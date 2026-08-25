#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "${repo_root}"

chart=charts/gateway-lens
release=gateway-lens
namespace=default

# Strip helm-instance-specific noise so the output reads like a plain,
# hand-maintained manifest rather than a raw `helm template` dump.
clean() {
	sed \
		-e '1{/^---$/d;}' \
		-e '/^# Source:/d' \
		-e '/helm\.sh\/chart:/d' \
		-e '/app\.kubernetes\.io\/managed-by: Helm/d' \
		-e '/^[[:space:]]*$/d'
}

generate() {
	local manifest=$1
	shift
	{
		echo "# Generated from ${chart} via scripts/generate-manifests.sh. DO NOT EDIT BY HAND."
		helm template "${release}" "${chart}" --namespace "${namespace}" "$@" | clean
	} >"${manifest}"
}

generate deploy/manifests.yaml

for provider_file in "${chart}"/providers/*.yaml; do
	provider=$(basename "${provider_file}" .yaml)
	generate "deploy/manifests-${provider}.yaml" --set "rbac.providers[0]=${provider}"
done
