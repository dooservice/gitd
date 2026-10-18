#!/usr/bin/env bash
# A pod gets a workspace of gitd as a CSI volume, on a kind cluster: gitd and its plugin
# installed with the Helm chart, an agent's pod that commits in its workspace with no
# credential, and, when gitd keeps one, the platform publishing what it committed.
#
#   ./demo.sh                 make the cluster, install, run the pod
#   ./demo.sh down            delete the cluster
#
# Needs Docker, kind, kubectl and helm. REPOSITORY is the repository the pod works on; with
# CREDENTIALS_DIR (files for a Secret) and CONFIG (gitd.toml lines naming them), gitd can
# publish, as BRANCH.
set -euo pipefail
cd "$(dirname "$0")"
CLUSTER="${CLUSTER:-gitd}"
if [ "${1:-}" = down ]; then
  kind delete cluster --name "$CLUSTER"
  exit 0
fi
REPOSITORY="${REPOSITORY:-https://github.com/octocat/Hello-World}"
RUN="${RUN:-local-$(date +%s)}"

kind get clusters | grep -qx "$CLUSTER" || kind create cluster --name "$CLUSTER" --wait 120s
docker build -q -t gitd:kind ../.. > /dev/null
kind load docker-image gitd:kind --name "$CLUSTER"

helm_args=(--set image.repository=gitd --set image.tag=kind --set image.pullPolicy=Never)
if [ -n "${CREDENTIALS_DIR:-}" ]; then
  kubectl create secret generic gitd-credentials --from-file="$CREDENTIALS_DIR" --dry-run=client -o yaml | kubectl apply -f -
  helm_args+=(--set credentialsSecret=gitd-credentials --set-string "config=${CONFIG:-}")
fi
helm upgrade --install gitd ../../charts/gitd "${helm_args[@]}" --wait --timeout 3m
kubectl rollout status daemonset/gitd --timeout 3m

echo "== an agent's pod with a workspace volume"
kubectl create namespace agents --dry-run=client -o yaml | kubectl apply -f -
kubectl delete pod -n agents agent-1 --ignore-not-found --wait
sed -e "s#https://github.com/octocat/Hello-World#$REPOSITORY#" -e "s#value: \"local\"#value: \"$RUN\"#" agent.yaml | kubectl apply -f -
for _ in $(seq 90); do
  kubectl logs -n agents agent-1 2>/dev/null | grep -q committed && break
  sleep 2
done
kubectl logs -n agents agent-1

GITD_POD="$(kubectl get pod -l app.kubernetes.io/name=gitd -o jsonpath='{.items[0].metadata.name}')"
WORKSPACE=/var/lib/gitd/workspaces/agents/agent-1
echo "== the workspace, as gitd sees it on the node"
kubectl exec "$GITD_POD" -c gitd -- gitd describe "$WORKSPACE"
kubectl exec "$GITD_POD" -c gitd -- gitd changes "$WORKSPACE"

if [ -n "${BRANCH:-}" ]; then
  echo "== the platform publishes what the agent committed"
  kubectl exec "$GITD_POD" -c gitd -- gitd --json publish "$WORKSPACE" "$BRANCH" --agent "kubernetes pod" | tee published.json
fi

echo "== the pod goes, and its workspace with it"
kubectl delete pod -n agents agent-1 --wait
for _ in $(seq 30); do
  kubectl exec "$GITD_POD" -c gitd -- test -e "$WORKSPACE" || break
  sleep 2
done
kubectl exec "$GITD_POD" -c gitd -- test -e "$WORKSPACE" && { echo "the workspace outlived its pod"; exit 1; }
echo "removed"
