#!/usr/bin/env bash
# Runs the integration tests against several Docker Engine versions, each as
# its own three-node development swarm, one after the other.
#
#   scripts/matrix.sh                     oldest supported engine and the latest
#   D8S_MATRIX="img1 img2" scripts/matrix.sh
#
# Images come from a public mirror of the official ones, which has no
# anonymous pull limit.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mirror="public.ecr.aws/docker/library/docker"
images=(${D8S_MATRIX:-$mirror:20.10-dind $mirror:dind})

wait_for_stack() {
	for _ in $(seq 1 90); do
		ready="$(docker --context d8s-swarm service ls --format '{{.Replicas}}' 2>/dev/null | tr '\n' ' ')"
		[[ "$ready" == "2/2 3/3 " || "$ready" == "3/3 2/2 " ]] && return 0
		sleep 2
	done
	echo "the sample stack did not become ready (replicas: $ready)" >&2
	return 1
}

results=()
failed=0
for image in "${images[@]}"; do
	echo "=== $image"
	outcome="FAIL"
	engine="?"
	if D8S_DIND_IMAGE="$image" "$root/scripts/swarm.sh" up >/dev/null && wait_for_stack; then
		engine="$(docker --context d8s-swarm version --format '{{.Server.Version}} (API {{.Server.APIVersion}})' 2>/dev/null)"
		if (cd "$root" && make --no-print-directory test-integration); then
			outcome="pass"
		fi
	fi
	[[ "$outcome" == "pass" ]] || failed=1
	results+=("$(printf '%-52s %-22s %s' "$image" "$engine" "$outcome")")
	"$root/scripts/swarm.sh" down >/dev/null
done

echo
printf '%-52s %-22s %s\n' "IMAGE" "ENGINE" "RESULT"
printf '%s\n' "${results[@]}"
exit "$failed"
