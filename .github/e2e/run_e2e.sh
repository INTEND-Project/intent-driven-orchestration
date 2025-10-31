#!/bin/bash
set -eo pipefail

TAG="e2e"
REG="127.0.0.1:5000"
cd .github/e2e/

docker image tag planner:$TAG $REG/planner:$TAG && docker push $REG/planner:$TAG
docker image tag scaleout:$TAG $REG/scaleout:$TAG && docker push  $REG/scaleout:$TAG
docker image tag rmpod:$TAG $REG/rmpod:$TAG && docker push  $REG/rmpod:$TAG
docker image tag cpuscale:$TAG $REG/cpuscale:$TAG && docker push $REG/cpuscale:$TAG

kubectl delete ns test-ns || true # Clean old test-ns if it exists
kubectl create ns test-ns 
kubectl apply -n test-ns -f data/e2e-deployment.yaml

ginkgo -v --tags e2e  -- --kubeconfig ~/.kube/config

docker rmi planner:$TAG $REG/planner:$TAG  || true && crictl rmi $REG/planner:$TAG || true
docker rmi scaleout:$TAG $REG/scaleout:$TAG || true  && crictl rmi $REG/scaleout:$TAG || true
docker rmi rmpod:$TAG $REG/rmpod:$TAG || true  && crictl rmi $REG/rmpod:$TAG || true
docker rmi cpuscale:$TAG $REG/cpuscale:$TAG || true && crictl rmi $REG/cpuscale:$TAG || true

docker system prune -f
docker buildx prune -f
