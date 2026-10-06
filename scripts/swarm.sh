#!/usr/bin/env bash
# A three-node Swarm for development and integration tests, running as
# docker-in-docker containers. The host daemon itself never joins a swarm.
#
#   scripts/swarm.sh up     create the cluster, its contexts and a sample stack
#   scripts/swarm.sh down   remove all of it
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
fixtures="$root/test/fixtures/swarm"

image="${D8S_DIND_IMAGE:-docker:27-dind}"
workload="${D8S_DEMO_IMAGE:-nginx:alpine}"
net="d8s-swarm-net"
manager_ctx="d8s-swarm"

# name:host-port. A node's context is named after its hostname, except the
# manager's, which is the one people type.
nodes=("d8s-swarm-manager:23750" "d8s-swarm-worker1:23751" "d8s-swarm-worker2:23752")

node_name() { echo "${1%%:*}"; }
node_port() { echo "${1##*:}"; }
node_ctx() { if [[ "$1" == "d8s-swarm-manager" ]]; then echo "$manager_ctx"; else echo "$1"; fi; }
on() { docker -H "tcp://127.0.0.1:$1" "${@:2}"; }

up() {
	down >/dev/null 2>&1 || true
	docker image inspect "$workload" >/dev/null 2>&1 || docker pull "$workload"
	docker network create "$net" >/dev/null

	for n in "${nodes[@]}"; do
		name="$(node_name "$n")" port="$(node_port "$n")"
		echo "starting $name"
		docker run -d --privileged --name "$name" --hostname "$name" --network "$net" \
			--label d8s.swarm=1 -e DOCKER_TLS_CERTDIR= \
			-p "127.0.0.1:$port:2375" -v "$name-data:/var/lib/docker" "$image" >/dev/null
	done

	for n in "${nodes[@]}"; do
		name="$(node_name "$n")" port="$(node_port "$n")"
		for _ in $(seq 1 60); do
			on "$port" info >/dev/null 2>&1 && break
			sleep 1
		done
		on "$port" info >/dev/null 2>&1 || { echo "$name did not come up" >&2; exit 1; }
		# Nodes get the workload image from the host, not from a registry.
		docker save "$workload" | on "$port" load >/dev/null
		docker context rm -f "$(node_ctx "$name")" >/dev/null 2>&1 || true
		docker context create "$(node_ctx "$name")" --docker "host=tcp://127.0.0.1:$port" >/dev/null
	done

	mport="$(node_port "${nodes[0]}")"
	maddr="$(docker inspect -f "{{(index .NetworkSettings.Networks \"$net\").IPAddress}}" "$(node_name "${nodes[0]}")")"
	on "$mport" swarm init --advertise-addr "$maddr" >/dev/null
	token="$(on "$mport" swarm join-token -q worker)"
	for n in "${nodes[@]:1}"; do
		on "$(node_port "$n")" swarm join --token "$token" "$maddr:2377" >/dev/null
	done

	(cd "$fixtures" && WORKLOAD_IMAGE="$workload" docker --context "$manager_ctx" stack deploy -c shop.yml shop >/dev/null)
	echo "swarm ready: docker --context $manager_ctx node ls"
	docker --context "$manager_ctx" node ls
}

down() {
	for n in "${nodes[@]}"; do
		name="$(node_name "$n")"
		docker context rm -f "$(node_ctx "$name")" >/dev/null 2>&1 || true
		docker rm -f -v "$name" >/dev/null 2>&1 || true
		docker volume rm "$name-data" >/dev/null 2>&1 || true
	done
	docker network rm "$net" >/dev/null 2>&1 || true
	echo "swarm removed"
}

case "${1:-}" in
up) up ;;
down) down ;;
*)
	echo "usage: $0 up | down" >&2
	exit 2
	;;
esac
